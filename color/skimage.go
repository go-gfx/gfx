// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import "math"

// This file adds a second, deliberately distinct regime of the sRGB<->XYZ<->Lab
// pipeline: one that is byte-for-byte compatible with scikit-image's
// skimage.color module, as opposed to the textbook-CIE regime in space.go.
//
// The two regimes are BOTH correct for their purpose and genuinely diverge:
//
//   - space.go (rendering) uses the high-precision IEC/sRGB matrix
//     (0.4124564...) and the exact CIE Lab constants (the 6/29 break point), so
//     pure white maps to Lab (100, 0, 0) and a round trip is analytically clean.
//     This is what painter / webengine want.
//   - this file (skimage-compat) uses scikit-image's rounded matrix
//     (0.412453...), its inverse as numpy.linalg.inv produced it, and its Lab
//     constants (0.008856 / 7.787), plus its clip of a negative f-space Z before
//     the inverse nonlinearity. This reproduces scikit-image to floating-point
//     precision — including the tiny white-point offset that sends pure white to
//     Lab (100, -0.00245, 0.00465) — so go-images can share ONE colour library
//     for its scikit-image-parity image processing instead of a local copy.
//
// The sRGB companding transfer functions ([SRGBToLinear] / [LinearToSRGB]) are
// bit-identical between the two regimes, so they are shared, not duplicated.

// skimageXYZFromLinRGB is scikit-image's linear-sRGB -> CIE XYZ matrix
// (colorconv.xyz_from_rgb): the rounded coefficients it ships, distinct from the
// high-precision matrix used by [LinearRGBToXYZ].
var skimageXYZFromLinRGB = [3][3]float64{
	{0.412453, 0.357580, 0.180423},
	{0.212671, 0.715160, 0.072169},
	{0.019334, 0.119193, 0.950227},
}

// skimageLinRGBFromXYZ is scikit-image's CIE XYZ -> linear-sRGB matrix
// (colorconv.rgb_from_xyz), hardcoded to the exact values numpy.linalg.inv
// yields so the inverse matches scikit-image in the last bits.
var skimageLinRGBFromXYZ = [3][3]float64{
	{3.2404813432005266, -1.5371515162713183, -0.4985363261688878},
	{-0.9692549499965682, 1.8759900014898907, 0.04155592655829283},
	{0.05564663913517717, -0.20404133836651123, 1.0573110696453443},
}

// SkimageLinearRGBToXYZ converts linear-light sRGB primaries to CIE XYZ using
// scikit-image's matrix (skimage.color rgb2xyz without the companding step).
func SkimageLinearRGBToXYZ(r, g, b float64) XYZ {
	x, y, z := mul3(skimageXYZFromLinRGB, r, g, b)
	return XYZ{X: x, Y: y, Z: z}
}

// SkimageXYZToLinearRGB converts CIE XYZ to linear-light sRGB primaries using
// scikit-image's inverse matrix, the inverse of [SkimageLinearRGBToXYZ].
func SkimageXYZToLinearRGB(c XYZ) (r, g, b float64) {
	return mul3(skimageLinRGBFromXYZ, c.X, c.Y, c.Z)
}

// SkimageSRGBToXYZ converts a gamma-encoded sRGB colour (each channel 0..1) to
// CIE XYZ, reproducing skimage.color.rgb2xyz (inverse companding then the
// scikit-image matrix).
func SkimageSRGBToXYZ(r, g, b float64) XYZ {
	return SkimageLinearRGBToXYZ(SRGBToLinear(r), SRGBToLinear(g), SRGBToLinear(b))
}

// SkimageXYZToSRGB converts CIE XYZ to gamma-encoded sRGB (each channel 0..1,
// not quantised), reproducing skimage.color.xyz2rgb (the inverse matrix then
// forward companding), the inverse of [SkimageSRGBToXYZ].
func SkimageXYZToSRGB(c XYZ) (r, g, b float64) {
	lr, lg, lb := SkimageXYZToLinearRGB(c)
	return LinearToSRGB(lr), LinearToSRGB(lg), LinearToSRGB(lb)
}

// skimageLabF is scikit-image's CIELAB forward nonlinearity, with its rounded
// threshold (0.008856) and slope (7.787), distinct from the exact-6/29 [labF].
func skimageLabF(t float64) float64 {
	if t > 0.008856 {
		return math.Cbrt(t)
	}
	return 7.787*t + 16.0/116.0
}

// skimageLabFInv inverts [skimageLabF] using scikit-image's stored threshold
// constant 0.2068966 (its rounded cube root of 0.008856).
func skimageLabFInv(t float64) float64 {
	if t > 0.2068966 {
		return t * t * t
	}
	return (t - 16.0/116.0) / 7.787
}

// SkimageXYZToLab converts CIE XYZ to CIELAB relative to the D65 white, matching
// skimage.color.xyz2lab (its constants and reference white).
func SkimageXYZToLab(c XYZ) Lab {
	fx := skimageLabF(c.X / D65.X)
	fy := skimageLabF(c.Y / D65.Y)
	fz := skimageLabF(c.Z / D65.Z)
	return Lab{L: 116*fy - 16, A: 500 * (fx - fy), B: 200 * (fy - fz)}
}

// SkimageLabToXYZ converts CIELAB back to CIE XYZ, matching skimage.color
// .lab2xyz — including its clip of a negative Z in f-space, applied BEFORE the
// inverse nonlinearity rather than after it. Inverse of [SkimageXYZToLab].
func SkimageLabToXYZ(l Lab) XYZ {
	fy := (l.L + 16) / 116
	fx := l.A/500 + fy
	fz := fy - l.B/200
	if fz < 0 {
		fz = 0
	}
	return XYZ{
		X: skimageLabFInv(fx) * D65.X,
		Y: skimageLabFInv(fy) * D65.Y,
		Z: skimageLabFInv(fz) * D65.Z,
	}
}

// SkimageSRGBToLab converts a gamma-encoded sRGB colour to CIELAB, reproducing
// skimage.color.rgb2lab (rgb2xyz composed with xyz2lab). Because it uses
// scikit-image's matrix and white together, pure white maps to
// Lab (100, -0.00245, 0.00465), exactly as scikit-image does.
func SkimageSRGBToLab(r, g, b float64) Lab { return SkimageXYZToLab(SkimageSRGBToXYZ(r, g, b)) }

// SkimageLabToSRGB converts a CIELAB colour to gamma-encoded sRGB (each channel
// 0..1, not quantised, not gamut-clamped), reproducing skimage.color.lab2rgb
// before its byte conversion. Inverse of [SkimageSRGBToLab].
func SkimageLabToSRGB(l Lab) (r, g, b float64) { return SkimageXYZToSRGB(SkimageLabToXYZ(l)) }

// QuantizeUnitToByte clamps a channel value to [0, 1] and quantises it to an
// 8-bit value using round-half-to-even, matching scikit-image's img_as_ubyte
// (numpy.rint). It is the quantisation step image-level consumers apply after
// [SkimageLabToSRGB] / [SkimageXYZToSRGB] to reproduce scikit-image byte output.
func QuantizeUnitToByte(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return uint8(math.RoundToEven(v * 255))
}
