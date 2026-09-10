// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import (
	"math"
	"math/rand"
	"testing"
)

// This file measures the calibrated spaces against poppler, transcribed below
// from poppler/GfxState.cc (26.04.0). poppler is an INDEPENDENT implementation
// of the same two specifications, so agreeing with it says the arithmetic here
// is the arithmetic the format describes rather than a reading of it that only
// this repository shares.
//
// The agreement is close but not exact, and the reasons are named constants
// rather than mysteries:
//
//   - poppler's D65 is (0.9505, 1, 1.0890) where [D65] carries the CIE value
//     (0.95047, 1, 1.08883);
//   - poppler's sRGB encoding breaks at 0.03928/12.92321 with a slope of
//     12.92321, where IEC 61966-2-1 breaks at 0.0031308 with a slope of 12.92
//     — its own comment flags the discrepancy and picks the PostScript
//     Reference value;
//   - poppler folds the inverse Bradford matrix and the destination white
//     point into one constant matrix, where [Adapt] multiplies the two.
//
// Each is a rounding in the fourth digit or beyond, so the tolerance below is
// stated in eighth-bit levels: what a reader would see.

// popplerXYZRGB is the xyzrgb matrix of GfxState.cc:653, the D65-referenced
// inverse sRGB primaries.
var popplerXYZRGB = [3][3]float64{
	{3.240449, -1.537136, -0.498531},
	{-0.969265, 1.876011, 0.041556},
	{0.055643, -0.204026, 1.057229},
}

// popplerSRGBGamma is srgb_gamma_function of GfxState.cc:657.
func popplerSRGBGamma(x float64) float64 {
	if x <= 0.03928/12.92321 {
		return x * 12.92321
	}
	return 1.055*math.Pow(x, 1.0/2.4) - 0.055
}

// popplerBradfordToD65 is bradford_transform_to_d65 of GfxState.cc:702, which
// carries XYZ under the source white point to XYZ under D65.
func popplerBradfordToD65(x, y, z, wx, wy, wz float64) (float64, float64, float64) {
	if wx == 0.9505 && wy == 1.0 && wz == 1.0890 {
		return x, y, z
	}
	rho := 0.8951*x + 0.2664*y - 0.1614*z
	gam := -0.7502*x + 1.7135*y + 0.0367*z
	bet := 0.0389*x - 0.0685*y + 1.0296*z
	rho /= 0.8951*wx + 0.2664*wy - 0.1614*wz
	gam /= -0.7502*wx + 1.7135*wy + 0.0367*wz
	bet /= 0.0389*wx - 0.0685*wy + 1.0296*wz
	return 0.92918329*rho - 0.15299782*gam + 0.17428453*bet,
		0.40698452*rho + 0.53931108*gam + 0.05370440*bet,
		-0.00802913*rho + 0.04166125*gam + 1.05519788*bet
}

// popplerCalRGB is GfxCalRGBColorSpace::getXYZ followed by the non-CMS branch
// of ::getRGB. pdfimages takes that branch: GfxState's constructor builds its
// XYZ-to-display transforms from a null profile, and nothing in pdfimages ever
// sets one, so the little-cms path is never armed for a calibrated space.
func popplerCalRGB(s CalRGB, a, b, c float64) (float64, float64, float64) {
	da := math.Pow(a, s.Gamma[0])
	db := math.Pow(b, s.Gamma[1])
	dc := math.Pow(c, s.Gamma[2])
	m := s.Matrix
	x := m[0]*da + m[3]*db + m[6]*dc
	y := m[1]*da + m[4]*db + m[7]*dc
	z := m[2]*da + m[5]*db + m[8]*dc
	x, y, z = popplerBradfordToD65(x, y, z, s.White.X, s.White.Y, s.White.Z)
	out := [3]float64{}
	for i, row := range popplerXYZRGB {
		out[i] = popplerSRGBGamma(clamp01(row[0]*x + row[1]*y + row[2]*z))
	}
	return out[0], out[1], out[2]
}

