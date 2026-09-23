// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import (
	"encoding/binary"
	"fmt"
)

// This file reads the transform icc.go declines: the one held as a
// multi-dimensional lookup table. It is what a CMYK profile carries, because
// no matrix describes ink -- a printer's colorants are not additive and their
// overlaps are not sums -- and it is what a scanner profile carries for the
// same reason.
//
// A profile writes such a transform as an `A2B0`, `A2B1` or `A2B2` tag, one
// per rendering intent, and inside the tag the shape is always the same
// sandwich: a curve per input channel, a grid of sampled output colours, a
// curve per output channel. Reading it is reading three arrays and
// interpolating between grid points; there is no approximation in it, and the
// result is the profile's own answer rather than a stand-in for it.
//
// What is NOT here: the version 4 `mAB ` shape, which is a longer sandwich
// with a second set of curves and a matrix in the middle. A profile carrying
// only that is still declined, and [ErrICCNotArithmetic] still says so.

// An ICCIntent names one of the three transforms a profile may hold, which
// differ in what they do to colours the destination cannot show.
type ICCIntent int

// The rendering intents, numbered as ICC numbers them.
const (
	// ICCPerceptual compresses the whole gamut so that relations between
	// colours survive. ICC requires a lookup-table profile to carry it.
	ICCPerceptual ICCIntent = 0
	// ICCRelativeColorimetric keeps in-gamut colours exactly and clips the
	// rest. It is what PDF means by /RelativeColorimetric, and what a
	// reader that is asked for no intent in particular should use: it is
	// poppler's default and Acrobat's.
	ICCRelativeColorimetric ICCIntent = 1
	// ICCSaturation keeps colours vivid at the cost of their exact hue.
	ICCSaturation ICCIntent = 2
)

// An ICCPCS says what a table's output numbers mean.
type ICCPCS int

// The connection-space encodings.
const (
	// ICCPCSXYZ is CIE XYZ as u1Fixed15: 0x8000 stands for 1.0.
	ICCPCSXYZ ICCPCS = iota
	// ICCPCSLabV2 is the legacy 16-bit CIELAB encoding, in which 0xff00 --
	// not 0xffff -- stands for L* = 100. A profile written to version 2
	// uses it in a lookup table, and reading it as the version 4 encoding
	// shifts every lightness by a quarter of a level.
	ICCPCSLabV2
	// ICCPCSLabV4 is the version 4 CIELAB encoding, in which full scale
	// stands for L* = 100. It is also the 8-bit encoding, at every version.
	ICCPCSLabV4
)

// ICCLutProfile is an ICC profile whose transform is a lookup table: one per
// rendering intent, of which at least one is present.
//
// The tables are read whole at [ReadICC] time. A four-input table on an
// eleven-point grid -- the usual shape for a CMYK press profile -- is 14 641
// grid points, so a profile costs a few hundred kilobytes of memory and is
// worth holding on to rather than reading twice.
type ICCLutProfile struct {
	// Inputs is how many channels the profile takes: 4 for CMYK, 3 for a
	// scanner's RGB.
	Inputs int
	// Tables holds the transform for each intent, indexed by [ICCIntent].
	// An intent the profile does not carry is nil.
	Tables [3]*ICCLut
	// Black is the darkest colour the profile can reach, in D50-referenced
	// CIE XYZ, and the zero value when the profile does not say -- which is
	// also what a profile carrying no way back from the connection space
	// means here. [ICCLutProfile.ToSRGBCompensated] is what reads it.
	Black XYZ
}

// Table returns the transform for an intent, falling back to any the profile
// does carry when it does not carry that one. It returns nil only for a
// profile that holds no table at all, which [ReadICC] does not produce.
func (p *ICCLutProfile) Table(intent ICCIntent) *ICCLut {
	if intent >= 0 && int(intent) < len(p.Tables) && p.Tables[intent] != nil {
		return p.Tables[intent]
	}
	for _, t := range p.Tables {
		if t != nil {
			return t
		}
	}
	return nil
}

// ToSRGB converts one colour to gamma-encoded sRGB through the table for an
// intent, adapting the connection space's D50 to D65 and clamping to the
// gamut.
func (p *ICCLutProfile) ToSRGB(intent ICCIntent, in []float64) (r, g, b float64) {
	return p.Table(intent).ToSRGB(in)
}

// ICCLut is one lookup-table transform: input curves, a grid, output curves.
type ICCLut struct {
	// Inputs and Outputs are the channel counts either side.
	Inputs, Outputs int
	// Grid is how many points each input axis is sampled at, the same for
	// every axis in this shape of tag.
	Grid int
	// In and Out are the curves before and after the grid, one per channel.
	In, Out []Curve
	// Matrix is applied to the three inputs before their curves, and the
	// format requires it to be the identity unless the input is CIE XYZ.
	Matrix [9]float64
	// CLUT is the grid itself, normalised to [0, 1]: Grid^Inputs points of
	// Outputs values each, the FIRST input channel varying slowest.
	CLUT []float64
	// PCS says how to read the values that come out.
	PCS ICCPCS
}

