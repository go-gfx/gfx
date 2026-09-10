// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import "math"

// This file provides the two calibrated grey and RGB spaces PDF defines
// (ISO 32000-2, 8.6.5.2 and 8.6.5.3) and PostScript before it. Both describe
// their colours by saying what CIE XYZ a component triple stands for, under a
// white point of their own choosing, so reaching sRGB means undoing a gamma,
// applying a matrix, carrying the result to D65 and encoding it.
//
// A calibrated space is NOT its device namesake. CalGray with the default
// gamma of 1 says its component is linear luminance, so a mid grey of 0.5
// encodes to 188 rather than the 128 a DeviceGray sample would; a CalRGB with
// an Adobe RGB matrix moves a saturated red by a hundred levels. Treating
// either as the device space drops the calibration entirely, which is the one
// thing the space exists to carry.

// CalGray is a PDF CalGray colour space: a white point and the gamma by which
// its single component is encoded.
//
// The zero value is not usable — a white point of (0, 0, 0) has no colour in
// it. Build one with [NewCalGray], which carries the defaults the format
// specifies, then set what the dictionary gives.
type CalGray struct {
	// White is the space's white point, the /WhitePoint entry. The format
	// requires it; there is no default.
	White WhitePoint
	// Gamma is the exponent by which the component is encoded, the /Gamma
	// entry. It defaults to 1, which means the component is linear luminance.
	Gamma float64
}

// NewCalGray returns a CalGray with the given white point and the default
// gamma of 1.
func NewCalGray(white WhitePoint) CalGray { return CalGray{White: white, Gamma: 1} }

// XYZ returns the CIE XYZ colour the component a stands for, under the space's
// own white point. It is the A^gamma scaling of that white point.
func (s CalGray) XYZ(a float64) XYZ {
	v := math.Pow(clamp01(a), s.Gamma)
	return XYZ{X: s.White.X * v, Y: s.White.Y * v, Z: s.White.Z * v}
}

// CalGrayToSRGB converts one component of a CalGray space to gamma-encoded
// sRGB, adapting from the space's white point to D65.
//
// The three results are equal only when the white point is D65; under any
// other illuminant a neutral grey acquires a cast, which is what the space is
// saying about it.
func CalGrayToSRGB(s CalGray, a float64) (r, g, b float64) {
	return xyzToSRGB(Adapt(s.XYZ(a), s.White, D65))
}

// CalRGB is a PDF CalRGB colour space: a white point, a per-component gamma
// and the matrix carrying the decoded components to CIE XYZ.
//
// The zero value is not usable. Build one with [NewCalRGB], which carries the
// defaults the format specifies, then set what the dictionary gives.
type CalRGB struct {
	// White is the space's white point, the /WhitePoint entry. The format
	// requires it; there is no default.
	White WhitePoint
	// Gamma holds the three exponents by which the components are encoded,
	// the /Gamma entry. Each defaults to 1.
	Gamma [3]float64
	// Matrix is the /Matrix entry in the order the format writes it, which is
	// by COLUMN: XA YA ZA XB YB ZB XC YC ZC. So Matrix[0], Matrix[3] and
	// Matrix[6] are the three contributions to X. It defaults to the identity.
	Matrix [9]float64
}

// NewCalRGB returns a CalRGB with the given white point, the default gamma of
// 1 on each component and the identity matrix.
func NewCalRGB(white WhitePoint) CalRGB {
	return CalRGB{
		White:  white,
		Gamma:  [3]float64{1, 1, 1},
		Matrix: [9]float64{1, 0, 0, 0, 1, 0, 0, 0, 1},
	}
}

// XYZ returns the CIE XYZ colour the components (a, b, c) stand for, under the
// space's own white point.
func (s CalRGB) XYZ(a, b, c float64) XYZ {
	da := math.Pow(clamp01(a), s.Gamma[0])
	db := math.Pow(clamp01(b), s.Gamma[1])
	dc := math.Pow(clamp01(c), s.Gamma[2])
	m := s.Matrix
	return XYZ{
		X: m[0]*da + m[3]*db + m[6]*dc,
		Y: m[1]*da + m[4]*db + m[7]*dc,
		Z: m[2]*da + m[5]*db + m[8]*dc,
	}
}

// CalRGBToSRGB converts a CalRGB colour to gamma-encoded sRGB, adapting from
// the space's white point to D65.
func CalRGBToSRGB(s CalRGB, a, b, c float64) (r, g, bb float64) {
	return xyzToSRGB(Adapt(s.XYZ(a, b, c), s.White, D65))
}

// xyzToSRGB carries a D65-referenced CIE XYZ colour to gamma-encoded sRGB,
// clamping to the gamut on the way. A calibrated space may well name a colour
// sRGB cannot show — an Adobe RGB primary is the ordinary case — and the clamp
// is where that colour is lost, deliberately and at the last step so the
// matrix never sees it.
func xyzToSRGB(c XYZ) (r, g, b float64) {
	lr, lg, lb := XYZToLinearRGB(c)
	return LinearToSRGB(clamp01(lr)), LinearToSRGB(clamp01(lg)), LinearToSRGB(clamp01(lb))
}