// popplerCalGray is GfxCalGrayColorSpace::getXYZ and the same tail.
func popplerCalGray(s CalGray, a float64) (float64, float64, float64) {
	v := math.Pow(a, s.Gamma)
	x, y, z := popplerBradfordToD65(s.White.X*v, s.White.Y*v, s.White.Z*v, s.White.X, s.White.Y, s.White.Z)
	out := [3]float64{}
	for i, row := range popplerXYZRGB {
		out[i] = popplerSRGBGamma(clamp01(row[0]*x + row[1]*y + row[2]*z))
	}
	return out[0], out[1], out[2]
}

// levels is the tolerance in eighth-bit levels. One level is what the constant
// differences listed at the top of this file can move a channel by, and it is
// half of the ±2 the conformance baseline allows a decoded picture.
const levels = 1

// adobeRGBD50 is the space of the PDF 2.0 test file
// openpdf-core/pdf-2-0_PDF_2.0_image_with_BPC.pdf, whose two images are the
// same JPEG stream drawn once through this space and once through DeviceRGB.
// It is here because it is a real document's real space rather than one chosen
// to be easy: an Adobe RGB matrix under D50 with a gamma of 2.2.
var adobeRGBD50 = CalRGB{
	White:  WhitePoint{X: 0.9643, Y: 1.0000, Z: 0.8251},
	Gamma:  [3]float64{2.2, 2.2, 2.2},
	Matrix: [9]float64{0.7161, 0.2582, 0.0000, 0.1009, 0.7249, 0.0518, 0.1472, 0.0168, 0.7734},
}

func byteOf(v float64) int { return int(math.Round(clamp01(v) * 255)) }

func TestCalRGBAgreesWithPoppler(t *testing.T) {
	spaces := map[string]CalRGB{
		"adobe RGB under D50": adobeRGBD50,
		"the defaults under D65": func() CalRGB {
			s := NewCalRGB(D65)
			return s
		}(),
		"sRGB primaries under D65 with gamma 2.2": {
			White:  D65,
			Gamma:  [3]float64{2.2, 2.2, 2.2},
			Matrix: [9]float64{0.4124, 0.2126, 0.0193, 0.3576, 0.7152, 0.1192, 0.1805, 0.0722, 0.9505},
		},
	}
	for name, s := range spaces {
		t.Run(name, func(t *testing.T) {
			worst, worstAt := 0, [3]float64{}
			// Every eighth level on each axis: 33^3 = 35937 colours, which
			// covers the corners and the neutral axis exactly.
			for i := 0; i <= 32; i++ {
				for j := 0; j <= 32; j++ {
					for k := 0; k <= 32; k++ {
						a, b, c := float64(i)/32, float64(j)/32, float64(k)/32
						gr, gg, gb := CalRGBToSRGB(s, a, b, c)
						wr, wg, wb := popplerCalRGB(s, a, b, c)
						for _, d := range [][2]float64{{gr, wr}, {gg, wg}, {gb, wb}} {
							if e := byteOf(d[0]) - byteOf(d[1]); e > worst || -e > worst {
								if e < 0 {
									e = -e
								}
								worst, worstAt = e, [3]float64{a, b, c}
							}
						}
					}
				}
			}
			if worst > levels {
				t.Errorf("worst disagreement %d levels at (%.3f, %.3f, %.3f), want at most %d",
					worst, worstAt[0], worstAt[1], worstAt[2], levels)
			}
			t.Logf("worst disagreement over 35937 colours: %d level(s)", worst)
		})
	}
}

func TestCalGrayAgreesWithPoppler(t *testing.T) {
	spaces := map[string]CalGray{
		"the default gamma under D65": NewCalGray(D65),
		"the default gamma under D50": NewCalGray(D50),
		"gamma 2.2 under D65":         {White: D65, Gamma: 2.2},
	}
	for name, s := range spaces {
		t.Run(name, func(t *testing.T) {
			worst, worstAt := 0, 0.0
			for i := 0; i <= 255; i++ {
				a := float64(i) / 255
				gr, gg, gb := CalGrayToSRGB(s, a)
				wr, wg, wb := popplerCalGray(s, a)
				for _, d := range [][2]float64{{gr, wr}, {gg, wg}, {gb, wb}} {
					if e := byteOf(d[0]) - byteOf(d[1]); e > worst || -e > worst {
						if e < 0 {
							e = -e
						}
						worst, worstAt = e, a
					}
				}
			}
			if worst > levels {
				t.Errorf("worst disagreement %d levels at %.4f, want at most %d", worst, worstAt, levels)
			}
			t.Logf("worst disagreement over 256 levels: %d level(s)", worst)
		})
	}
}

