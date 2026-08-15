// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import "math"

// This file adds go-gfx's colour-space conversions on top of the premultiplied
// -alpha helpers: the sRGB transfer function (gamma <-> linear light), the
// linear-sRGB <-> CIE XYZ matrices, and CIE XYZ <-> CIE L*a*b*. Channel values
// are float64 in [0, 1] for sRGB and linear RGB; XYZ and Lab keep their CIE
// units. The matrices and constants are the standard sRGB (IEC 61966-2-1) and
// CIE definitions, referenced against a D65 white point, so a round trip and the
// canonical primaries match published values.

// SRGBToLinear converts one sRGB-encoded channel value (0..1) to linear light
// using the IEC 61966-2-1 electro-optical transfer function.
func SRGBToLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// LinearToSRGB converts one linear-light channel value (0..1) to its sRGB
// encoding, the inverse of [SRGBToLinear].
func LinearToSRGB(c float64) float64 {
	if c <= 0.0031308 {
		return c * 12.92
	}
	return 1.055*math.Pow(c, 1/2.4) - 0.055
}

// XYZ is a colour in the CIE 1931 XYZ space (D65-referenced here). Y is
// luminance; for in-gamut linear sRGB the components lie roughly in [0, 1].
type XYZ struct{ X, Y, Z float64 }

// Lab is a colour in the CIE L*a*b* space: L* is lightness (0 black .. 100
// diffuse white), a* is the green(-)/red(+) axis and b* the blue(-)/yellow(+)
// axis.
type Lab struct{ L, A, B float64 }

// D65 reference white in CIE XYZ (2-degree observer), the white point sRGB is
// defined against.
var (
	d65Xn = 0.95047
	d65Yn = 1.0
	d65Zn = 1.08883
)

// LinearRGBToXYZ converts linear-light sRGB primaries (each 0..1) to CIE XYZ
// using the standard sRGB D65 matrix.
func LinearRGBToXYZ(r, g, b float64) XYZ {
	return XYZ{
		X: 0.4124564*r + 0.3575761*g + 0.1804375*b,
		Y: 0.2126729*r + 0.7151522*g + 0.0721750*b,
		Z: 0.0193339*r + 0.1191920*g + 0.9503041*b,
	}
}

// XYZToLinearRGB converts CIE XYZ to linear-light sRGB primaries using the
// inverse sRGB D65 matrix (values may fall outside [0, 1] for out-of-gamut
// colours).
func XYZToLinearRGB(c XYZ) (r, g, b float64) {
	r = 3.2404542*c.X - 1.5371385*c.Y - 0.4985314*c.Z
	g = -0.9692660*c.X + 1.8760108*c.Y + 0.0415560*c.Z
	b = 0.0556434*c.X - 0.2040259*c.Y + 1.0572252*c.Z
	return
}

// labDelta is the CIE break point 6/29; below t == labDelta^3 the L*a*b*
// transfer function is linear rather than a cube root.
const labDelta = 6.0 / 29.0

// labF is the forward CIE L*a*b* nonlinearity f(t).
func labF(t float64) float64 {
	if t > labDelta*labDelta*labDelta {
		return math.Cbrt(t)
	}
	return t/(3*labDelta*labDelta) + 4.0/29.0
}

// labFInv is the inverse of [labF].
func labFInv(t float64) float64 {
	if t > labDelta {
		return t * t * t
	}
	return 3 * labDelta * labDelta * (t - 4.0/29.0)
}

// XYZToLab converts CIE XYZ (D65) to CIE L*a*b*.
func XYZToLab(c XYZ) Lab {
	fx := labF(c.X / d65Xn)
	fy := labF(c.Y / d65Yn)
	fz := labF(c.Z / d65Zn)
	return Lab{
		L: 116*fy - 16,
		A: 500 * (fx - fy),
		B: 200 * (fy - fz),
	}
}

// LabToXYZ converts CIE L*a*b* back to CIE XYZ (D65), the inverse of
// [XYZToLab].
func LabToXYZ(l Lab) XYZ {
	fy := (l.L + 16) / 116
	fx := fy + l.A/500
	fz := fy - l.B/200
	return XYZ{
		X: d65Xn * labFInv(fx),
		Y: d65Yn * labFInv(fy),
		Z: d65Zn * labFInv(fz),
	}
}

// SRGBToLab converts an sRGB colour (each channel 0..1, gamma-encoded) to CIE
// L*a*b*, chaining sRGB->linear->XYZ->Lab.
func SRGBToLab(r, g, b float64) Lab {
	lr := SRGBToLinear(r)
	lg := SRGBToLinear(g)
	lb := SRGBToLinear(b)
	return XYZToLab(LinearRGBToXYZ(lr, lg, lb))
}

// LabToSRGB converts a CIE L*a*b* colour to gamma-encoded sRGB (each channel
// 0..1, not clamped to gamut), the inverse of [SRGBToLab].
func LabToSRGB(l Lab) (r, g, b float64) {
	lr, lg, lb := XYZToLinearRGB(LabToXYZ(l))
	return LinearToSRGB(lr), LinearToSRGB(lg), LinearToSRGB(lb)
}
