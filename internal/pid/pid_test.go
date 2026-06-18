package pid_test

import (
	"math"
	"testing"

	"github.com/shizzz/openwrt-fancontrol/internal/pid"
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

// TestPIDOutputClampedAboveMax verifies that the upper output clamp holds
// when the temperature is well above the setpoint (large positive err
// drives output to OutMax).
func TestPIDOutputClampedAboveMax(t *testing.T) {
	p := defaultParams()
	s := pid.NewState()
	for i := 0; i < 10; i++ {
		terms := pid.Compute(&p, s, 200.0)
		if terms.Output > p.OutMax {
			t.Fatalf("iteration %d: output %.2f exceeds OutMax %.2f", i, terms.Output, p.OutMax)
		}
	}
}

// TestPIDOutputClampedBelowMin verifies that the lower output clamp holds
// when the temperature is well below the setpoint (large negative err
// drives output to OutMin).
func TestPIDOutputClampedBelowMin(t *testing.T) {
	p := defaultParams()
	s := pid.NewState()
	for i := 0; i < 10; i++ {
		terms := pid.Compute(&p, s, 0.0)
		if terms.Output < p.OutMin {
			t.Fatalf("iteration %d: output %.2f below OutMin %.2f", i, terms.Output, p.OutMin)
		}
	}
}

// TestPIDConvergesToSetpoint checks that the controller drives a simulated
// thermal system toward the setpoint over time.
//
// Plant: ambient heat at 0.5°C/s, fan cooling proportional to PWM up to
// 2°C/s.  With err = measured - setpoint the controller correctly cools
// when temp > setpoint.  Starting at 70°C (above setpoint 55) exercises
// the full proportional + integral + derivative path.
func TestPIDConvergesToSetpoint(t *testing.T) {
	p := defaultParams()
	s := pid.NewState()

	temp := 70.0
	for i := 0; i < 300; i++ {
		terms := pid.Compute(&p, s, temp)
		cooling := terms.Output / 255.0 * 2.0
		heating := 0.5
		temp += (heating - cooling) * p.Dt
	}

	if math.Abs(temp-p.Setpoint) > 3.0 {
		t.Fatalf("temperature did not converge: final=%.2f setpoint=%.2f", temp, p.Setpoint)
	}
}

// TestPIDResetClearsIntegral verifies that Reset() zeroes out accumulated
// state so a restarted controller behaves like a fresh one.  For
// defaultParams and measured=80 the first call after Reset/NewState is
// deterministic: err=25, pTerm=50, iTerm=12.5, dTerm=0 → output=62.5
func TestPIDResetClearsIntegral(t *testing.T) {
	p := defaultParams()
	s := pid.NewState()

	for i := 0; i < 20; i++ {
		pid.Compute(&p, s, 40.0)
	}

	s.Reset()
	terms := pid.Compute(&p, s, 80.0)

	const want = 62.5
	if terms.Output != want {
		t.Fatalf("after Reset, output %.4f != expected %.4f", terms.Output, want)
	}
}

// TestPIDAntiWindup confirms that the integral does not accumulate beyond
// the output bounds even under sustained saturation.
func TestPIDAntiWindup(t *testing.T) {
	p := defaultParams()
	p.OutMax = 50 // tight ceiling to force saturation quickly
	s := pid.NewState()

	for i := 0; i < 100; i++ {
		terms := pid.Compute(&p, s, 200.0) // huge positive err → output saturates high
		if terms.Output > p.OutMax+1e-9 {
			t.Fatalf("iteration %d: saturated output %.4f > OutMax %.4f", i, terms.Output, p.OutMax)
		}
	}
}
