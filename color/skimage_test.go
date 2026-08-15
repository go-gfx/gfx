package color

import (
	"math"
	"testing"
)

// The reference tuples below are the exact outputs of scikit-image 0.26
// (skimage.color.rgb2xyz / rgb2lab / lab2rgb), captured so the suite proves
// numerical parity of the skimage-compat regime without importing Python.

// byte8 scales an 8-bit channel to the unit range the scalar functions take.
func byte8(v uint8) float64 { return float64(v) / 255 }

// SkimageSRGBToXYZ matches scikit-image's rgb2xyz to floating-point precision.
func TestSkimageXYZReference(t *testing.T) {
	cases := []struct {
		name    string
		r, g, b uint8
		want    XYZ
	}{
		{"red", 255, 0, 0, XYZ{0.412453, 0.212671, 0.019334}},
		{"green", 0, 255, 0, XYZ{0.35758, 0.71516, 0.119193}},
		{"blue", 0, 0, 255, XYZ{0.180423, 0.072169, 0.950227}},
		{"white", 255, 255, 255, XYZ{0.950456, 1.0, 1.088754}},
		{"black", 0, 0, 0, XYZ{0, 0, 0}},
		{"gray", 128, 128, 128, XYZ{0.20516590749625624, 0.21586050011389926, 0.2350189829410083}},
	}
	for _, c := range cases {
		got := SkimageSRGBToXYZ(byte8(c.r), byte8(c.g), byte8(c.b))
		if math.Abs(got.X-c.want.X) > 1e-12 || math.Abs(got.Y-c.want.Y) > 1e-12 || math.Abs(got.Z-c.want.Z) > 1e-12 {
			t.Errorf("%s SkimageSRGBToXYZ = %+v, want %+v", c.name, got, c.want)
		}
	}
}

// SkimageSRGBToLab matches scikit-image's rgb2lab, including the tiny non-zero
// a*/b* for pure white that proves the scikit-image matrix and white are both in
// play (the textbook regime gives exactly (100,0,0), see below).
func TestSkimageLabReference(t *testing.T) {
	cases := []struct {
		name    string
		r, g, b uint8
		want    Lab
	}{
		{"red", 255, 0, 0, Lab{53.2405879437449, 80.0923082256922, 67.2027510444287}},
		{"green", 0, 255, 0, Lab{87.73509948831895, -86.18302974439501, 83.17970317538452}},
		{"blue", 0, 0, 255, Lab{32.29567256501352, 79.18559091176553, -107.85730020669489}},
		{"white", 255, 255, 255, Lab{100.0, -0.0024549378620508655, 0.004653421154054982}},
		{"black", 0, 0, 0, Lab{0, 0, 0}},
		{"gray", 128, 128, 128, Lab{53.58501345216902, -0.0014726455530578164, 0.0027914514965754478}},
	}
	for _, c := range cases {
		got := SkimageSRGBToLab(byte8(c.r), byte8(c.g), byte8(c.b))
		if math.Abs(got.L-c.want.L) > 1e-11 || math.Abs(got.A-c.want.A) > 1e-11 || math.Abs(got.B-c.want.B) > 1e-11 {
			t.Errorf("%s SkimageSRGBToLab = %+v, want %+v", c.name, got, c.want)
		}
	}
}

// The two regimes genuinely diverge: on pure white the textbook regime gives
// exactly (100, 0, 0) while the skimage regime carries a ~0.0025 offset in a*.
func TestRegimesDiverge(t *testing.T) {
	// The textbook matrix's column sums equal D65 to ~1e-5, so white is
	// (100, 0, 0) to within 1e-4 -- two orders of magnitude tighter than the
	// scikit-image regime's deliberate ~0.00245 offset asserted below.
	textbook := SRGBToLab(1, 1, 1)
	if math.Abs(textbook.A) > 1e-4 || math.Abs(textbook.B) > 1e-4 {
		t.Errorf("textbook white a*/b* = (%v,%v), want ~0", textbook.A, textbook.B)
	}
	sk := SkimageSRGBToLab(1, 1, 1)
	if math.Abs(sk.A) < 1e-4 {
		t.Errorf("skimage white a* = %v, want the scikit-image ~-0.00245 offset", sk.A)
	}
	// And the two RGB->XYZ matrices differ on a mid grey beyond rounding.
	if math.Abs(SRGBToLab(0.5, 0.5, 0.5).L-SkimageSRGBToLab(0.5, 0.5, 0.5).L) < 1e-9 {
		// L happens to be close; assert the a* axis instead, which reflects the
		// matrix/white offset.
		if math.Abs(SRGBToLab(0.5, 0.5, 0.5).A-SkimageSRGBToLab(0.5, 0.5, 0.5).A) < 1e-6 {
			t.Error("expected the two regimes to differ on mid grey")
		}
	}
}