func TestCalGrayIsNotDeviceGray(t *testing.T) {
	// The whole reason this file exists: with the gamma the format defaults
	// to, a CalGray component is LINEAR luminance, so it encodes far lighter
	// than the same number read as a device grey. If these ever agree, the
	// calibration has been dropped somewhere.
	s := NewCalGray(D65)
	r, _, _ := CalGrayToSRGB(s, 0.5)
	if got, want := byteOf(r), 188; got != want {
		t.Errorf("mid CalGray = %d, want %d", got, want)
	}
	if byteOf(r) == 128 {
		t.Error("mid CalGray encoded as a device grey would; the gamma was dropped")
	}
}

func TestCalRGBDefaultsAreTheIdentityOnXYZ(t *testing.T) {
	// NewCalRGB must carry the format's defaults, or a dictionary that omits
	// /Gamma or /Matrix would be read as a space with none.
	s := NewCalRGB(D65)
	if s.Gamma != [3]float64{1, 1, 1} {
		t.Errorf("default gamma = %v, want all ones", s.Gamma)
	}
	if s.Matrix != [9]float64{1, 0, 0, 0, 1, 0, 0, 0, 1} {
		t.Errorf("default matrix = %v, want the identity", s.Matrix)
	}
	// With the identity matrix the components ARE X, Y and Z.
	got := s.XYZ(0.25, 0.5, 0.75)
	if got != (XYZ{X: 0.25, Y: 0.5, Z: 0.75}) {
		t.Errorf("XYZ under the defaults = %+v, want the components unchanged", got)
	}
}

func TestCalibratedClampsToTheGamut(t *testing.T) {
	// An Adobe RGB primary is outside sRGB. It must come back inside [0, 1]
	// rather than out of range, and it must not come back as pure white.
	r, g, b := CalRGBToSRGB(adobeRGBD50, 0, 1, 0)
	for _, v := range []float64{r, g, b} {
		if v < 0 || v > 1 {
			t.Errorf("out-of-gamut colour left the range: (%.4f, %.4f, %.4f)", r, g, b)
		}
	}
	if byteOf(r) == 255 && byteOf(g) == 255 && byteOf(b) == 255 {
		t.Error("an out-of-gamut green clamped to white; the clamp is per channel, not on the sum")
	}
}

func TestCalibratedIsStableUnderRandomSpaces(t *testing.T) {
	// A space whose numbers came from a document rather than from this file:
	// random gammas, matrices and white points, checked against poppler.
	rng := rand.New(rand.NewSource(4))
	worst := 0
	for n := 0; n < 200; n++ {
		s := CalRGB{
			White: WhitePoint{X: 0.85 + 0.25*rng.Float64(), Y: 1, Z: 0.7 + 0.5*rng.Float64()},
			Gamma: [3]float64{0.8 + 2*rng.Float64(), 0.8 + 2*rng.Float64(), 0.8 + 2*rng.Float64()},
		}
		for i := range s.Matrix {
			s.Matrix[i] = rng.Float64()
		}
		for m := 0; m < 40; m++ {
			a, b, c := rng.Float64(), rng.Float64(), rng.Float64()
			gr, gg, gb := CalRGBToSRGB(s, a, b, c)
			wr, wg, wb := popplerCalRGB(s, a, b, c)
			for _, d := range [][2]float64{{gr, wr}, {gg, wg}, {gb, wb}} {
				e := byteOf(d[0]) - byteOf(d[1])
				if e < 0 {
					e = -e
				}
				if e > worst {
					worst = e
				}
			}
		}
	}
	if worst > levels {
		t.Errorf("worst disagreement over 200 random spaces: %d levels, want at most %d", worst, levels)
	}
	t.Logf("worst disagreement over 200 random spaces, 8000 colours: %d level(s)", worst)
}
