// ===== file: internal/control/loop.go =====

// Package control implements the main fan control loop and the mode
// dispatch layer.  Adding a new control mode only requires implementing
// the controller interface and registering it in newController.
package control

import (
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/shizzz/openwrt-fancontrol/internal/config"
	"github.com/shizzz/openwrt-fancontrol/internal/log"
	"github.com/shizzz/openwrt-fancontrol/internal/pid"
	"github.com/shizzz/openwrt-fancontrol/internal/sysfs"
)

// controller is the interface every control mode must satisfy.
type controller interface {
	// Compute receives the current temperature (°C) and returns the desired
	// PWM value before safety clamping.
	Compute(tempC float64) (pwm float64, terms pid.Terms, err error)
	// Reset clears accumulated state (called on sensor read errors).
	Reset()
}

// Loop is the top-level control loop object.
type Loop struct {
	cfg  *config.Config
	log  *log.Logger
	ctrl controller
	stop atomic.Bool
}

// NewLoop constructs a Loop and initialises the selected controller.
func NewLoop(cfg *config.Config, logger *log.Logger) *Loop {
	return &Loop{
		cfg:  cfg,
		log:  logger,
		ctrl: newController(cfg),
	}
}

// Stop signals the loop to exit after the current iteration finishes.
// Safe to call from any goroutine.
func (l *Loop) Stop() {
	l.stop.Store(true)
}

// Run executes the control loop until Stop is called or an unrecoverable
// error occurs.  It returns nil on clean shutdown.
func (l *Loop) Run() error {
	cfg := l.cfg

	// Attempt to enable PWM manual mode once at startup.
	if !cfg.DryRun {
		if err := sysfs.EnablePWMManual(cfg.PWMEnablePath); err != nil {
			l.log.Warnf("could not enable PWM manual mode: %v (continuing)", err)
		}
	}

	// Warn if hardware nodes are missing so the operator knows immediately.
	if !sysfs.NodeExists(cfg.ThermalZonePath) {
		l.log.Warnf("thermal node %q does not exist; reads will fail until it appears", cfg.ThermalZonePath)
	}
	if !sysfs.NodeExists(cfg.PWMPath) {
		l.log.Warnf("pwm node %q does not exist; writes will fail until it appears", cfg.PWMPath)
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	l.log.Infof("control loop started (mode=%s interval=%s setpoint=%.1f°C)",
		cfg.ControlMode, cfg.Interval, cfg.Setpoint)

	for {
		if l.stop.Load() {
			return nil
		}
		select {
		case <-ticker.C:
			l.iterate()
		}
	}
}

// iterate executes one control cycle: read → compute → clamp → write → log.
func (l *Loop) iterate() {
	cfg := l.cfg

	// 1. Read temperature
	tempC, err := sysfs.ReadTemperatureCelsius(cfg.ThermalZonePath)
	if err != nil {
		l.log.Errorf("temperature read failed: %v; holding last PWM output", err)
		// Reset PID state so derivative/integral don't accumulate stale data.
		l.ctrl.Reset()
		return
	}

	// 2. Compute desired PWM via the selected controller
	rawPWM, terms, err := l.ctrl.Compute(tempC)
	if err != nil {
		l.log.Errorf("controller error: %v", err)
		return
	}

	// 3. Safety clamp to configured [MinPWM, MaxPWM]
	pwm := clampFloat(rawPWM, float64(cfg.MinPWM), float64(cfg.MaxPWM))
	pwmInt := int(math.Round(pwm))

	// 4. Write PWM (skip in dry-run mode)
	if cfg.DryRun {
		l.log.Infof("[DRY-RUN] temp=%.2f°C pwm=%d (raw=%.2f)", tempC, pwmInt, rawPWM)
	} else {
		if werr := sysfs.WritePWM(cfg.PWMPath, pwmInt); werr != nil {
			l.log.Errorf("PWM write failed: %v", werr)
			// Non-fatal: keep running; next iteration will retry.
		}
	}

	// 5. Log status
	l.log.Infof("temp=%.2f°C pwm=%d setpoint=%.1f°C mode=%s",
		tempC, pwmInt, cfg.Setpoint, cfg.ControlMode)

	l.log.Debugf("PID terms  P=%.4f  I=%.4f  D=%.4f  raw=%.4f",
		terms.P, terms.I, terms.D, terms.Output)
}

// ---- mode factory -----------------------------------------------------------

func newController(cfg *config.Config) controller {
	switch cfg.ControlMode {
	case config.ModePID:
		return newPIDController(cfg)
	case config.ModeFixed:
		return newFixedController(cfg)
	case config.ModeTable:
		return newTableController(cfg)
	default:
		// Should never reach here; config.Load validates the mode.
		panic(fmt.Sprintf("unknown control mode: %s", cfg.ControlMode))
	}
}

// ---- PID mode ---------------------------------------------------------------

type pidController struct {
	params pid.Params
	state  *pid.State
}

func newPIDController(cfg *config.Config) *pidController {
	return &pidController{
		params: pid.Params{
			Kp:       cfg.Kp,
			Ki:       cfg.Ki,
			Kd:       cfg.Kd,
			Setpoint: cfg.Setpoint,
			OutMin:   float64(cfg.MinPWM),
			OutMax:   float64(cfg.MaxPWM),
			Dt:       cfg.PIDInterval,
		},
		state: pid.NewState(),
	}
}

func (c *pidController) Compute(tempC float64) (float64, pid.Terms, error) {
	terms := pid.Compute(&c.params, c.state, tempC)
	return terms.Output, terms, nil
}

func (c *pidController) Reset() {
	c.state.Reset()
}

// ---- Fixed mode (stub) ------------------------------------------------------

type fixedController struct {
	pwm float64
}

func newFixedController(cfg *config.Config) *fixedController {
	return &fixedController{pwm: float64(cfg.FixedPWM)}
}

func (c *fixedController) Compute(_ float64) (float64, pid.Terms, error) {
	return c.pwm, pid.Terms{Output: c.pwm}, nil
}

func (c *fixedController) Reset() {}

// ---- Table mode (stub) ------------------------------------------------------
// Future implementation: linear interpolation over a []TempPWMPoint table.
// The table would be loaded from UCI config or a JSON file.

type tableController struct {
	cfg *config.Config
}

func newTableController(cfg *config.Config) *tableController {
	return &tableController{cfg: cfg}
}

func (c *tableController) Compute(tempC float64) (float64, pid.Terms, error) {
	// Stub: linearly map [Setpoint-10, Setpoint+10] → [MinPWM, MaxPWM].
	// Replace with a proper interpolation table loaded from config.
	lo := c.cfg.Setpoint - 10.0
	hi := c.cfg.Setpoint + 10.0
	t := (tempC - lo) / (hi - lo)
	t = clampFloat(t, 0, 1)
	pwm := float64(c.cfg.MinPWM) + t*float64(c.cfg.MaxPWM-c.cfg.MinPWM)
	return pwm, pid.Terms{Output: pwm}, nil
}

func (c *tableController) Reset() {}

// ---- helpers ----------------------------------------------------------------

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}