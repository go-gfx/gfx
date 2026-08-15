package color

import (
	"math"
	"testing"
)

func approx(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// The sRGB transfer function round-trips, matches its two published anchors, and
// exercises both the linear (near-black) and power (mid/high) branches in each
// direction.
func TestSRGBTransfer(t *testing.T) {
	// Endpoints are exact.
	if v := SRGBToLinear(0); v != 0 {
		t.Fatalf("SRGBToLinear(0) = %v, want 0", v)
	}
	if v := SRGBToLinear(1); !approx(v, 1, 1e-12) {
		t.Fatalf("SRGBToLinear(1) = %v, want 1", v)
	}
	// Linear (below-knee) branch: 0.03 / 12.92.
	if v := SRGBToLinear(0.03); !approx(v, 0.03/12.92, 1e-12) {
		t.Fatalf("SRGBToLinear(0.03) = %v", v)
	}
	// Power branch: mid grey.
	if v := SRGBToLinear(0.5); !approx(v, 0.21404114, 1e-6) {
		t.Fatalf("SRGBToLinear(0.5) = %v, want ~0.214041", v)
	}
	// Inverse, both branches.
	if v := LinearToSRGB(0.002); !approx(v, 0.002*12.92, 1e-12) {
		t.Fatalf("LinearToSRGB(0.002) = %v", v)
	}
	if v := LinearToSRGB(0.21404114); !approx(v, 0.5, 1e-6) {
		t.Fatalf("LinearToSRGB round trip = %v, want 0.5", v)
	}
	// Full round trip across the range. The knee value 0.04045 sits on the
	// piecewise boundary, where the two thresholds are not exact inverses, so a
	// ~1e-7 tolerance is inherent there rather than machine epsilon.
	for _, c := range []float64{0, 0.01, 0.04045, 0.25, 0.6, 1} {
		if got := LinearToSRGB(SRGBToLinear(c)); !approx(got, c, 1e-6) {
			t.Fatalf("round trip %v = %v", c, got)
		}
	}
}

// Linear sRGB (1,1,1) maps to the D65 white point, and XYZ<->linear-RGB round
// trips.
func TestXYZMatrix(t *testing.T) {
	w := LinearRGBToXYZ(1, 1, 1)
	if !approx(w.X, D65.X, 1e-4) || !approx(w.Y, D65.Y, 1e-4) || !approx(w.Z, D65.Z, 1e-4) {
		t.Fatalf("white XYZ = %+v, want D65", w)
	}
	r, g, b := XYZToLinearRGB(LinearRGBToXYZ(0.3, 0.6, 0.9))
	if !approx(r, 0.3, 1e-6) || !approx(g, 0.6, 1e-6) || !approx(b, 0.9, 1e-6) {
		t.Fatalf("XYZ round trip = (%v,%v,%v), want (0.3,0.6,0.9)", r, g, b)
	}
}

// The four canonical sRGB corners convert to their published CIE L*a*b* values.
// White and black also drive the two branches of the Lab nonlinearity.
func TestLabReferenceColors(t *testing.T) {
	cases := []struct {
		name       string
		r, g, b    float64
		wl, wa, wb float64
		tol        float64
	}{
		{"white", 1, 1, 1, 100, 0, 0, 1e-2},
		{"black", 0, 0, 0, 0, 0, 0, 1e-9},
		{"red", 1, 0, 0, 53.2408, 80.0925, 67.2032, 1e-2},
		{"green", 0, 1, 0, 87.7347, -86.1827, 83.1793, 1e-2},
		{"blue", 0, 0, 1, 32.2970, 79.1875, -107.8602, 1e-2},
	}
	for _, c := range cases {
		got := SRGBToLab(c.r, c.g, c.b)
		if !approx(got.L, c.wl, c.tol) || !approx(got.A, c.wa, c.tol) || !approx(got.B, c.wb, c.tol) {
			t.Fatalf("%s SRGBToLab = %+v, want (%v,%v,%v)", c.name, got, c.wl, c.wa, c.wb)
		}
	}
}

// Lab -> sRGB inverts SRGBToLab across a spread of in-gamut colours, exercising
// LabToXYZ / XYZToLinearRGB / LabFInv.
func TestLabRoundTrip(t *testing.T) {
	for _, c := range [][3]float64{
		{0.5, 0.5, 0.5}, {0.2, 0.7, 0.4}, {0.9, 0.1, 0.3}, {0.05, 0.05, 0.05},
	} {
		lab := SRGBToLab(c[0], c[1], c[2])
		r, g, b := LabToSRGB(lab)
		if !approx(r, c[0], 1e-6) || !approx(g, c[1], 1e-6) || !approx(b, c[2], 1e-6) {
			t.Fatalf("Lab round trip of %v = (%v,%v,%v)", c, r, g, b)
		}
	}
}
