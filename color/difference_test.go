package color

import (
	"math"
	"testing"
)

// DeltaE76, DeltaE94 and DeltaE2000 match the colour-science oracle across a
// spread of L*a*b* pairs, including near-threshold small differences. The hue
// pairs additionally drive the CIEDE2000 hue-wrap branches (the shorter arc
// crossing 0/360 in both directions).
func TestDeltaEMetrics(t *testing.T) {
	cases := []struct {
		a, b            Lab
		e76, e94, e2000 float64
	}{
		{Lab{53.24, 80.09, 67.20}, Lab{87.73, -86.18, 83.18}, 170.5597648920, 73.4281864791, 86.6064656122},
		{Lab{32.30, 79.19, -107.86}, Lab{53.24, 80.09, 67.20}, 176.3102299925, 61.2391242493, 52.8795626944},
		{Lab{50.0, 10.0, -5.0}, Lab{55.0, -8.0, 12.0}, 25.2586618806, 21.7142957546, 27.5479057137},
		{Lab{20.0, 5.0, 5.0}, Lab{20.5, 5.2, 4.7}, 0.6164414003, 0.5961033632, 0.5142436781},
	}
	for i, c := range cases {
		if got := DeltaE76(c.a, c.b); math.Abs(got-c.e76) > 1e-6 {
			t.Errorf("case %d DeltaE76 = %v, want %v", i, got, c.e76)
		}
		if got := DeltaE94(c.a, c.b); math.Abs(got-c.e94) > 1e-6 {
			t.Errorf("case %d DeltaE94 = %v, want %v", i, got, c.e94)
		}
		if got := DeltaE2000(c.a, c.b); math.Abs(got-c.e2000) > 1e-4 {
			t.Errorf("case %d DeltaE2000 = %v, want %v", i, got, c.e2000)
		}
	}
}

// The CIEDE2000 hue-difference and mean-hue branches around the 0/360 wrap.
func TestDeltaE2000HueWrap(t *testing.T) {
	cases := []struct {
		a, b Lab
		want float64
	}{
		{Lab{50, 20, -1}, Lab{50, 20, 1}, 1.3229857222},   // hue delta > 180 -> -360; mean-hue h1+h2 >= 360
		{Lab{50, -20, 1}, Lab{50, -20, -1}, 1.4501665419}, // mean-hue h1+h2 < 360
		{Lab{50, 1, 20}, Lab{50, -1, -20}, 30.6882774400}, // near-opposite hues
	}
	for i, c := range cases {
		if got := DeltaE2000(c.a, c.b); math.Abs(got-c.want) > 1e-4 {
			t.Errorf("hue case %d DeltaE2000 = %v, want %v", i, got, c.want)
		}
	}
}

// DeltaE76 and DeltaE2000 are zero for identical colours, and CIEDE2000 handles
// the achromatic pair (both chroma zero) without dividing by zero.
func TestDeltaEDegenerate(t *testing.T) {
	c := Lab{50, 12, -7}
	if got := DeltaE76(c, c); got != 0 {
		t.Errorf("DeltaE76 self = %v, want 0", got)
	}
	if got := DeltaE94(c, c); got != 0 {
		t.Errorf("DeltaE94 self = %v, want 0", got)
	}
	if got := DeltaE2000(c, c); got != 0 {
		t.Errorf("DeltaE2000 self = %v, want 0", got)
	}
	if got := DeltaE2000(Lab{40, 0, 0}, Lab{60, 0, 0}); got <= 0 {
		t.Errorf("DeltaE2000 grey pair = %v, want > 0", got)
	}
}

// DeltaEOK is the OKLab Euclidean distance: zero for equal colours and matching
// a hand-computed value for a simple offset.
func TestDeltaEOK(t *testing.T) {
	a := SRGBToOKLab(0.2, 0.4, 0.6)
	if got := DeltaEOK(a, a); got != 0 {
		t.Errorf("DeltaEOK self = %v, want 0", got)
	}
	b := OKLab{a.L + 0.03, a.A - 0.04, a.B + 0.12}
	want := math.Sqrt(0.03*0.03 + 0.04*0.04 + 0.12*0.12)
	if got := DeltaEOK(a, b); math.Abs(got-want) > 1e-12 {
		t.Errorf("DeltaEOK = %v, want %v", got, want)
	}
}