// Eval runs one colour through the table and returns the encoded
// connection-space values, each in [0, 1].
func (l *ICCLut) Eval(in []float64) []float64 {
	v := make([]float64, l.Inputs)
	for i := range v {
		if i < len(in) {
			v[i] = clamp01(in[i])
		}
	}
	if l.Inputs == 3 {
		m := l.Matrix
		v = []float64{
			m[0]*v[0] + m[1]*v[1] + m[2]*v[2],
			m[3]*v[0] + m[4]*v[1] + m[5]*v[2],
			m[6]*v[0] + m[7]*v[1] + m[8]*v[2],
		}
	}

	// Locate the cell each input falls in, and how far across it.
	idx := make([]int, l.Inputs)
	frac := make([]float64, l.Inputs)
	for i := range v {
		t := clamp01(l.In[i].At(v[i])) * float64(l.Grid-1)
		k := int(t)
		if k > l.Grid-2 {
			k = l.Grid - 2
		}
		idx[i] = k
		frac[i] = t - float64(k)
	}

	// Read the grid the way little-cms does, because matching an engine
	// means matching how it reads between points and not only what it
	// stores: see [ICCLut.read].
	strides := make([]int, l.Inputs)
	base := 0
	for i := l.Inputs - 1; i >= 0; i-- {
		if i == l.Inputs-1 {
			strides[i] = l.Outputs
		} else {
			strides[i] = strides[i+1] * l.Grid
		}
	}
	for i := range strides {
		base += idx[i] * strides[i]
	}
	out := make([]float64, l.Outputs)
	l.read(base, strides, frac, out)
	for o := range out {
		out[o] = clamp01(l.Out[o].At(out[o]))
	}
	return out
}

// XYZ returns the D50-referenced CIE XYZ one colour stands for.
func (l *ICCLut) XYZ(in []float64) XYZ {
	o := l.Eval(in)
	switch l.PCS {
	case ICCPCSLabV2:
		// 0xff00 of 0xffff is L* = 100, so full scale overshoots.
		const s = 65535.0 / 65280.0
		return LabToXYZWP(Lab{L: o[0] * s * 100, A: o[1]*s*255 - 128, B: o[2]*s*255 - 128}, D50)
	case ICCPCSLabV4:
		return LabToXYZWP(Lab{L: o[0] * 100, A: o[1]*255 - 128, B: o[2]*255 - 128}, D50)
	}
	const s = 65535.0 / 32768.0
	return XYZ{X: o[0] * s, Y: o[1] * s, Z: o[2] * s}
}

// ToSRGB converts one colour to gamma-encoded sRGB, adapting from D50 to D65
// and clamping to the gamut.
func (l *ICCLut) ToSRGB(in []float64) (r, g, b float64) {
	return xyzToSRGB(Adapt(l.XYZ(in), D50, D65))
}

// Device runs one connection-space colour through a B-to-A table, whose input
// is the connection space and whose output is device values. It is the same
// machinery read the other way round, so [ICCLut.PCS] says how the colour is
// encoded on the way IN rather than on the way out.
func (l *ICCLut) Device(c Lab) []float64 {
	var in []float64
	switch l.PCS {
	case ICCPCSLabV2:
		const s = 65280.0 / 65535.0
		in = []float64{c.L / 100 * s, (c.A + 128) / 255 * s, (c.B + 128) / 255 * s}
	case ICCPCSLabV4:
		in = []float64{c.L / 100, (c.A + 128) / 255, (c.B + 128) / 255}
	default:
		x := LabToXYZWP(c, D50)
		const s = 32768.0 / 65535.0
		in = []float64{x.X * s, x.Y * s, x.Z * s}
	}
	return l.Eval(in)
}

// ToSRGBCompensated converts one colour the way little-cms does when it is
// asked for black point compensation, which is what poppler asks for on every
// transform it builds: the profile's own black is carried to the destination's
// -- nought, sRGB being a matrix and curves -- and the white is left where it
// is, everything between sliding along that line.
//
// Without it a press profile's darkest ink lands on the grey it truly
// measures, some way above black, and the whole picture is lifted with it.
func (p *ICCLutProfile) ToSRGBCompensated(intent ICCIntent, in []float64) (r, g, b float64) {
	if p.Black == (XYZ{}) {
		// Nothing to carry anywhere, and saying so here rather than
		// letting the arithmetic say it keeps the answer bit for bit
		// the one [ICCLutProfile.ToSRGB] gives.
		return p.ToSRGB(intent, in)
	}
	c := p.Table(intent).XYZ(in)
	f := func(v, black, white float64) float64 {
		if white == black {
			return v
		}
		return white * (v - black) / (white - black)
	}
	c = XYZ{
		X: f(c.X, p.Black.X, D50.X),
		Y: f(c.Y, p.Black.Y, D50.Y),
		Z: f(c.Z, p.Black.Z, D50.Z),
	}
	return xyzToSRGB(Adapt(c, D50, D65))
}

