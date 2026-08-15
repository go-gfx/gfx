package color

import "testing"

// Bradford adaptation is an identity when source and destination whites match,
// carries the source white exactly onto the destination white, and the D65<->D50
// pair round-trips.
func TestBradfordAdaptation(t *testing.T) {
	c := XYZ{0.3, 0.42, 0.55}
	// The forward and inverse Bradford matrices are independently rounded
	// published constants, so a same-white adaptation is an identity only to
	// ~1e-7, not to machine epsilon.
	if got := Adapt(c, D65, D65); !approx(got.X, c.X, 1e-7) || !approx(got.Y, c.Y, 1e-7) || !approx(got.Z, c.Z, 1e-7) {
		t.Fatalf("Adapt(c, D65, D65) = %+v, want identity %+v", got, c)
	}
	// The D65 white, adapted to D50, is the D50 white.
	w := Adapt(XYZ(D65), D65, D50)
	if !approx(w.X, D50.X, 1e-6) || !approx(w.Y, D50.Y, 1e-6) || !approx(w.Z, D50.Z, 1e-6) {
		t.Fatalf("Adapt(D65 white, D65->D50) = %+v, want D50 %+v", w, D50)
	}
	// Round trip D65 -> D50 -> D65.
	back := AdaptD50ToD65(AdaptD65ToD50(c))
	if !approx(back.X, c.X, 1e-6) || !approx(back.Y, c.Y, 1e-6) || !approx(back.Z, c.Z, 1e-6) {
		t.Fatalf("D65->D50->D65 round trip = %+v, want %+v", back, c)
	}
}

// The CIE L*u*v* conversions handle the degenerate branches: pure black (L == 0)
// in both directions and the low-luminance linear branch (Y/Yn below (6/29)^3).
func TestLuvDegenerateAndLowLuminance(t *testing.T) {
	if got := XYZToLuvWP(XYZ{0, 0, 0}, D65); got != (Luv{}) {
		t.Fatalf("XYZToLuv(black) = %+v, want zero", got)
	}
	if got := LuvToXYZWP(Luv{}, D65); got != (XYZ{}) {
		t.Fatalf("LuvToXYZ(zero) = %+v, want black", got)
	}
	// A very dark but non-black colour uses the (29/3)^3 * (Y/Yn) branch and
	// round-trips through it.
	dark := XYZ{0.004, 0.005, 0.003}
	luv := XYZToLuvWP(dark, D65)
	if luv.L >= 8 { // below the kappa*eps ~= 8 knee
		t.Fatalf("dark L* = %v, expected low-luminance branch (< 8)", luv.L)
	}
	back := LuvToXYZWP(luv, D65)
	if !approx(back.X, dark.X, 1e-9) || !approx(back.Y, dark.Y, 1e-9) || !approx(back.Z, dark.Z, 1e-9) {
		t.Fatalf("dark Luv round trip = %+v, want %+v", back, dark)
	}
}

// uvPrime returns (0, 0) for the all-zero tristimulus (its denominator is zero);
// this branch is defensive and not reachable through XYZToLuv, which returns
// early for L* == 0, so it is exercised directly.
func TestUVPrimeZeroDenominator(t *testing.T) {
	if up, vp := uvPrime(XYZ{0, 0, 0}); up != 0 || vp != 0 {
		t.Fatalf("uvPrime(0) = (%v,%v), want (0,0)", up, vp)
	}
}
