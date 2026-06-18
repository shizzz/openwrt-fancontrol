// Package pid implements a discrete-time PID controller suitable for
// fan speed regulation on embedded Linux systems.
//
// Design choices for stability:
//   - Derivative is computed on the measured value (not the error) so that
//     a setpoint step change does not produce a derivative kick.
//   - Anti-windup via output clamping: the integral candidate is adjusted
//     before being committed so that P + I + D never exceeds the actuator
//     range, and the integrator itself is hard-clamped to [OutMin, OutMax].
//   - First-order derivative filter reduces noise amplification.  The
//     filter coefficient N controls the cut-off (larger N = less filtering).
package pid

import "math"

const (
	// derivativeFilterN is the filter coefficient for the derivative term.
	// N=10 is a common choice for fan control; raise it to reduce lag,
	// lower it to reduce noise sensitivity.
	derivativeFilterN = 10.0
)

// Params holds tunable PID parameters.  All fields are exported so callers
// can update them at runtime without restarting the controller.
type Params struct {
	Kp float64 // proportional gain
	Ki float64 // integral gain
	Kd float64 // derivative gain

	Setpoint float64 // desired temperature in °C

	OutMin float64 // lower clamp on controller output (PWM)
	OutMax float64 // upper clamp on controller output (PWM)

	Dt float64 // sample interval in seconds
}

// State holds the internal state of a running PID controller.
// Initialise with NewState; do not copy after first use.
type State struct {
	integral      float64 // accumulated integral term
	prevMeasured  float64 // measured value at previous sample
	derivFiltered float64 // low-pass filtered derivative
	firstSample   bool    // true until the first Compute call
}

// NewState returns a zeroed controller state ready for the first sample.
func NewState() *State {
	return &State{firstSample: true}
}

// Reset clears accumulated state (integral windup, derivative history).
// Call when the setpoint changes significantly or after a long pause.
func (s *State) Reset() {
	s.integral = 0
	s.prevMeasured = 0
	s.derivFiltered = 0
	s.firstSample = true
}

// Terms holds the individual PID contributions for logging / diagnostics.
type Terms struct {
	P      float64
	I      float64
	D      float64
	Output float64 // clamped final output
}

// Compute runs one PID iteration given the current measured value and the
// controller parameters.  It returns the output (clamped to [OutMin,OutMax])
// and the individual terms for diagnostic logging.
func Compute(p *Params, s *State, measured float64) Terms {
	err := p.Setpoint - measured

	// ---- Proportional -------------------------------------------------------
	pTerm := p.Kp * err

	// ---- Derivative on measurement with first-order low-pass filter --------
	// Derivative on the measurement (not the error) avoids a derivative kick
	// when the setpoint changes.
	var dTerm float64
	if s.firstSample {
		// No previous sample: derivative is undefined; treat as zero.
		s.derivFiltered = 0
		s.firstSample = false
	} else {
		rawDeriv := -(measured - s.prevMeasured) / p.Dt
		// First-order filter:  y[k] = α·y[k-1] + (1-α)·x[k]
		// where α = N·Dt / (1 + N·Dt) maps the continuous pole at N rad/s.
		alpha := derivativeFilterN * p.Dt / (1.0 + derivativeFilterN*p.Dt)
		s.derivFiltered = alpha*s.derivFiltered + (1.0-alpha)*rawDeriv
	}
	dTerm = p.Kd * s.derivFiltered
	s.prevMeasured = measured

	// ---- Integral with clamping anti-windup ---------------------------------
	// Compute the integral candidate, then constrain it so that
	// P + I + D does not exceed the output range.  Hard-clamp the result
	// to [OutMin, OutMax] as a final safety net.
	iCandidate := s.integral + p.Ki*err*p.Dt
	unsaturated := pTerm + iCandidate + dTerm
	switch {
	case unsaturated > p.OutMax:
		iCandidate = p.OutMax - pTerm - dTerm
	case unsaturated < p.OutMin:
		iCandidate = p.OutMin - pTerm - dTerm
	}
	s.integral = math.Max(p.OutMin, math.Min(p.OutMax, iCandidate))
	iTerm := s.integral

	// ---- Sum and clamp output -----------------------------------------------
	output := math.Max(p.OutMin, math.Min(p.OutMax, pTerm+iTerm+dTerm))

	return Terms{
		P:      pTerm,
		I:      iTerm,
		D:      dTerm,
		Output: output,
	}
}