// palette8 is the fixed set used for the byte-parity round trips.
var palette8 = [][3]uint8{
	{255, 0, 0}, {0, 255, 0}, {0, 0, 255}, {255, 255, 255},
	{0, 0, 0}, {128, 64, 192}, {10, 20, 30}, {200, 150, 90},
}

// scikit-image lab2rgb(rgb2lab(x)) and xyz2rgb(rgb2xyz(x)) recover each palette
// colour exactly after the round-half-to-even byte quantisation; so must the
// skimage-compat scalar pipeline plus [QuantizeUnitToByte].
func TestSkimageByteRoundTrips(t *testing.T) {
	for _, c := range palette8 {
		lab := SkimageSRGBToLab(byte8(c[0]), byte8(c[1]), byte8(c[2]))
		lr, lg, lb := SkimageLabToSRGB(lab)
		if QuantizeUnitToByte(lr) != c[0] || QuantizeUnitToByte(lg) != c[1] || QuantizeUnitToByte(lb) != c[2] {
			t.Errorf("Lab byte round trip %v = (%d,%d,%d)", c,
				QuantizeUnitToByte(lr), QuantizeUnitToByte(lg), QuantizeUnitToByte(lb))
		}
		xyz := SkimageSRGBToXYZ(byte8(c[0]), byte8(c[1]), byte8(c[2]))
		xr, xg, xb := SkimageXYZToSRGB(xyz)
		if QuantizeUnitToByte(xr) != c[0] || QuantizeUnitToByte(xg) != c[1] || QuantizeUnitToByte(xb) != c[2] {
			t.Errorf("XYZ byte round trip %v = (%d,%d,%d)", c,
				QuantizeUnitToByte(xr), QuantizeUnitToByte(xg), QuantizeUnitToByte(xb))
		}
	}
}

// Out-of-gamut Lab values reproduce scikit-image's lab2rgb bytes, exercising the
// negative-Z clip, the [0,1] clamp on both ends, and round-half-to-even.
func TestSkimageLabToRGBOutOfGamut(t *testing.T) {
	labs := []Lab{{50, 120, -120}, {50, 0, 120}, {100, 80, 80}, {20, -100, -100}, {90, -80, 90}}
	want := [][3]uint8{{184, 0, 255}, {147, 116, 0}, {255, 178, 100}, {0, 82, 204}, {86, 255, 0}}
	for i, l := range labs {
		r, g, b := SkimageLabToSRGB(l)
		got := [3]uint8{QuantizeUnitToByte(r), QuantizeUnitToByte(g), QuantizeUnitToByte(b)}
		if got != want[i] {
			t.Errorf("lab2rgb %+v = %v, want %v", l, got, want[i])
		}
	}
}

// QuantizeUnitToByte clamps out-of-range inputs and rounds halves to even.
func TestQuantizeUnitToByte(t *testing.T) {
	if QuantizeUnitToByte(-0.5) != 0 {
		t.Error("negative not clamped to 0")
	}
	if QuantizeUnitToByte(1.5) != 255 {
		t.Error("over-one not clamped to 255")
	}
	// 0.5/255 sits exactly between 0 and 1; round-half-to-even -> 0.
	if got := QuantizeUnitToByte(0.5 / 255); got != 0 {
		t.Errorf("round-half-even(0.5/255) = %d, want 0", got)
	}
	// 1.5/255 rounds half-to-even -> 2.
	if got := QuantizeUnitToByte(1.5 / 255); got != 2 {
		t.Errorf("round-half-even(1.5/255) = %d, want 2", got)
	}
}

// The skimage XYZ<->Lab pair round-trips in float space across the palette.
func TestSkimageXYZLabFloatRoundTrip(t *testing.T) {
	for _, c := range palette8 {
		if c == [3]uint8{0, 0, 0} {
			continue // black: Lab (0,0,0) recovers via the linear tail, checked in bytes above.
		}
		xyz := SkimageSRGBToXYZ(byte8(c[0]), byte8(c[1]), byte8(c[2]))
		back := SkimageLabToXYZ(SkimageXYZToLab(xyz))
		if math.Abs(back.X-xyz.X) > 1e-9 || math.Abs(back.Y-xyz.Y) > 1e-9 || math.Abs(back.Z-xyz.Z) > 1e-9 {
			t.Errorf("%v skimage XYZ<->Lab round trip = %+v, want %+v", c, back, xyz)
		}
	}
}
