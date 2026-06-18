// ===== file: internal/pid/pid_test.go =====

package pid_test

import (
	"math"
	"testing"

	"github.com/openwr-fancontrol/internal/pid"
)

func defaultParams() pid.Params {
	return pid.Params{
		Kp:       2.0,
		Ki:       0.5,
		Kd:       1.0,
		Setpoint: 55.0,
		OutMin:   0,
		OutMax:   255,
		Dt:       1.0,
	}
}

// TestPIDOutputClampedAboveMax verifies that output never exceeds OutMax
// even when the error is very large.
func TestPIDOutputClampedAboveMax(t *testing.T) {
	p := defaultParams()
	s := pid.NewState()
	// Temperature far below setpoint → large positive error → high output
	for i := 0; i < 10; i++ {
		terms := pid.Compute(&p, s, 0.0)
		if terms.Output > p.OutMax {
			t.Fatalf("iteration %d: output %.2f exceeds OutMax %.2f", i, terms.Output, p.OutMax)
		}
	}
}

// TestPIDOutputClampedBelowMin verifies that output never drops below OutMin
// even when the temperature is far above the setpoint.
func TestPIDOutputClampedBelowMin(t *testing.T) {
	p := defaultParams()
	s := pid.NewState()
	// Temperature far above setpoint → large negative error → low output
	for i := 0; i < 10; i++ {
		terms := pid.Compute(&p, s, 200.0)
		if terms.Output < p.OutMin {
			t.Fatalf("iteration %d: output %.2f below OutMin %.2f", i, terms.Output, p.OutMin)
		}
	}
}

// TestPIDConvergesToSetpoint checks that the controller drives the simulated
// system toward the setpoint over time using a simple first-order plant model.
func TestPIDConvergesToSetpoint(t *testing.T) {
	p := defaultParams()
	s := pid.NewState()

	// Simple thermal model: temp changes proportionally to PWM cooling effect.
	temp := 70.0 // start above setpoint
	for i := 0; i < 200; i++ {
		terms := pid.Compute(&p, s, temp)
		// Simplified plant: cooling proportional to PWM, heating is constant.
		cooling := terms.Output / 255.0 * 2.0 // max 2°C/s cooling
		heating := 0.5                          // 0.5°C/s ambient heating
		temp += (heating - cooling) * p.Dt
	}

	if math.Abs(temp-p.Setpoint) > 3.0 {
		t.Fatalf("temperature did not converge: final=%.2f setpoint=%.2f", temp, p.Setpoint)
	}
}

// TestPIDResetClearsIntegral verifies that Reset() zeroes out accumulated state
// so a restarted controller behaves like a fresh one.  For defaultParams and
// measured=40 the first call after Reset/NewState is deterministic:
//   err=15, pTerm=30, iTerm=7.5, dTerm=0 → output=37.5
func TestPIDResetClearsIntegral(t *testing.T) {
	p := defaultParams()
	s := pid.NewState()

	for i := 0; i < 20; i++ {
		pid.Compute(&p, s, 40.0)
	}

	s.Reset()
	terms := pid.Compute(&p, s, 40.0)

	const want = 37.5
	if terms.Output != want {
		t.Fatalf("after Reset, output %.4f != expected %.4f", terms.Output, want)
	}
}

// TestPIDAntiWindup confirms that the integral does not accumulate beyond the
// output bounds even under sustained saturation.
func TestPIDAntiWindup(t *testing.T) {
	p := defaultParams()
	p.OutMax = 50 // tight ceiling to force saturation quickly
	s := pid.NewState()

	for i := 0; i < 100; i++ {
		terms := pid.Compute(&p, s, 0.0) // huge positive error
		if terms.Output > p.OutMax+1e-9 {
			t.Fatalf("iteration %d: saturated output %.4f > OutMax %.4f", i, terms.Output, p.OutMax)
		}
	}
}