// iccBlackPoint is the darkest colour a profile reaches, found the way
// little-cms finds it for a printer: ask the profile what ink it would lay
// down for absolute black, then ask it what that ink actually measures.
//
// The two legs use DIFFERENT tables, and that is the whole of it. The way in
// is the perceptual B2A0, because only the perceptual table is asked for ink
// that cannot be reached; the way back is the media-relative A2B1, because the
// question is what that ink measures and not how it would be shown. Reading
// both legs perceptually answers L* = 0.5 for a coated press where the true
// answer is 10.7, which is a compensation that does nothing.
//
// The lightness is capped at 50 because a profile that answers something
// absurd should not be allowed to invert the picture, and the chroma is
// dropped because a black point is a point on the neutral axis by definition.
func iccBlackPoint(toDevice, toPCS *ICCLut) XYZ {
	if toDevice == nil || toPCS == nil || toDevice.Outputs != toPCS.Inputs {
		return XYZ{}
	}
	c := XYZToLabWP(toPCS.XYZ(toDevice.Device(Lab{})), D50)
	if c.L > 50 {
		c.L = 50
	}
	return LabToXYZWP(Lab{L: c.L}, D50)
}

// iccLut reads one A2B tag. An unknown shape is declined rather than guessed
// at, so that a profile carrying one readable intent and one unreadable is
// still read.
func iccLut(b []byte, s iccSpan) (*ICCLut, error) {
	bits := 0
	switch string(b[s.off : s.off+4]) {
	case "mft1":
		bits = 8
	case "mft2":
		bits = 16
	default:
		return nil, fmt.Errorf("%w: a %q lookup table", ErrICCNotArithmetic, string(b[s.off:s.off+4]))
	}
	pcs, err := iccPCS(b, bits)
	if err != nil {
		return nil, err
	}
	return iccLutTables(b, s, pcs, bits)
}

// iccLutTables reads an mft1 or mft2 tag, which differ only in the width of
// their numbers and in mft1 fixing both curve lengths at 256.
func iccLutTables(b []byte, s iccSpan, pcs ICCPCS, bits int) (*ICCLut, error) {
	if s.size < 52 {
		return nil, ErrICCMalformed
	}
	l := &ICCLut{
		Inputs:  int(b[s.off+8]),
		Outputs: int(b[s.off+9]),
		Grid:    int(b[s.off+10]),
		PCS:     pcs,
	}
	// An eight-input table would be 16 million grid points before anything
	// is read; the format's own ceiling is 15 and no real profile passes 4.
	if l.Inputs < 1 || l.Inputs > 8 || l.Outputs < 3 || l.Outputs > 8 || l.Grid < 2 {
		return nil, ErrICCMalformed
	}
	for i := range 9 {
		o := s.off + 12 + uint32(i)*4
		l.Matrix[i] = float64(int32(binary.BigEndian.Uint32(b[o:o+4]))) / 65536.0
	}

	inEntries, outEntries := 256, 256
	off := uint64(s.off) + 48
	if bits == 16 {
		inEntries = int(binary.BigEndian.Uint16(b[s.off+48 : s.off+50]))
		outEntries = int(binary.BigEndian.Uint16(b[s.off+50 : s.off+52]))
		if inEntries < 2 || outEntries < 2 {
			return nil, ErrICCMalformed
		}
		off = uint64(s.off) + 52
	}

	// Everything after the header is one run of numbers of the same width:
	// the input curves, the grid, then the output curves.
	width := uint64(bits / 8)
	points := uint64(1)
	for range l.Inputs {
		points *= uint64(l.Grid)
		if points > uint64(s.size) {
			return nil, ErrICCMalformed
		}
	}
	need := (uint64(l.Inputs*inEntries) + points*uint64(l.Outputs) + uint64(l.Outputs*outEntries)) * width
	if need > uint64(s.size)-(off-uint64(s.off)) {
		return nil, ErrICCMalformed
	}

	read := func(n int) []float64 {
		out := make([]float64, n)
		for i := range out {
			if bits == 8 {
				out[i] = float64(b[off]) / 255.0
			} else {
				out[i] = float64(binary.BigEndian.Uint16(b[off:off+2])) / 65535.0
			}
			off += width
		}
		return out
	}
	l.In = make([]Curve, l.Inputs)
	for i := range l.In {
		l.In[i] = SampledCurve(read(inEntries))
	}
	l.CLUT = read(int(points) * l.Outputs)
	l.Out = make([]Curve, l.Outputs)
	for i := range l.Out {
		l.Out[i] = SampledCurve(read(outEntries))
	}
	return l, nil
}

