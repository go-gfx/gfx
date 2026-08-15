// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

// This file defines reference white points and Bradford chromatic adaptation.
// Every CIE colour space that references a white (L*a*b*, L*u*v* and their
// cylindrical forms) is parameterised by a [WhitePoint], and colours can be
// carried between two illuminants with the Bradford cone-response transform,
// the method used by ICC profiles and the CSS Color 4 specification.

// WhitePoint is a reference white in CIE 1931 XYZ, its luminance Y normalised to
// 1. It is the illuminant a device-independent colour space is measured against.
type WhitePoint struct{ X, Y, Z float64 }

// D65 is the CIE Standard Illuminant D65 (noon daylight, ~6504 K) for the 2-deg
// observer — the white point sRGB, Rec.709 and Display-P3 are defined against.
var D65 = WhitePoint{X: 0.95047, Y: 1.0, Z: 1.08883}

// D50 is the CIE Standard Illuminant D50 (horizon light, ~5003 K) for the 2-deg
// observer — the connection-space white of ICC profiles and the reference white
// CSS Color 4 uses for Lab/LCH.
var D50 = WhitePoint{X: 0.96422, Y: 1.0, Z: 0.82521}

// bradford is the Bradford cone-response matrix M_A: it maps CIE XYZ onto the
// sharpened LMS cone space in which von Kries scaling is applied.
var bradford = [3][3]float64{
	{0.8951000, 0.2664000, -0.1614000},
	{-0.7502000, 1.7135000, 0.0367000},
	{0.0389000, -0.0685000, 1.0296000},
}

// bradfordInv is the inverse of [bradford], mapping the sharpened LMS cone space
// back to CIE XYZ.
var bradfordInv = [3][3]float64{
	{0.9869929, -0.1470543, 0.1599627},
	{0.4323053, 0.5183603, 0.0492912},
	{-0.0085287, 0.0400428, 0.9684867},
}

// mul3 applies the 3x3 matrix m to the column vector (x, y, z).
func mul3(m [3][3]float64, x, y, z float64) (float64, float64, float64) {
	return m[0][0]*x + m[0][1]*y + m[0][2]*z,
		m[1][0]*x + m[1][1]*y + m[1][2]*z,
		m[2][0]*x + m[2][1]*y + m[2][2]*z
}

// Adapt returns the CIE XYZ colour c, measured under illuminant src, expressed
// under illuminant dst using the Bradford chromatic-adaptation transform. When
// src == dst the colour is returned unchanged up to floating-point rounding.
func Adapt(c XYZ, src, dst WhitePoint) XYZ {
	sr, sg, sb := mul3(bradford, src.X, src.Y, src.Z)
	dr, dg, db := mul3(bradford, dst.X, dst.Y, dst.Z)
	// von Kries diagonal scaling in cone space.
	cr, cg, cb := mul3(bradford, c.X, c.Y, c.Z)
	cr *= dr / sr
	cg *= dg / sg
	cb *= db / sb
	x, y, z := mul3(bradfordInv, cr, cg, cb)
	return XYZ{X: x, Y: y, Z: z}
}

// AdaptD65ToD50 carries a CIE XYZ colour from the D65 white point to D50.
func AdaptD65ToD50(c XYZ) XYZ { return Adapt(c, D65, D50) }

// AdaptD50ToD65 carries a CIE XYZ colour from the D50 white point to D65.
func AdaptD50ToD65(c XYZ) XYZ { return Adapt(c, D50, D65) }
