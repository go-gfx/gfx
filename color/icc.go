// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import (
	"encoding/binary"
	"errors"
	"math"
)

// This file reads the part of an ICC profile that is arithmetic: a tone curve
// per channel and, for a colour profile, a matrix carrying the result to the
// profile connection space. That subset covers most RGB and grey profiles in
// the wild, and it is exactly the shape [CalRGB] and [CalGray] already
// describe -- a curve, a matrix and a white point -- so reading one is
// building one of those.
//
// It is NOT a colour-management engine. A profile whose transform is a
// multi-dimensional lookup table (an A2B0 tag, which is how CMYK and most
// scanner profiles are written) is declined rather than approximated: see
// [ErrICCNotArithmetic]. A caller that meets one has to fall back on whatever
// it did before, and know that it did.

// ErrICCNotArithmetic is returned by [ReadICC] for a profile this package
// cannot reduce to curves and a matrix -- most often one whose transform is a
// lookup table. It is not a malformed profile; it is a profile that needs an
// engine.
var ErrICCNotArithmetic = errors.New("color: ICC profile is not a matrix and tone curves")

// ErrICCMalformed is returned by [ReadICC] for bytes that are not an ICC
// profile, or are one that has been truncated.
var ErrICCMalformed = errors.New("color: malformed ICC profile")

// A Curve is one channel's tone curve: what a stored value means as a
// fraction of full intensity.
type Curve interface {
	// At maps a stored value in [0, 1] to linear intensity in [0, 1].
	At(v float64) float64
}

// GammaCurve raises its input to a power, which is how a profile writes a
// curve it can describe in one number.
type GammaCurve float64

// At implements [Curve].
func (g GammaCurve) At(v float64) float64 { return math.Pow(clamp01(v), float64(g)) }

// SampledCurve is a curve given as evenly spaced points, read between them by
// straight lines. ICC writes these as a `curv` tag of more than one entry.
type SampledCurve []float64

// At implements [Curve]. A curve of one point or none is the identity, which
// is what the format means by an empty `curv`.
func (s SampledCurve) At(v float64) float64 {
	if len(s) < 2 {
		return clamp01(v)
	}
	x := clamp01(v) * float64(len(s)-1)
	i := int(x)
	if i >= len(s)-1 {
		return s[len(s)-1]
	}
	f := x - float64(i)
	return s[i]*(1-f) + s[i+1]*f
}

// ICCMatrixTRC is a three-channel ICC profile reduced to arithmetic: a tone
// curve per channel and the matrix carrying the results to CIE XYZ.
//
// The matrix is in the profile connection space, which ICC fixes at D50, so
// the colorants a profile stores are already adapted there whatever its
// recorded media white point says. [ICCMatrixTRC.ToSRGB] therefore adapts from
// D50, not from the white point tag.
type ICCMatrixTRC struct {
	// Curves are the per-channel tone curves, in channel order.
	Curves [3]Curve
	// Matrix carries the three decoded channels to D50-referenced CIE XYZ,
	// by ROW: X from the three, then Y, then Z.
	Matrix [9]float64
}

// XYZ returns the D50-referenced CIE XYZ the three stored values stand for.
func (p ICCMatrixTRC) XYZ(a, b, c float64) XYZ {
	v := [3]float64{p.Curves[0].At(a), p.Curves[1].At(b), p.Curves[2].At(c)}
	m := p.Matrix
	return XYZ{
		X: m[0]*v[0] + m[1]*v[1] + m[2]*v[2],
		Y: m[3]*v[0] + m[4]*v[1] + m[5]*v[2],
		Z: m[6]*v[0] + m[7]*v[1] + m[8]*v[2],
	}
}

// ToSRGB converts three stored values to gamma-encoded sRGB, adapting from the
// connection space's D50 to D65 and clamping to the gamut.
func (p ICCMatrixTRC) ToSRGB(a, b, c float64) (r, g, bb float64) {
	return xyzToSRGB(Adapt(p.XYZ(a, b, c), D50, D65))
}

// ICCGrayTRC is a one-channel ICC profile reduced to arithmetic: one tone
// curve, whose output is luminance against the profile's media white point.
type ICCGrayTRC struct {
	// Curve maps the stored value to luminance.
	Curve Curve
	// White is the media white point the luminance is relative to.
	White WhitePoint
}

// XYZ returns the CIE XYZ the stored value stands for, under the profile's own
// white point.
func (p ICCGrayTRC) XYZ(v float64) XYZ {
	y := p.Curve.At(v)
	return XYZ{X: p.White.X * y, Y: p.White.Y * y, Z: p.White.Z * y}
}

// ToSRGB converts one stored value to gamma-encoded sRGB, adapting from the
// profile's white point to D65 and clamping to the gamut.
func (p ICCGrayTRC) ToSRGB(v float64) (r, g, b float64) {
	return xyzToSRGB(Adapt(p.XYZ(v), p.White, D65))
}

