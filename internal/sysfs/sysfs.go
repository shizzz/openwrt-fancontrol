// ===== file: internal/sysfs/sysfs.go =====

// Package sysfs provides safe, error-tolerant access to Linux sysfs nodes
// used for thermal sensors and PWM fan control.
package sysfs

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ReadTemperatureCelsius reads a sysfs thermal zone temperature file.
// The kernel exposes temperatures in millidegrees Celsius; this function
// converts to degrees Celsius.
// Returns an error if the file cannot be read or parsed, but never panics.
func ReadTemperatureCelsius(path string) (float64, error) {
	raw, err := readFile(path)
	if err != nil {
		return 0, fmt.Errorf("thermal read %q: %w", path, err)
	}
	millideg, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("thermal parse %q value=%q: %w", path, raw, err)
	}
	return float64(millideg) / 1000.0, nil
}

// ReadPWM reads the current PWM value (0–255) from a sysfs pwm node.
func ReadPWM(path string) (int, error) {
	raw, err := readFile(path)
	if err != nil {
		return 0, fmt.Errorf("pwm read %q: %w", path, err)
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("pwm parse %q value=%q: %w", path, raw, err)
	}
	return val, nil
}

// WritePWM writes a PWM value (0–255) to a sysfs pwm node.
// The value must already be clamped by the caller.
func WritePWM(path string, value int) error {
	if value < 0 || value > 255 {
		return fmt.Errorf("pwm value %d out of range [0,255]", value)
	}
	data := strconv.Itoa(value) + "\n"
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		return fmt.Errorf("pwm write %q: %w", path, err)
	}
	return nil
}

// EnablePWMManual writes "1" to the pwm_enable sysfs node to switch the fan
// controller to manual (software) mode.  Many hwmon drivers require this
// before accepting PWM writes; absence of the file is silently ignored.
func EnablePWMManual(enablePath string) error {
	if _, err := os.Stat(enablePath); os.IsNotExist(err) {
		// Node is optional; not all drivers expose it.
		return nil
	}
	if err := os.WriteFile(enablePath, []byte("1\n"), 0644); err != nil {
		return fmt.Errorf("pwm_enable write %q: %w", enablePath, err)
	}
	return nil
}

// NodeExists returns true if the sysfs path is accessible.
func NodeExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ---- internal helpers -------------------------------------------------------

func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}