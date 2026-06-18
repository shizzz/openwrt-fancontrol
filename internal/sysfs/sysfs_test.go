// ===== file: internal/sysfs/sysfs_test.go =====

package sysfs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shizzz/openwrt-fancontrol/internal/sysfs"
)

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeTempFile: %v", err)
	}
	return path
}

func TestReadTemperatureCelsius(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "temp", "55000\n")

	got, err := sysfs.ReadTemperatureCelsius(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 55.0 {
		t.Fatalf("expected 55.0, got %.4f", got)
	}
}

func TestReadTemperatureMissingFile(t *testing.T) {
	_, err := sysfs.ReadTemperatureCelsius("/nonexistent/thermal_zone99/temp")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestReadTemperatureInvalidContent(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "temp", "notanumber\n")
	_, err := sysfs.ReadTemperatureCelsius(path)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestWriteReadPWM(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "pwm1", "0\n")

	if err := sysfs.WritePWM(path, 128); err != nil {
		t.Fatalf("WritePWM: %v", err)
	}
	got, err := sysfs.ReadPWM(path)
	if err != nil {
		t.Fatalf("ReadPWM: %v", err)
	}
	if got != 128 {
		t.Fatalf("expected 128, got %d", got)
	}
}

func TestWritePWMOutOfRange(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "pwm1", "0\n")
	if err := sysfs.WritePWM(path, 300); err == nil {
		t.Fatal("expected error for out-of-range value, got nil")
	}
}

func TestNodeExists(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "node", "1\n")

	if !sysfs.NodeExists(path) {
		t.Fatal("NodeExists returned false for existing file")
	}
	if sysfs.NodeExists("/nonexistent/path/xyz") {
		t.Fatal("NodeExists returned true for non-existent path")
	}
}