// ReadICC reads an ICC profile and returns it as arithmetic: an
// [*ICCMatrixTRC] for a three-channel profile, an [*ICCGrayTRC] for a
// one-channel one.
//
// It returns [ErrICCNotArithmetic] for a profile that cannot be reduced that
// way -- which a caller must treat as "I cannot read this one" rather than as
// a failure, because such a profile is perfectly valid and simply needs a
// colour-management engine.
func ReadICC(b []byte) (any, error) {
	tags, err := iccTags(b)
	if err != nil {
		return nil, err
	}
	// A lookup-table transform takes precedence in the format, so a profile
	// that has one is not ours to read even if it also carries colorants.
	for _, t := range []string{"A2B0", "A2B1", "A2B2"} {
		if _, ok := tags[t]; ok {
			return nil, ErrICCNotArithmetic
		}
	}

	if _, ok := tags["kTRC"]; ok {
		curve, err := iccCurve(b, tags["kTRC"])
		if err != nil {
			return nil, err
		}
		white, err := iccXYZ(b, tags["wtpt"])
		if err != nil {
			return nil, err
		}
		if white.Y <= 0 {
			return nil, ErrICCMalformed
		}
		return &ICCGrayTRC{Curve: curve, White: WhitePoint(white)}, nil
	}

	var p ICCMatrixTRC
	cols := [3]XYZ{}
	for i, name := range [3]string{"rXYZ", "gXYZ", "bXYZ"} {
		span, ok := tags[name]
		if !ok {
			return nil, ErrICCNotArithmetic
		}
		c, err := iccXYZ(b, span)
		if err != nil {
			return nil, err
		}
		cols[i] = c
	}
	for i, name := range [3]string{"rTRC", "gTRC", "bTRC"} {
		span, ok := tags[name]
		if !ok {
			return nil, ErrICCNotArithmetic
		}
		c, err := iccCurve(b, span)
		if err != nil {
			return nil, err
		}
		p.Curves[i] = c
	}
	// The colorants are the COLUMNS of the matrix: each is where one channel
	// alone lands in XYZ.
	p.Matrix = [9]float64{
		cols[0].X, cols[1].X, cols[2].X,
		cols[0].Y, cols[1].Y, cols[2].Y,
		cols[0].Z, cols[1].Z, cols[2].Z,
	}
	return &p, nil
}

// iccSpan is where a tag's data sits in the profile.
type iccSpan struct{ off, size uint32 }

// iccTags reads the tag table, checking that the profile says it is as long as
// it is and that every tag lies inside it.
func iccTags(b []byte) (map[string]iccSpan, error) {
	if len(b) < 132 {
		return nil, ErrICCMalformed
	}
	if size := binary.BigEndian.Uint32(b[0:4]); int(size) > len(b) {
		return nil, ErrICCMalformed
	}
	if string(b[36:40]) != "acsp" {
		return nil, ErrICCMalformed
	}
	n := binary.BigEndian.Uint32(b[128:132])
	if n > 1024 || 132+int(n)*12 > len(b) {
		return nil, ErrICCMalformed
	}
	out := make(map[string]iccSpan, n)
	for i := range int(n) {
		e := 132 + i*12
		sig := string(b[e : e+4])
		off := binary.BigEndian.Uint32(b[e+4 : e+8])
		size := binary.BigEndian.Uint32(b[e+8 : e+12])
		if int(off)+int(size) > len(b) || size < 8 {
			return nil, ErrICCMalformed
		}
		out[sig] = iccSpan{off, size}
	}
	return out, nil
}

// iccXYZ reads an XYZType tag, whose numbers are s15Fixed16.
func iccXYZ(b []byte, s iccSpan) (XYZ, error) {
	if s.size < 20 || string(b[s.off:s.off+4]) != "XYZ " {
		return XYZ{}, ErrICCMalformed
	}
	f := func(o uint32) float64 {
		return float64(int32(binary.BigEndian.Uint32(b[o:o+4]))) / 65536.0
	}
	return XYZ{X: f(s.off + 8), Y: f(s.off + 12), Z: f(s.off + 16)}, nil
}

// iccCurve reads a curveType tag. Nought points is the identity, one point is
// a gamma written as u8Fixed8, and more are the curve itself.
//
// A parametricCurveType is declined rather than guessed at: it is a different
// tag with its own five shapes, and returning the wrong one silently would be
// worse than saying no.
func iccCurve(b []byte, s iccSpan) (Curve, error) {
	switch string(b[s.off : s.off+4]) {
	case "curv":
		if s.size < 12 {
			return nil, ErrICCMalformed
		}
		n := binary.BigEndian.Uint32(b[s.off+8 : s.off+12])
		if uint64(s.size) < 12+uint64(n)*2 {
			return nil, ErrICCMalformed
		}
		switch n {
		case 0:
			return GammaCurve(1), nil
		case 1:
			return GammaCurve(float64(binary.BigEndian.Uint16(b[s.off+12:s.off+14])) / 256.0), nil
		}
		pts := make(SampledCurve, n)
		for i := range pts {
			o := s.off + 12 + uint32(i)*2
			pts[i] = float64(binary.BigEndian.Uint16(b[o:o+2])) / 65535.0
		}
		return pts, nil
	case "para":
		return nil, ErrICCNotArithmetic
	}
	return nil, ErrICCNotArithmetic
}
