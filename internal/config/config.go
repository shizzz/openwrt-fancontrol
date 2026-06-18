// ===== file: internal/config/config.go =====

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

// Config holds all runtime configuration for the daemon.
// Values are sourced from environment variables so the daemon can be
// configured via /etc/init.d procd env blocks and later extended to
// read UCI config without breaking the existing interface.
type Config struct {
	// Hardware paths
	ThermalZonePath string // e.g. /sys/class/thermal/thermal_zone0/temp
	PWMPath         string // e.g. /sys/class/hwmon/hwmon0/pwm1
	PWMEnablePath   string // e.g. /sys/class/hwmon/hwmon0/pwm1_enable

	// Control
	ControlMode ControlMode
	Interval    time.Duration

	// Safety limits (raw PWM 0–255)
	MinPWM int
	MaxPWM int

	// Fixed mode
	FixedPWM int

	// PID parameters
	Kp          float64
	Ki          float64
	Kd          float64
	Setpoint    float64 // target temperature in °C
	PIDInterval float64 // dt in seconds, derived from Interval

	// Operational flags
	DryRun bool
	Debug  bool
}

// Load reads configuration from environment variables and returns a Config
// populated with sensible production defaults for OpenWrt.
func Load() (*Config, error) {
	cfg := &Config{
		ThermalZonePath: getenv("FANCTL_THERMAL_PATH", "/sys/class/thermal/thermal_zone0/temp"),
		PWMPath:         getenv("FANCTL_PWM_PATH", "/sys/class/hwmon/hwmon0/pwm1"),
		PWMEnablePath:   getenv("FANCTL_PWM_ENABLE_PATH", "/sys/class/hwmon/hwmon0/pwm1_enable"),

		ControlMode: ControlMode(getenv("FANCTL_MODE", string(ModePID))),

		MinPWM:   getenvInt("FANCTL_MIN_PWM", 0),
		MaxPWM:   getenvInt("FANCTL_MAX_PWM", 255),
		FixedPWM: getenvInt("FANCTL_FIXED_PWM", 128),

		Kp:       getenvFloat("FANCTL_KP", 2.0),
		Ki:       getenvFloat("FANCTL_KI", 0.5),
		Kd:       getenvFloat("FANCTL_KD", 1.0),
		Setpoint: getenvFloat("FANCTL_SETPOINT", 55.0), // °C

		DryRun: getenvBool("FANCTL_DRY_RUN", false),
		Debug:  getenvBool("FANCTL_DEBUG", false),
	}

	intervalSec := getenvFloat("FANCTL_INTERVAL_SEC", 1.0)
	if intervalSec <= 0 {
		return nil, fmt.Errorf("FANCTL_INTERVAL_SEC must be > 0, got %v", intervalSec)
	}
	cfg.Interval = time.Duration(intervalSec * float64(time.Second))
	cfg.PIDInterval = intervalSec

	switch cfg.ControlMode {
	case ModePID, ModeFixed, ModeTable:
		// valid
	default:
		return nil, fmt.Errorf("unknown control mode %q; valid: pid, fixed, table", cfg.ControlMode)
	}

	if cfg.MinPWM < 0 || cfg.MinPWM > 255 {
		return nil, fmt.Errorf("FANCTL_MIN_PWM must be 0–255, got %d", cfg.MinPWM)
	}
	if cfg.MaxPWM < 0 || cfg.MaxPWM > 255 {
		return nil, fmt.Errorf("FANCTL_MAX_PWM must be 0–255, got %d", cfg.MaxPWM)
	}
	if cfg.MinPWM > cfg.MaxPWM {
		return nil, fmt.Errorf("FANCTL_MIN_PWM (%d) must be <= FANCTL_MAX_PWM (%d)", cfg.MinPWM, cfg.MaxPWM)
	}

	return cfg, nil
}

// ---- helpers ---------------------------------------------------------------

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getenvFloat(key string, fallback float64) float64 {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func getenvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}