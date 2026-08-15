package color

import (
	stdcolor "image/color"
	"math"
	"testing"
)

// samples8 is a synthetic 8-bit RGB grid used to validate YCbCr and CMYK against
// the reference transforms in Go's standard image/color package.
func samples8() [][3]uint8 {
	var out [][3]uint8
	for _, r := range []uint8{0, 1, 64, 128, 200, 255} {
		for _, g := range []uint8{0, 90, 128, 255} {
			for _, b := range []uint8{0, 50, 128, 255} {
				out = append(out, [3]uint8{r, g, b})
			}
		}
	}
	return out
}

// SRGBToYCbCr601 reproduces the standard library's full-range BT.601 JFIF
// transform (image/color.RGBToYCbCr). Luma matches to within one 8-bit level;
// chroma is allowed 1.5 levels because the library centres Cb/Cr on the integer
// 128 while this package uses the exact fractional neutral 0.5 (= 127.5/255).
func TestYCbCr601VsStdlib(t *testing.T) {
	for _, s := range samples8() {
		wantY, wantCb, wantCr := stdcolor.RGBToYCbCr(s[0], s[1], s[2])
		got := SRGBToYCbCr601(float64(s[0])/255, float64(s[1])/255, float64(s[2])/255)
		if d := math.Abs(got.Y*255 - float64(wantY)); d > 1 {
			t.Errorf("%v Y = %v, want %d", s, got.Y*255, wantY)
		}
		if d := math.Abs(got.Cb*255 - float64(wantCb)); d > 1.5 {
			t.Errorf("%v Cb = %v, want %d", s, got.Cb*255, wantCb)
		}
		if d := math.Abs(got.Cr*255 - float64(wantCr)); d > 1.5 {
			t.Errorf("%v Cr = %v, want %d", s, got.Cr*255, wantCr)
		}
	}
}

// The BT.601 forward/inverse pair round-trips exactly, and BT.709 does too; the
// two primary sets give genuinely different chroma for the same colour.
func TestYCbCrRoundTripAndDistinctPrimaries(t *testing.T) {
	distinct := false
	for _, s := range samples8() {
		r, g, b := float64(s[0])/255, float64(s[1])/255, float64(s[2])/255
		if rr, gg, bb := YCbCr601ToSRGB(SRGBToYCbCr601(r, g, b)); !srgbClose(r, g, b, rr, gg, bb, 1e-12) {
			t.Errorf("%v BT.601 round trip = (%v,%v,%v)", s, rr, gg, bb)
		}
		if rr, gg, bb := YCbCr709ToSRGB(SRGBToYCbCr709(r, g, b)); !srgbClose(r, g, b, rr, gg, bb, 1e-12) {
			t.Errorf("%v BT.709 round trip = (%v,%v,%v)", s, rr, gg, bb)
		}
		c6 := SRGBToYCbCr601(r, g, b)
		c7 := SRGBToYCbCr709(r, g, b)
		if math.Abs(c6.Y-c7.Y) > 1e-3 {
			distinct = true
		}
	}
	if !distinct {
		t.Fatal("BT.601 and BT.709 produced identical luma for every sample")
	}
}

// SRGBToCMYK reproduces the standard library's naive CMYK transform
// (image/color.RGBToCMYK / CMYKToRGB) to within one 8-bit level, including the
// pure-black K == 1 branch.
func TestCMYKVsStdlib(t *testing.T) {
	sawBlack := false
	for _, s := range samples8() {
		wc, wm, wy, wk := stdcolor.RGBToCMYK(s[0], s[1], s[2])
		got := SRGBToCMYK(float64(s[0])/255, float64(s[1])/255, float64(s[2])/255)
		for i, pair := range [][2]float64{
			{got.C * 255, float64(wc)}, {got.M * 255, float64(wm)},
			{got.Y * 255, float64(wy)}, {got.K * 255, float64(wk)},
		} {
			if math.Abs(pair[0]-pair[1]) > 1 {
				t.Errorf("%v CMYK[%d] = %v, want %v", s, i, pair[0], pair[1])
			}
		}
		if s == [3]uint8{0, 0, 0} {
			sawBlack = true
			if got.K != 1 || got.C != 0 || got.M != 0 || got.Y != 0 {
				t.Errorf("black CMYK = %+v, want {0,0,0,1}", got)
			}
		}
	}
	if !sawBlack {
		t.Fatal("black sample missing")
	}
}

// The CMYK forward/inverse pair round-trips for non-black colours.
func TestCMYKRoundTrip(t *testing.T) {
	for _, s := range oracleRows {
		if s.name == "black" {
			continue
		}
		rr, gg, bb := CMYKToSRGB(SRGBToCMYK(s.r, s.g, s.b))
		if !srgbClose(s.r, s.g, s.b, rr, gg, bb, 1e-12) {
			t.Errorf("%s CMYK round trip = (%v,%v,%v)", s.name, rr, gg, bb)
		}
	}
	// Black recovers to black through the inverse too.
	if rr, gg, bb := CMYKToSRGB(SRGBToCMYK(0, 0, 0)); rr != 0 || gg != 0 || bb != 0 {
		t.Errorf("black CMYK round trip = (%v,%v,%v)", rr, gg, bb)
	}
}
