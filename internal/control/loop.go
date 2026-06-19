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

// Loop drives a single fan.  One Loop instance per fan; the caller is
// responsible for running them concurrently.
type Loop struct {
	fan  *config.FanConfig
	log  *log.Logger
	ctrl controller
	stop atomic.Bool
}

// NewLoop constructs a Loop for the given fan configuration.
func NewLoop(fan *config.FanConfig, logger *log.Logger) *Loop {
	return &Loop{
		fan:  fan,
		log:  logger,
		ctrl: newController(fan),
	}
}

// Name returns the fan's identifier (UCI section name or "default").
func (l *Loop) Name() string { return l.fan.Name }

// Stop signals the loop to exit after the current iteration finishes.
// Safe to call from any goroutine.
func (l *Loop) Stop() {
	l.stop.Store(true)
}

// Run executes the control loop until Stop is called or an unrecoverable
// error occurs.  It returns nil on clean shutdown.
func (l *Loop) Run() error {
	fan := l.fan
	tag := "[" + fan.Name + "]"

	if !fan.DryRun {
		if err := sysfs.EnablePWMManual(fan.PWMEnablePath); err != nil {
			l.log.Warnf("%s could not enable PWM manual mode: %v (continuing)", tag, err)
		}
	}

	if !sysfs.NodeExists(fan.ThermalPath) {
		l.log.Warnf("%s thermal node %q does not exist; reads will fail until it appears", tag, fan.ThermalPath)
	}
	if !sysfs.NodeExists(fan.PWMPath) {
		l.log.Warnf("%s pwm node %q does not exist; writes will fail until it appears", tag, fan.PWMPath)
	}

	ticker := time.NewTicker(fan.Interval)
	defer ticker.Stop()

	l.log.Infof("%s loop started (mode=%s interval=%s setpoint=%.1f°C)",
		tag, fan.Mode, fan.Interval, fan.Setpoint)

	for {
		if l.stop.Load() {
			return nil
		}
		<-ticker.C
		l.iterate()
	}
}

// iterate executes one control cycle: read → compute → clamp → write → log.
func (l *Loop) iterate() {
	fan := l.fan
	tag := "[" + fan.Name + "]"

	// 1. Read temperature
	tempC, err := sysfs.ReadTemperatureCelsius(fan.ThermalPath)
	if err != nil {
		l.log.Errorf("%s temperature read failed: %v; holding last PWM output", tag, err)
		l.ctrl.Reset()
		return
	}

	// 2. Compute desired PWM via the selected controller
	rawPWM, terms, err := l.ctrl.Compute(tempC)
	if err != nil {
		l.log.Errorf("%s controller error: %v", tag, err)
		return
	}

	// 3. Safety clamp to configured [MinPWM, MaxPWM]
	pwm := clampFloat(rawPWM, float64(fan.MinPWM), float64(fan.MaxPWM))
	pwmInt := int(math.Round(pwm))

	// 4. Write PWM (skip in dry-run mode)
	if fan.DryRun {
		l.log.Infof("%s [DRY-RUN] temp=%.2f°C pwm=%d (raw=%.2f)", tag, tempC, pwmInt, rawPWM)
	} else {
		if werr := sysfs.WritePWM(fan.PWMPath, pwmInt); werr != nil {
			l.log.Errorf("%s PWM write failed: %v", tag, werr)
			// Non-fatal: keep running; next iteration will retry.
		}
	}

	// 5. Log status
	l.log.Infof("%s temp=%.2f°C pwm=%d setpoint=%.1f°C mode=%s",
		tag, tempC, pwmInt, fan.Setpoint, fan.Mode)

	l.log.Debugf("%s PID terms  P=%.4f  I=%.4f  D=%.4f  raw=%.4f",
		tag, terms.P, terms.I, terms.D, terms.Output)
}

// ---- mode factory -----------------------------------------------------------

func newController(fan *config.FanConfig) controller {
	switch fan.Mode {
	case config.ModePID:
		return newPIDController(fan)
	case config.ModeFixed:
		return newFixedController(fan)
	case config.ModeTable:
		return newTableController(fan)
	default:
		// Should never reach here; config.Load validates the mode.
		panic(fmt.Sprintf("unknown control mode: %s", fan.Mode))
	}
}

// ---- PID mode ---------------------------------------------------------------

type pidController struct {
	params pid.Params
	state  *pid.State
}

func newPIDController(fan *config.FanConfig) *pidController {
	return &pidController{
		params: pid.Params{
			Kp:       fan.Kp,
			Ki:       fan.Ki,
			Kd:       fan.Kd,
			Setpoint: fan.Setpoint,
			OutMin:   float64(fan.MinPWM),
			OutMax:   float64(fan.MaxPWM),
			Dt:       fan.Interval.Seconds(),
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

func newFixedController(fan *config.FanConfig) *fixedController {
	return &fixedController{pwm: float64(fan.FixedPWM)}
}

func (c *fixedController) Compute(_ float64) (float64, pid.Terms, error) {
	return c.pwm, pid.Terms{Output: c.pwm}, nil
}

func (c *fixedController) Reset() {}

// ---- Table mode (stub) ------------------------------------------------------
// Future implementation: linear interpolation over a []TempPWMPoint table.
// The table would be loaded from UCI config or a JSON file.

type tableController struct {
	fan *config.FanConfig
}

func newTableController(fan *config.FanConfig) *tableController {
	return &tableController{fan: fan}
}

func (c *tableController) Compute(tempC float64) (float64, pid.Terms, error) {
	// Stub: linearly map [Setpoint-10, Setpoint+10] → [MinPWM, MaxPWM].
	// Replace with a proper interpolation table loaded from config.
	lo := c.fan.Setpoint - 10.0
	hi := c.fan.Setpoint + 10.0
	t := (tempC - lo) / (hi - lo)
	t = clampFloat(t, 0, 1)
	pwm := float64(c.fan.MinPWM) + t*float64(c.fan.MaxPWM-c.fan.MinPWM)
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