// iccPCS says which connection space a profile's header names, and how a
// lookup table of this version encodes it.
func iccPCS(b []byte, bits int) (ICCPCS, error) {
	if string(b[20:24]) != "Lab " {
		if string(b[20:24]) != "XYZ " {
			return 0, ErrICCMalformed
		}
		return ICCPCSXYZ, nil
	}
	// Eight-bit tables have always used the straightforward encoding; the
	// sixteen-bit one changed at version 4.
	if bits == 8 || b[8] >= 4 {
		return ICCPCSLabV4, nil
	}
	return ICCPCSLabV2, nil
}

// read interpolates the grid at one point, dividing the work the way
// little-cms divides it: one input linearly, two bilinearly, three
// tetrahedrally, and four or more linearly between the two sub-lattices of the
// first input with the rest read the same way again.
//
// The choice matters. Weighing all 2^n corners of a cell -- the obvious
// reading, and what this did first -- agrees with a tetrahedral reading at
// every grid point and differs between them: a quarter of a CIELAB unit on a
// press profile's grid of eleven, which is invisible in eight bits but is a
// difference from the engine every other measurement here is taken against.
func (l *ICCLut) read(base int, strides []int, frac []float64, out []float64) {
	switch len(frac) {
	case 1:
		for o := range out {
			lo := l.CLUT[base+o]
			out[o] = lo + (l.CLUT[base+strides[0]+o]-lo)*frac[0]
		}
	case 2:
		for o := range out {
			c00, c10 := l.CLUT[base+o], l.CLUT[base+strides[0]+o]
			c01, c11 := l.CLUT[base+strides[1]+o], l.CLUT[base+strides[0]+strides[1]+o]
			out[o] = c00*(1-frac[0])*(1-frac[1]) + c10*frac[0]*(1-frac[1]) +
				c01*(1-frac[0])*frac[1] + c11*frac[0]*frac[1]
		}
	case 3:
		l.tetra(base, strides, frac, out)
	default:
		lo := make([]float64, len(out))
		hi := make([]float64, len(out))
		l.read(base, strides[1:], frac[1:], lo)
		l.read(base+strides[0], strides[1:], frac[1:], hi)
		for o := range out {
			out[o] = lo[o] + (hi[o]-lo[o])*frac[0]
		}
	}
}

// tetra reads three dimensions of the grid by cutting the cell into six
// tetrahedra and weighing the four corners of the one the point falls in.
// Which tetrahedron that is, is decided by the ORDER of the three fractions,
// which is why this is six cases and not one expression.
func (l *ICCLut) tetra(base int, strides []int, frac []float64, out []float64) {
	rx, ry, rz := frac[0], frac[1], frac[2]
	at := func(x, y, z, o int) float64 {
		return l.CLUT[base+x*strides[0]+y*strides[1]+z*strides[2]+o]
	}
	for o := range out {
		c0 := at(0, 0, 0, o)
		var c1, c2, c3 float64
		switch {
		case rx >= ry && ry >= rz:
			c1 = at(1, 0, 0, o) - c0
			c2 = at(1, 1, 0, o) - at(1, 0, 0, o)
			c3 = at(1, 1, 1, o) - at(1, 1, 0, o)
		case rx >= rz && rz >= ry:
			c1 = at(1, 0, 0, o) - c0
			c2 = at(1, 1, 1, o) - at(1, 0, 1, o)
			c3 = at(1, 0, 1, o) - at(1, 0, 0, o)
		case rz >= rx && rx >= ry:
			c1 = at(1, 0, 1, o) - at(0, 0, 1, o)
			c2 = at(1, 1, 1, o) - at(1, 0, 1, o)
			c3 = at(0, 0, 1, o) - c0
		case ry >= rx && rx >= rz:
			c1 = at(1, 1, 0, o) - at(0, 1, 0, o)
			c2 = at(0, 1, 0, o) - c0
			c3 = at(1, 1, 1, o) - at(1, 1, 0, o)
		case ry >= rz && rz >= rx:
			c1 = at(1, 1, 1, o) - at(0, 1, 1, o)
			c2 = at(0, 1, 0, o) - c0
			c3 = at(0, 1, 1, o) - at(0, 1, 0, o)
		default:
			c1 = at(1, 1, 1, o) - at(0, 1, 1, o)
			c2 = at(0, 1, 1, o) - at(0, 0, 1, o)
			c3 = at(0, 0, 1, o) - c0
		}
		out[o] = c0 + c1*rx + c2*ry + c3*rz
	}
}
