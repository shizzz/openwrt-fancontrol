package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// ControlMode defines which fan control algorithm is active.
type ControlMode string

const (
	ModePID   ControlMode = "pid"
	ModeFixed ControlMode = "fixed"
	ModeTable ControlMode = "table"
)

// FanConfig is the configuration for a single fan.  In UCI each section
// of type `fancontrol` becomes one FanConfig; in env-only mode a single
// fan named "default" is constructed from FANCTL_* variables.
type FanConfig struct {
	Name string // identifier (from UCI section name or "default" for env)

	Enabled bool
	Mode    ControlMode
	Setpoint float64

	Kp float64
	Ki float64
	Kd float64

	MinPWM   int
	MaxPWM   int
	FixedPWM int

	Interval time.Duration

	ThermalPath   string
	PWMPath       string
	PWMEnablePath string

	DryRun bool
	Debug  bool
}

// Config is the daemon-wide configuration: one or more fans.
type Config struct {
	Fans []FanConfig
}

// Load reads configuration:
//   - If /etc/config/fancontrol exists, it is parsed and env vars are
//     IGNORED.  Every `config fancontrol '<name>'` section that has
//     `option enabled '1'` (or omits `enabled`) becomes a running fan.
//   - If no UCI file exists, a single fan named "default" is built
//     from FANCTL_* env vars (always enabled).
//
// Returns an error if no fans end up enabled.
func Load() (*Config, error) {
	return loadAt(defaultUCIPath)
}

// loadAt is like Load but lets callers (and tests) override the UCI path.
func loadAt(uciPath string) (*Config, error) {
	switch _, err := os.Stat(uciPath); {
	case err == nil:
		return loadFromUCI(uciPath)
	case os.IsNotExist(err):
		return loadFromEnv()
	default:
		return nil, fmt.Errorf("stat uci %q: %w", uciPath, err)
	}
}

func loadFromUCI(path string) (*Config, error) {
	sections, err := parseUCISections(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	for _, sec := range sections {
		if sec.Type != "fancontrol" {
			continue
		}
		fan, err := fanFromUCI(sec)
		if err != nil {
			return nil, fmt.Errorf("fan section %q: %w", sectionLabel(sec), err)
		}
		if !fan.Enabled {
			continue
		}
		cfg.Fans = append(cfg.Fans, fan)
	}

	if len(cfg.Fans) == 0 {
		return nil, fmt.Errorf("no enabled fans in %s (every `config fancontrol` section has `option enabled '0'`, or no `fancontrol` sections are present)", path)
	}
	return cfg, nil
}

func loadFromEnv() (*Config, error) {
	fan, err := fanFromEnv()
	if err != nil {
		return nil, err
	}
	return &Config{Fans: []FanConfig{fan}}, nil
}

// sectionLabel returns a human-readable identifier for diagnostics.
func sectionLabel(sec UCISection) string {
	if sec.Name != "" {
		return "fancontrol." + sec.Name
	}
	return "fancontrol (anonymous)"
}

// fanFromUCI builds a FanConfig from a single UCI section, layered on
// top of hardcoded defaults.  Unrecognised option names are silently
// ignored.
func fanFromUCI(sec UCISection) (FanConfig, error) {
	fan := defaultsFan(sec.Name)
	if err := applyUCIOptions(&fan, sec.Options); err != nil {
		return fan, err
	}
	if err := validateFan(&fan); err != nil {
		return fan, err
	}
	return fan, nil
}

// fanFromEnv builds the single env-driven fan from FANCTL_* vars
// layered on top of defaults.
func fanFromEnv() (FanConfig, error) {
	fan := defaultsFan("default")
	applyEnvFan(&fan)
	if err := validateFan(&fan); err != nil {
		return fan, err
	}
	return fan, nil
}

// defaultsFan returns a FanConfig populated with OpenWrt-friendly
// defaults.  `name` is the fan identifier (UCI section name or
// "default" for env-only mode).
func defaultsFan(name string) FanConfig {
	return FanConfig{
		Name:          name,
		Enabled:       true,
		Mode:          ModePID,
		Setpoint:      55.0,
		Kp:            2.0,
		Ki:            0.5,
		Kd:            1.0,
		MinPWM:        0,
		MaxPWM:        255,
		FixedPWM:      128,
		Interval:      time.Second,
		ThermalPath:   "/sys/class/thermal/thermal_zone0/temp",
		PWMPath:       "/sys/class/hwmon/hwmon0/pwm1",
		PWMEnablePath: "/sys/class/hwmon/hwmon0/pwm1_enable",
	}
}

// applyUCIOptions overlays UCI options onto fan.  Unknown options are
// silently ignored so the binary can read future schema additions.
// Type errors in numeric/bool options are returned.
func applyUCIOptions(fan *FanConfig, opts map[string]string) error {
	if v, ok := opts["enabled"]; ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("enabled=%q: %w", v, err)
		}
		fan.Enabled = b
	}
	if v, ok := opts["thermal_path"]; ok {
		fan.ThermalPath = v
	}
	if v, ok := opts["pwm_path"]; ok {
		fan.PWMPath = v
	}
	if v, ok := opts["pwm_enable_path"]; ok {
		fan.PWMEnablePath = v
	}
	if v, ok := opts["mode"]; ok {
		switch ControlMode(v) {
		case ModePID, ModeFixed, ModeTable:
			fan.Mode = ControlMode(v)
		default:
			return fmt.Errorf("unknown mode %q", v)
		}
	}
	if v, ok := opts["min_pwm"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("min_pwm=%q: %w", v, err)
		}
		fan.MinPWM = n
	}
	if v, ok := opts["max_pwm"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("max_pwm=%q: %w", v, err)
		}
		fan.MaxPWM = n
	}
	if v, ok := opts["fixed_pwm"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("fixed_pwm=%q: %w", v, err)
		}
		fan.FixedPWM = n
	}
	for _, key := range []string{"kp", "ki", "kd", "setpoint"} {
		v, ok := opts[key]
		if !ok {
			continue
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("%s=%q: %w", key, v, err)
		}
		switch key {
		case "kp":
			fan.Kp = f
		case "ki":
			fan.Ki = f
		case "kd":
			fan.Kd = f
		case "setpoint":
			fan.Setpoint = f
		}
	}
	if v, ok := opts["interval_sec"]; ok {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("interval_sec=%q: %w", v, err)
		}
		fan.Interval = time.Duration(f * float64(time.Second))
	}
	if v, ok := opts["dry_run"]; ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("dry_run=%q: %w", v, err)
		}
		fan.DryRun = b
	}
	if v, ok := opts["debug"]; ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("debug=%q: %w", v, err)
		}
		fan.Debug = b
	}
	return nil
}

