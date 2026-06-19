package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeUCI is a test helper that writes content to a temp UCI file and
// returns its path.
func writeUCI(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fancontrol")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write uci fixture: %v", err)
	}
	return path
}

// unsetAll clears every FANCTL_* env var this package reads so a test
// starts from a known-clean state.
func unsetAll(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"FANCTL_ENABLED",
		"FANCTL_THERMAL_PATH", "FANCTL_PWM_PATH", "FANCTL_PWM_ENABLE_PATH",
		"FANCTL_MODE", "FANCTL_KP", "FANCTL_KI", "FANCTL_KD",
		"FANCTL_SETPOINT", "FANCTL_MIN_PWM", "FANCTL_MAX_PWM",
		"FANCTL_FIXED_PWM", "FANCTL_INTERVAL_SEC",
		"FANCTL_DRY_RUN", "FANCTL_DEBUG",
	} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

// ---- parser -----------------------------------------------------------------

func TestParseUCISingleSection(t *testing.T) {
	path := writeUCI(t, `
# Top-level comment
config fancontrol 'main'
	option mode 'pid'
	option setpoint '50'
	option kp '1.5'
	option dry_run '1'
`)

	sections, err := parseUCISections(path)
	if err != nil {
		t.Fatalf("parseUCISections: %v", err)
	}
	if len(sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(sections))
	}
	got := sections[0]
	if got.Type != "fancontrol" || got.Name != "main" {
		t.Errorf("section header: got %q/%q, want fancontrol/main", got.Type, got.Name)
	}
	if got.Options["mode"] != "pid" {
		t.Errorf("mode = %q", got.Options["mode"])
	}
	if got.Options["setpoint"] != "50" {
		t.Errorf("setpoint = %q", got.Options["setpoint"])
	}
}

func TestParseUCIMultipleSections(t *testing.T) {
	path := writeUCI(t, `
config fancontrol 'cpu'
	option setpoint '55'

config fancontrol 'case'
	option setpoint '40'
	option kp '3.0'

config firewall 'lan'
	option name 'lan'
`)

	sections, err := parseUCISections(path)
	if err != nil {
		t.Fatalf("parseUCISections: %v", err)
	}
	if len(sections) != 3 {
		t.Fatalf("got %d sections, want 3", len(sections))
	}
	if sections[0].Type != "fancontrol" || sections[0].Name != "cpu" {
		t.Errorf("sections[0]: %q/%q", sections[0].Type, sections[0].Name)
	}
	if sections[1].Options["kp"] != "3.0" {
		t.Errorf("case kp = %q", sections[1].Options["kp"])
	}
	if sections[2].Type != "firewall" {
		t.Errorf("third section type = %q, want firewall (cross-section contamination check)", sections[2].Type)
	}
	if _, leaked := sections[2].Options["setpoint"]; leaked {
		t.Errorf("option from previous section leaked into firewall: %v", sections[2].Options)
	}
}

func TestParseUCIUnquotedValue(t *testing.T) {
	path := writeUCI(t, `
config fancontrol 'main'
	option mode fixed
	option debug 1
`)

	sections, err := parseUCISections(path)
	if err != nil {
		t.Fatalf("parseUCISections: %v", err)
	}
	if sections[0].Options["mode"] != "fixed" {
		t.Errorf("mode = %q, want fixed", sections[0].Options["mode"])
	}
	if sections[0].Options["debug"] != "1" {
		t.Errorf("debug = %q, want 1", sections[0].Options["debug"])
	}
}

