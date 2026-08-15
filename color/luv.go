// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import "math"

// This file adds the two CIE 1976 perceptual spaces that complement L*a*b*: the
// cylindrical form LCh(ab) (chroma and hue angle instead of the a*/b* Cartesian
// axes) and CIE L*u*v* with its own cylindrical form LCh(uv). All three carry a
// [WhitePoint]; the sRGB convenience wrappers use [D65]. Hue is in degrees
// [0, 360).

// LCH is a colour in a cylindrical CIE space: L is lightness, C is chroma
// (radial) and H is the hue angle in degrees [0, 360). It represents both
// LCh(ab) (from [Lab]) and LCh(uv) (from [Luv]); which one is fixed by the
// conversion that produced it.
type LCH struct{ L, C, H float64 }

// Luv is a colour in CIE 1976 L*u*v*: L is lightness (0..100) and u*, v* are the
// chromatic axes.
type Luv struct{ L, U, V float64 }

// toCylindrical turns a lightness and a Cartesian (a, b) chroma pair into
// (chroma, hue-degrees), the shared core of the LCh conversions.
func toCylindrical(l, a, b float64) LCH {
	h := math.Atan2(b, a) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return LCH{L: l, C: math.Hypot(a, b), H: h}
}

// fromCylindrical turns an LCh triple back into a Cartesian (a, b) chroma pair.
func fromCylindrical(c LCH) (a, b float64) {
	r := c.H * math.Pi / 180
	return c.C * math.Cos(r), c.C * math.Sin(r)
}

// LabToLCH converts CIE L*a*b* to its cylindrical form LCh(ab).
func LabToLCH(l Lab) LCH { return toCylindrical(l.L, l.A, l.B) }

// LCHToLab converts cylindrical LCh(ab) back to CIE L*a*b*, the inverse of
// [LabToLCH].
func LCHToLab(c LCH) Lab {
	a, b := fromCylindrical(c)
	return Lab{L: c.L, A: a, B: b}
}

// SRGBToLCH converts a gamma-encoded sRGB colour to LCh(ab) under D65.
func SRGBToLCH(r, g, b float64) LCH { return LabToLCH(SRGBToLab(r, g, b)) }

// LCHToSRGB converts an LCh(ab) colour under D65 back to gamma-encoded sRGB.
func LCHToSRGB(c LCH) (r, g, b float64) { return LabToSRGB(LCHToLab(c)) }

// uvPrime returns the CIE 1976 u', v' chromaticity of an XYZ colour. The
// denominator X + 15Y + 3Z is zero only for pure black, for which (0, 0) is
// returned.
func uvPrime(c XYZ) (up, vp float64) {
	d := c.X + 15*c.Y + 3*c.Z
	if d == 0 {
		return 0, 0
	}
	return 4 * c.X / d, 9 * c.Y / d
}

// luvKappa is the CIE constant (29/3)^3 governing the low-luminance branch, and
// luvEps is (6/29)^3, the Y/Yn threshold between the two branches.
const (
	luvKappa = 24389.0 / 27.0  // (29/3)^3
	luvEps   = 216.0 / 24389.0 // (6/29)^3
)

// XYZToLuvWP converts CIE XYZ to CIE L*u*v* relative to the white point wp.
func XYZToLuvWP(c XYZ, wp WhitePoint) Luv {
	yr := c.Y / wp.Y
	var l float64
	if yr > luvEps {
		l = 116*math.Cbrt(yr) - 16
	} else {
		l = luvKappa * yr
	}
	if l == 0 {
		return Luv{}
	}
	up, vp := uvPrime(c)
	upn, vpn := uvPrime(XYZ(wp))
	return Luv{L: l, U: 13 * l * (up - upn), V: 13 * l * (vp - vpn)}
}

// LuvToXYZWP converts CIE L*u*v* back to CIE XYZ relative to the white point wp,
// the inverse of [XYZToLuvWP].
func LuvToXYZWP(l Luv, wp WhitePoint) XYZ {
	if l.L == 0 {
		return XYZ{}
	}
	var y float64
	if l.L > luvKappa*luvEps {
		t := (l.L + 16) / 116
		y = wp.Y * t * t * t
	} else {
		y = wp.Y * l.L / luvKappa
	}
	upn, vpn := uvPrime(XYZ(wp))
	up := l.U/(13*l.L) + upn
	vp := l.V/(13*l.L) + vpn
	x := y * 9 * up / (4 * vp)
	z := y * (12 - 3*up - 20*vp) / (4 * vp)
	return XYZ{X: x, Y: y, Z: z}
}

// XYZToLuv converts CIE XYZ (D65) to CIE L*u*v*.
func XYZToLuv(c XYZ) Luv { return XYZToLuvWP(c, D65) }

// LuvToXYZ converts CIE L*u*v* back to CIE XYZ (D65), the inverse of [XYZToLuv].
func LuvToXYZ(l Luv) XYZ { return LuvToXYZWP(l, D65) }

// SRGBToLuv converts a gamma-encoded sRGB colour to CIE L*u*v* under D65.
func SRGBToLuv(r, g, b float64) Luv {
	return XYZToLuv(LinearRGBToXYZ(SRGBToLinear(r), SRGBToLinear(g), SRGBToLinear(b)))
}

// LuvToSRGB converts a CIE L*u*v* colour under D65 back to gamma-encoded sRGB
// (not clamped to gamut), the inverse of [SRGBToLuv].
func LuvToSRGB(l Luv) (r, g, b float64) {
	lr, lg, lb := XYZToLinearRGB(LuvToXYZ(l))
	return LinearToSRGB(lr), LinearToSRGB(lg), LinearToSRGB(lb)
}

// LuvToLCH converts CIE L*u*v* to its cylindrical form LCh(uv).
func LuvToLCH(l Luv) LCH { return toCylindrical(l.L, l.U, l.V) }

// LCHToLuv converts cylindrical LCh(uv) back to CIE L*u*v*, the inverse of
// [LuvToLCH].
func LCHToLuv(c LCH) Luv {
	u, v := fromCylindrical(c)
	return Luv{L: c.L, U: u, V: v}
}
