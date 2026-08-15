package color

import "testing"

// EncodeGamma and DecodeGamma are inverse power laws and match hand values.
func TestGammaRoundTrip(t *testing.T) {
	if got := DecodeGamma(0.5, 2.2); !approx(got, 0.5*0.5, 1e-12) && !approx(got, 0.217637640824, 1e-9) {
		t.Fatalf("DecodeGamma(0.5, 2.2) = %v", got)
	}
	for _, c := range []float64{0, 0.1, 0.5, 0.9, 1} {
		if got := DecodeGamma(EncodeGamma(c, 2.2), 2.2); !approx(got, c, 1e-12) {
			t.Fatalf("gamma round trip %v = %v", c, got)
		}
	}
	// Encoding is the 1/gamma power: 1.0 stays 1.0, 0 stays 0.
	if got := EncodeGamma(1, 2.4); !approx(got, 1, 1e-12) {
		t.Fatalf("EncodeGamma(1) = %v", got)
	}
}

// Clamp01 pins values into [0, 1] on both sides and passes interior values.
func TestClamp01(t *testing.T) {
	cases := [][2]float64{{-0.3, 0}, {0, 0}, {0.42, 0.42}, {1, 1}, {1.7, 1}}
	for _, c := range cases {
		if got := Clamp01(c[0]); got != c[1] {
			t.Fatalf("Clamp01(%v) = %v, want %v", c[0], got, c[1])
		}
	}
}

// ClampRGB clamps each channel independently.
func TestClampRGB(t *testing.T) {
	r, g, b := ClampRGB(-0.1, 0.5, 1.4)
	if r != 0 || g != 0.5 || b != 1 {
		t.Fatalf("ClampRGB = (%v,%v,%v), want (0,0.5,1)", r, g, b)
	}
}

// InGamut reports membership in [0,1] within eps, on every boundary.
func TestInGamut(t *testing.T) {
	if !InGamut(0, 0.5, 1, 0) {
		t.Fatal("in-gamut colour reported out")
	}
	if InGamut(-0.001, 0.5, 0.5, 0) {
		t.Fatal("slightly negative reported in-gamut with eps 0")
	}
	if !InGamut(-1e-9, 0.5, 1+1e-9, 1e-6) {
		t.Fatal("rounding-margin colour reported out with eps 1e-6")
	}
	if InGamut(0.5, 0.5, 1.2, 1e-6) {
		t.Fatal("clearly out-of-range channel reported in-gamut")
	}
}