func TestParseUCIMissingFile(t *testing.T) {
	sections, err := parseUCISections(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if sections != nil {
		t.Fatalf("expected nil sections for missing file, got %v", sections)
	}
}

func TestParseUCICommentsAndBlankLines(t *testing.T) {
	path := writeUCI(t, `

# full-line comment
   # indented comment

config fancontrol 'main'
	option setpoint '60'
`)
	sections, err := parseUCISections(path)
	if err != nil {
		t.Fatalf("parseUCISections: %v", err)
	}
	if len(sections) != 1 || sections[0].Options["setpoint"] != "60" {
		t.Errorf("got %+v", sections)
	}
}

// ---- loadAt dispatch ---------------------------------------------------------

func TestLoadAt_UCIExistsEnvIgnored(t *testing.T) {
	unsetAll(t)
	t.Setenv("FANCTL_SETPOINT", "99")
	t.Setenv("FANCTL_MODE", "fixed")

	path := writeUCI(t, `
config fancontrol 'cpu'
	option setpoint '55'
	option mode 'pid'
`)

	cfg, err := loadAt(path)
	if err != nil {
		t.Fatalf("loadAt: %v", err)
	}
	if len(cfg.Fans) != 1 {
		t.Fatalf("got %d fans, want 1", len(cfg.Fans))
	}
	if cfg.Fans[0].Setpoint != 55 {
		t.Errorf("setpoint: got %v, want 55 (UCI should beat env 99)", cfg.Fans[0].Setpoint)
	}
	if cfg.Fans[0].Mode != ModePID {
		t.Errorf("mode: got %v, want pid", cfg.Fans[0].Mode)
	}
}

func TestLoadAt_NoUCIFallsBackToEnv(t *testing.T) {
	unsetAll(t)
	t.Setenv("FANCTL_SETPOINT", "42")
	t.Setenv("FANCTL_MODE", "fixed")
	t.Setenv("FANCTL_KP", "9.5")

	cfg, err := loadAt(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("loadAt: %v", err)
	}
	if len(cfg.Fans) != 1 {
		t.Fatalf("got %d fans, want 1 (env fallback)", len(cfg.Fans))
	}
	if cfg.Fans[0].Name != "default" {
		t.Errorf("fan name: got %q, want default", cfg.Fans[0].Name)
	}
	if cfg.Fans[0].Setpoint != 42 {
		t.Errorf("env setpoint: got %v, want 42", cfg.Fans[0].Setpoint)
	}
	if cfg.Fans[0].Mode != ModeFixed {
		t.Errorf("env mode: got %v, want fixed", cfg.Fans[0].Mode)
	}
	if cfg.Fans[0].Kp != 9.5 {
		t.Errorf("env kp: got %v, want 9.5", cfg.Fans[0].Kp)
	}
}

func TestLoadAt_NoUCINoEnvUsesDefaults(t *testing.T) {
	unsetAll(t)

	cfg, err := loadAt(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("loadAt: %v", err)
	}
	if len(cfg.Fans) != 1 {
		t.Fatalf("got %d fans, want 1", len(cfg.Fans))
	}
	if cfg.Fans[0].Setpoint != 55.0 {
		t.Errorf("default setpoint: got %v, want 55", cfg.Fans[0].Setpoint)
	}
	if cfg.Fans[0].Mode != ModePID {
		t.Errorf("default mode: got %v, want pid", cfg.Fans[0].Mode)
	}
	if cfg.Fans[0].Interval.Seconds() != 1.0 {
		t.Errorf("default interval: got %v, want 1s", cfg.Fans[0].Interval)
	}
}

// ---- multi-fan ---------------------------------------------------------------

func TestLoadAt_UCI_MultipleFans(t *testing.T) {
	unsetAll(t)
	path := writeUCI(t, `
config fancontrol 'cpu'
	option setpoint '60'
	option kp '3.0'

config fancontrol 'case'
	option setpoint '45'
	option thermal_path '/sys/class/thermal/thermal_zone1/temp'
	option pwm_path '/sys/class/hwmon/hwmon1/pwm2'
	option pwm_enable_path '/sys/class/hwmon/hwmon1/pwm2_enable'
`)

	cfg, err := loadAt(path)
	if err != nil {
		t.Fatalf("loadAt: %v", err)
	}
	if len(cfg.Fans) != 2 {
		t.Fatalf("got %d fans, want 2", len(cfg.Fans))
	}
	if cfg.Fans[0].Name != "cpu" || cfg.Fans[0].Setpoint != 60 {
		t.Errorf("fan[0]: %+v", cfg.Fans[0])
	}
	if cfg.Fans[1].Name != "case" || cfg.Fans[1].Setpoint != 45 {
		t.Errorf("fan[1]: %+v", cfg.Fans[1])
	}
	if cfg.Fans[1].ThermalPath != "/sys/class/thermal/thermal_zone1/temp" {
		t.Errorf("fan[1] thermal path: %q", cfg.Fans[1].ThermalPath)
	}
}

func TestLoadAt_UCI_DisabledFanFiltered(t *testing.T) {
	unsetAll(t)
	path := writeUCI(t, `
config fancontrol 'cpu'
	option setpoint '60'

config fancontrol 'case'
	option enabled '0'
	option setpoint '45'

config fancontrol 'extra'
	option enabled '1'
	option setpoint '50'
`)

	cfg, err := loadAt(path)
	if err != nil {
		t.Fatalf("loadAt: %v", err)
	}
	if len(cfg.Fans) != 2 {
		t.Fatalf("got %d fans, want 2 (case disabled)", len(cfg.Fans))
	}
	names := []string{cfg.Fans[0].Name, cfg.Fans[1].Name}
	if names[0] != "cpu" || names[1] != "extra" {
		t.Errorf("fan names: got %v, want [cpu extra]", names)
	}
}

func TestLoadAt_UCI_AllDisabledReturnsError(t *testing.T) {
	unsetAll(t)
	path := writeUCI(t, `
config fancontrol 'cpu'
	option enabled '0'

config fancontrol 'case'
	option enabled '0'
`)

	_, err := loadAt(path)
	if err == nil {
		t.Fatal("expected error when all fans are disabled")
	}
}

func TestLoadAt_UCI_NoFancontrolSections(t *testing.T) {
	unsetAll(t)
	path := writeUCI(t, `
config firewall 'lan'
	option name 'lan'

config dhcp 'dnsmasq'
	option domain 'lan'
`)

	_, err := loadAt(path)
	if err == nil {
		t.Fatal("expected error when no fancontrol sections exist")
	}
}

// ---- validation --------------------------------------------------------------

func TestLoadAt_UCI_TypeError(t *testing.T) {
	unsetAll(t)
	path := writeUCI(t, `
config fancontrol 'cpu'
	option setpoint 'not-a-number'
`)
	if _, err := loadAt(path); err == nil {
		t.Fatal("expected error for non-numeric setpoint")
	}
}

func TestLoadAt_UCI_UnknownMode(t *testing.T) {
	unsetAll(t)
	path := writeUCI(t, `
config fancontrol 'cpu'
	option mode 'turbo'
`)
	if _, err := loadAt(path); err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func TestLoadAt_UCI_BadEnabledValue(t *testing.T) {
	unsetAll(t)
	path := writeUCI(t, `
config fancontrol 'cpu'
	option enabled 'maybe'
`)
	if _, err := loadAt(path); err == nil {
		t.Fatal("expected error for non-bool enabled")
	}
}

func TestLoadAt_UCI_MinGTMax(t *testing.T) {
	unsetAll(t)
	path := writeUCI(t, `
config fancontrol 'cpu'
	option min_pwm '200'
	option max_pwm '100'
`)
	if _, err := loadAt(path); err == nil {
		t.Fatal("expected error for min_pwm > max_pwm")
	}
}

func TestLoadAt_UCI_MissingPath(t *testing.T) {
	unsetAll(t)
	path := writeUCI(t, `
config fancontrol 'cpu'
	option pwm_path ''
`)
	if _, err := loadAt(path); err == nil {
		t.Fatal("expected error for empty pwm_path")
	}
}