// applyEnvFan overlays FANCTL_* env vars onto fan.  Type errors are
// silently ignored (defaults are preserved).
func applyEnvFan(fan *FanConfig) {
	if v, ok := os.LookupEnv("FANCTL_ENABLED"); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			fan.Enabled = b
		}
	}
	if v, ok := os.LookupEnv("FANCTL_THERMAL_PATH"); ok {
		fan.ThermalPath = v
	}
	if v, ok := os.LookupEnv("FANCTL_PWM_PATH"); ok {
		fan.PWMPath = v
	}
	if v, ok := os.LookupEnv("FANCTL_PWM_ENABLE_PATH"); ok {
		fan.PWMEnablePath = v
	}
	if v, ok := os.LookupEnv("FANCTL_MODE"); ok {
		fan.Mode = ControlMode(v)
	}
	if v, ok := os.LookupEnv("FANCTL_INTERVAL_SEC"); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			fan.Interval = time.Duration(f * float64(time.Second))
		}
	}
	if v, ok := os.LookupEnv("FANCTL_SETPOINT"); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			fan.Setpoint = f
		}
	}
	if v, ok := os.LookupEnv("FANCTL_KP"); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			fan.Kp = f
		}
	}
	if v, ok := os.LookupEnv("FANCTL_KI"); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			fan.Ki = f
		}
	}
	if v, ok := os.LookupEnv("FANCTL_KD"); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			fan.Kd = f
		}
	}
	if v, ok := os.LookupEnv("FANCTL_MIN_PWM"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			fan.MinPWM = n
		}
	}
	if v, ok := os.LookupEnv("FANCTL_MAX_PWM"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			fan.MaxPWM = n
		}
	}
	if v, ok := os.LookupEnv("FANCTL_FIXED_PWM"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			fan.FixedPWM = n
		}
	}
	if v, ok := os.LookupEnv("FANCTL_DRY_RUN"); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			fan.DryRun = b
		}
	}
	if v, ok := os.LookupEnv("FANCTL_DEBUG"); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			fan.Debug = b
		}
	}
}

func validateFan(fan *FanConfig) error {
	switch fan.Mode {
	case ModePID, ModeFixed, ModeTable:
		// valid
	default:
		return fmt.Errorf("unknown control mode %q", fan.Mode)
	}

	if fan.MinPWM < 0 || fan.MinPWM > 255 {
		return fmt.Errorf("min_pwm must be 0–255, got %d", fan.MinPWM)
	}
	if fan.MaxPWM < 0 || fan.MaxPWM > 255 {
		return fmt.Errorf("max_pwm must be 0–255, got %d", fan.MaxPWM)
	}
	if fan.MinPWM > fan.MaxPWM {
		return fmt.Errorf("min_pwm (%d) must be <= max_pwm (%d)", fan.MinPWM, fan.MaxPWM)
	}

	if fan.Interval <= 0 {
		return fmt.Errorf("interval_sec must be > 0, got %v", fan.Interval)
	}

	if fan.ThermalPath == "" {
		return fmt.Errorf("thermal_path is required")
	}
	if fan.PWMPath == "" {
		return fmt.Errorf("pwm_path is required")
	}
	if fan.PWMEnablePath == "" {
		return fmt.Errorf("pwm_enable_path is required")
	}

	return nil
}