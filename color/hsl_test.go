package color

import "testing"

// NormalizeHue folds arbitrary hue angles into [0, 360), and the hue-based
// inverses accept out-of-range hues accordingly: -30 deg and 330 deg name the
// same colour, as do 390 deg and 30 deg.
func TestNormalizeHueAndInverses(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{-30, 330}, {390, 30}, {0, 0}, {359, 359}, {720, 0}, {-360, 0},
	}
	for _, c := range cases {
		if got := NormalizeHue(c.in); !approx(got, c.want, 1e-9) {
			t.Errorf("NormalizeHue(%v) = %v, want %v", c.in, got, c.want)
		}
	}
	// A negative hue and its normalised twin produce the same sRGB.
	r1, g1, b1 := HSVToSRGB(HSV{H: -30, S: 0.5, V: 0.8})
	r2, g2, b2 := HSVToSRGB(HSV{H: 330, S: 0.5, V: 0.8})
	if !srgbClose(r1, g1, b1, r2, g2, b2, 1e-12) {
		t.Errorf("HSV(-30) = (%v,%v,%v) != HSV(330) = (%v,%v,%v)", r1, g1, b1, r2, g2, b2)
	}
	// A hue >= 360 also wraps (drives the hp == 6 boundary via 360 deg).
	r3, g3, b3 := HSLToSRGB(HSL{H: 360, S: 0.4, L: 0.6})
	r4, g4, b4 := HSLToSRGB(HSL{H: 0, S: 0.4, L: 0.6})
	if !srgbClose(r3, g3, b3, r4, g4, b4, 1e-12) {
		t.Errorf("HSL(360) = (%v,%v,%v) != HSL(0) = (%v,%v,%v)", r3, g3, b3, r4, g4, b4)
	}
}

// HWB with whiteness and blackness that saturate the range collapses to the
// achromatic grey W/(W+B), independent of hue.
func TestHWBGrayscaleClamp(t *testing.T) {
	r, g, b := HWBToSRGB(HWB{H: 210, W: 0.6, B: 0.6})
	want := 0.6 / (0.6 + 0.6)
	if !approx(r, want, 1e-12) || !approx(g, want, 1e-12) || !approx(b, want, 1e-12) {
		t.Errorf("HWB grey = (%v,%v,%v), want %v each", r, g, b, want)
	}
}
