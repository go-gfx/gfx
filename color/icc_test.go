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
	"testing"
)

// iccBuilder writes a real ICC byte stream, so the tests below exercise the
// PARSER as well as the arithmetic. A test that called the types directly
// would say nothing about whether a profile on disk is read correctly.
type iccBuilder struct {
	space, pcs string
	tags       []iccTagIn
}

type iccTagIn struct {
	sig  string
	data []byte
}

func (b *iccBuilder) xyz(sig string, x, y, z float64) {
	d := make([]byte, 20)
	copy(d, "XYZ ")
	for i, v := range []float64{x, y, z} {
		binary.BigEndian.PutUint32(d[8+i*4:], uint32(int32(math.Round(v*65536))))
	}
	b.tags = append(b.tags, iccTagIn{sig, d})
}

func (b *iccBuilder) gamma(sig string, g float64) {
	d := make([]byte, 14)
	copy(d, "curv")
	binary.BigEndian.PutUint32(d[8:], 1)
	binary.BigEndian.PutUint16(d[12:], uint16(math.Round(g*256)))
	b.tags = append(b.tags, iccTagIn{sig, d})
}

func (b *iccBuilder) sampled(sig string, pts []uint16) {
	d := make([]byte, 12+len(pts)*2)
	copy(d, "curv")
	binary.BigEndian.PutUint32(d[8:], uint32(len(pts)))
	for i, p := range pts {
		binary.BigEndian.PutUint16(d[12+i*2:], p)
	}
	b.tags = append(b.tags, iccTagIn{sig, d})
}

func (b *iccBuilder) raw(sig string, typ string, extra int) {
	d := make([]byte, 8+extra)
	copy(d, typ)
	b.tags = append(b.tags, iccTagIn{sig, d})
}

// raw2 appends a tag whose bytes the caller has already written whole.
func (b *iccBuilder) raw2(sig string, d []byte) {
	b.tags = append(b.tags, iccTagIn{sig, d})
}

func (b *iccBuilder) bytes() []byte {
	head := make([]byte, 132)
	copy(head[16:20], b.space)
	copy(head[20:24], b.pcs)
	copy(head[36:40], "acsp")
	binary.BigEndian.PutUint32(head[128:], uint32(len(b.tags)))
	table := make([]byte, len(b.tags)*12)
	off := len(head) + len(table)
	var body []byte
	for i, t := range b.tags {
		copy(table[i*12:], t.sig)
		binary.BigEndian.PutUint32(table[i*12+4:], uint32(off+len(body)))
		binary.BigEndian.PutUint32(table[i*12+8:], uint32(len(t.data)))
		body = append(body, t.data...)
	}
	out := append(append(head, table...), body...)
	binary.BigEndian.PutUint32(out[0:4], uint32(len(out)))
	return out
}

// appleRGB is the profile carried by
// pdfscans/ia-medical/2011001RegenerativeEndodonticsPart2.pdf: gamma 1.8008
// on each channel and colorants that sum to D50, as the connection space
// requires. It is a real document's real profile rather than one chosen to be
// easy.
func appleRGB() []byte {
	b := &iccBuilder{space: "RGB ", pcs: "XYZ "}
	b.xyz("wtpt", 0.950455, 1.0, 1.089050)
	b.gamma("rTRC", 1.8008)
	b.gamma("gTRC", 1.8008)
	b.gamma("bTRC", 1.8008)
	b.xyz("rXYZ", 0.475540, 0.255157, 0.018448)
	b.xyz("gXYZ", 0.339722, 0.672592, 0.113327)
	b.xyz("bXYZ", 0.148956, 0.072250, 0.693115)
	return b.bytes()
}

func byteOfSRGB(v float64) int {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return int(v*255 + 0.5)
}

// TestAMatrixTRCProfileAgreesWithLittleCMS.
//
// The expected values are poppler's, which is little-cms: a sixteen-pixel
// image of these samples, in a PDF carrying exactly this profile, extracted
// with `pdfimages -png`. They are measurements, not this package's own output
// written down.
func TestAMatrixTRCProfileAgreesWithLittleCMS(t *testing.T) {
	p, err := ReadICC(appleRGB())
	if err != nil {
		t.Fatalf("ReadICC: %v", err)
	}
	m, ok := p.(*ICCMatrixTRC)
	if !ok {
		t.Fatalf("read as %T, want *ICCMatrixTRC", p)
	}
	for _, c := range []struct{ in, want [3]int }{
		{[3]int{255, 0, 0}, [3]int{255, 43, 6}},
		{[3]int{0, 255, 0}, [3]int{0, 250, 48}},
		{[3]int{0, 0, 255}, [3]int{25, 34, 251}},
		{[3]int{255, 255, 0}, [3]int{254, 253, 50}},
		{[3]int{0, 255, 255}, [3]int{0, 252, 255}},
		{[3]int{255, 0, 255}, [3]int{255, 56, 252}},
		{[3]int{128, 0, 0}, [3]int{151, 20, 2}},
		{[3]int{0, 128, 0}, [3]int{0, 144, 23}},
		{[3]int{0, 0, 128}, [3]int{9, 14, 144}},
		{[3]int{192, 96, 32}, [3]int{208, 118, 48}},
		{[3]int{32, 96, 192}, [3]int{36, 116, 201}},
		{[3]int{96, 192, 32}, [3]int{104, 200, 57}},
		{[3]int{64, 64, 192}, [3]int{84, 85, 201}},
		{[3]int{192, 64, 64}, [3]int{209, 87, 82}},
		{[3]int{20, 200, 120}, [3]int{0, 207, 142}},
		{[3]int{200, 20, 120}, [3]int{217, 48, 137}},
		// And the neutral axis, from the picture the profile came out of.
		{[3]int{17, 17, 17}, [3]int{21, 21, 21}},
		{[3]int{104, 104, 103}, [3]int{123, 123, 122}},
		{[3]int{183, 183, 183}, [3]int{196, 196, 196}},
		{[3]int{255, 255, 255}, [3]int{255, 255, 255}},
		{[3]int{0, 0, 0}, [3]int{0, 0, 0}},
	} {
		r, g, b := m.ToSRGB(float64(c.in[0])/255, float64(c.in[1])/255, float64(c.in[2])/255)
		got := [3]int{byteOfSRGB(r), byteOfSRGB(g), byteOfSRGB(b)}
		for i := range 3 {
			if d := got[i] - c.want[i]; d > 1 || d < -1 {
				t.Errorf("%v channel %d = %d, little-cms says %d", c.in, i, got[i], c.want[i])
			}
		}
	}
}

// TestAProfileIsNotItsDeviceNamesake: the defect this replaces. Reading the
// samples as though they were sRGB is what a caller does when it cannot read
// the profile, and it is wrong by nineteen levels on the picture above.
func TestAProfileIsNotItsDeviceNamesake(t *testing.T) {
	p, _ := ReadICC(appleRGB())
	m := p.(*ICCMatrixTRC)
	worst := 0
	for v := 0; v <= 255; v += 5 {
		f := float64(v) / 255
		r, _, _ := m.ToSRGB(f, f, f)
		if d := byteOfSRGB(r) - v; d > worst {
			worst = d
		}
	}
	if worst < 15 {
		t.Errorf("the profile moves a neutral by at most %d levels; it should be worth reading", worst)
	}
}

func TestANeutralStaysNeutral(t *testing.T) {
	p, _ := ReadICC(appleRGB())
	m := p.(*ICCMatrixTRC)
	for v := 0; v <= 255; v += 17 {
		f := float64(v) / 255
		r, g, b := m.ToSRGB(f, f, f)
		hi, lo := byteOfSRGB(r), byteOfSRGB(r)
		for _, x := range []int{byteOfSRGB(g), byteOfSRGB(b)} {
			if x > hi {
				hi = x
			}
			if x < lo {
				lo = x
			}
		}
		if hi-lo > 1 {
			t.Errorf("a neutral of %d came back with a cast: %d %d %d", v, byteOfSRGB(r), byteOfSRGB(g), byteOfSRGB(b))
		}
	}
}

func TestAGreyProfileIsReadAsLuminance(t *testing.T) {
	b := &iccBuilder{space: "GRAY", pcs: "XYZ "}
	b.xyz("wtpt", 0.9642, 1.0, 0.8249)
	b.gamma("kTRC", 2.2)
	p, err := ReadICC(b.bytes())
	if err != nil {
		t.Fatalf("ReadICC: %v", err)
	}
	g, ok := p.(*ICCGrayTRC)
	if !ok {
		t.Fatalf("read as %T, want *ICCGrayTRC", p)
	}
	// Full scale is the white point itself, so it must come back white.
	r, gg, bb := g.ToSRGB(1)
	if byteOfSRGB(r) != 255 || byteOfSRGB(gg) != 255 || byteOfSRGB(bb) != 255 {
		t.Errorf("full scale = %d %d %d, want white", byteOfSRGB(r), byteOfSRGB(gg), byteOfSRGB(bb))
	}
	if r, _, _ := g.ToSRGB(0); byteOfSRGB(r) != 0 {
		t.Errorf("nought = %d, want black", byteOfSRGB(r))
	}
	// With a gamma of 2.2 the mid tone lands near the sRGB mid, not at 188.
	if r, _, _ := g.ToSRGB(0.5); byteOfSRGB(r) < 120 || byteOfSRGB(r) > 136 {
		t.Errorf("mid tone = %d, want it near the sRGB mid", byteOfSRGB(r))
	}
}

func TestASampledCurveIsReadBetweenItsPoints(t *testing.T) {
	b := &iccBuilder{space: "GRAY", pcs: "XYZ "}
	b.xyz("wtpt", 0.9642, 1.0, 0.8249)
	// A curve that is flat at nought for the first half and then rises.
	pts := make([]uint16, 256)
	for i := range pts {
		if i >= 128 {
			pts[i] = uint16((i - 128) * 65535 / 127)
		}
	}
	b.sampled("kTRC", pts)
	p, err := ReadICC(b.bytes())
	if err != nil {
		t.Fatalf("ReadICC: %v", err)
	}
	g := p.(*ICCGrayTRC)
	if r, _, _ := g.ToSRGB(0.25); byteOfSRGB(r) != 0 {
		t.Errorf("a quarter in, on the flat half, = %d, want 0", byteOfSRGB(r))
	}
	if r, _, _ := g.ToSRGB(1); byteOfSRGB(r) != 255 {
		t.Errorf("full scale = %d, want 255", byteOfSRGB(r))
	}
	// And between two points it interpolates rather than stepping.
	a, _, _ := g.ToSRGB(0.75)
	c, _, _ := g.ToSRGB(0.76)
	if byteOfSRGB(a) == byteOfSRGB(c) {
		t.Error("two nearby inputs gave the same output; the curve is stepping, not interpolating")
	}
}

func TestAnEmptyCurveIsTheIdentity(t *testing.T) {
	b := &iccBuilder{space: "GRAY", pcs: "XYZ "}
	b.xyz("wtpt", 0.9505, 1.0, 1.0890)
	b.sampled("kTRC", nil)
	p, err := ReadICC(b.bytes())
	if err != nil {
		t.Fatalf("ReadICC: %v", err)
	}
	g := p.(*ICCGrayTRC)
	// Identity curve under D65: the stored value IS luminance, so a half
	// encodes to the sRGB of linear 0.5, which is 188 and not 128.
	if r, _, _ := g.ToSRGB(0.5); byteOfSRGB(r) < 180 || byteOfSRGB(r) > 195 {
		t.Errorf("mid = %d, want ~188: an empty curve is the identity, not a gamma", byteOfSRGB(r))
	}
}

// TestAProfileThatNeedsAnEngineIsDeclined. Saying no is the whole point: a
// profile approximated by a matrix it is not would be wrong silently.
//
// The shapes left here are the ones icclut.go does not read either. An `mft1`
// or `mft2` lookup table USED to be on this list and is not any more: it is
// read, and what it reads is measured in icclut_test.go.
func TestAProfileThatNeedsAnEngineIsDeclined(t *testing.T) {
	for name, build := range map[string]func() []byte{
		"a lookup table beside colorants": func() []byte {
			b := &iccBuilder{space: "RGB ", pcs: "XYZ "}
			b.xyz("rXYZ", 0.4, 0.2, 0.0)
			b.xyz("gXYZ", 0.3, 0.7, 0.1)
			b.xyz("bXYZ", 0.2, 0.1, 0.7)
			b.gamma("rTRC", 2.2)
			b.gamma("gTRC", 2.2)
			b.gamma("bTRC", 2.2)
			b.raw("A2B0", "mAB ", 64)
			return b.bytes()
		},
		"a parametric curve of a shape ICC does not define": func() []byte {
			// The five it DOES define are read; see
			// TestEveryParametricShapeIsReadAsItsOwnFormula.
			b := &iccBuilder{space: "RGB ", pcs: "XYZ "}
			b.xyz("rXYZ", 0.4, 0.2, 0.0)
			b.xyz("gXYZ", 0.3, 0.7, 0.1)
			b.xyz("bXYZ", 0.2, 0.1, 0.7)
			b.raw2("rTRC", paraTag(7, 1))
			b.raw2("gTRC", paraTag(7, 1))
			b.raw2("bTRC", paraTag(7, 1))
			return b.bytes()
		},
		"no colorants at all": func() []byte {
			b := &iccBuilder{space: "RGB ", pcs: "XYZ "}
			b.gamma("rTRC", 2.2)
			return b.bytes()
		},
	} {
		if _, err := ReadICC(build()); !errors.Is(err, ErrICCNotArithmetic) {
			t.Errorf("%s: err = %v, want ErrICCNotArithmetic", name, err)
		}
	}
}

func TestBytesThatAreNotAProfile(t *testing.T) {
	good := appleRGB()
	for name, b := range map[string][]byte{
		"nothing":              nil,
		"a header and no more": good[:100],
		"no acsp signature": func() []byte {
			c := append([]byte(nil), good...)
			copy(c[36:40], "xxxx")
			return c
		}(),
		"a size larger than the bytes": func() []byte {
			c := append([]byte(nil), good...)
			binary.BigEndian.PutUint32(c[0:4], uint32(len(c)+1))
			return c
		}(),
		"a tag reaching past the end": func() []byte {
			c := append([]byte(nil), good...)
			binary.BigEndian.PutUint32(c[132+8:], uint32(len(c)))
			return c
		}(),
		"an impossible tag count": func() []byte {
			c := append([]byte(nil), good...)
			binary.BigEndian.PutUint32(c[128:], 100000)
			return c
		}(),
		// rXYZ, not wtpt: an RGB profile's white point is never read, because
		// the connection space fixes it at D50 and the colorants are already
		// adapted there. Corrupting a tag nobody reads would prove nothing.
		"an XYZ tag that is not one": func() []byte {
			c := append([]byte(nil), good...)
			for i := range 7 {
				e := 132 + i*12
				if string(c[e:e+4]) == "rXYZ" {
					off := binary.BigEndian.Uint32(c[e+4:])
					copy(c[off:off+4], "curv")
					break
				}
			}
			return c
		}(),
		"an XYZ tag too short to hold three numbers": func() []byte {
			c := append([]byte(nil), good...)
			for i := range 7 {
				e := 132 + i*12
				if string(c[e:e+4]) == "gXYZ" {
					binary.BigEndian.PutUint32(c[e+8:], 12)
					break
				}
			}
			return c
		}(),
	} {
		if _, err := ReadICC(b); err == nil {
			t.Errorf("%s: read without error", name)
		}
	}
}

func TestAGreyProfileWithNoLightInIt(t *testing.T) {
	b := &iccBuilder{space: "GRAY", pcs: "XYZ "}
	b.xyz("wtpt", 0, 0, 0)
	b.gamma("kTRC", 2.2)
	if _, err := ReadICC(b.bytes()); !errors.Is(err, ErrICCMalformed) {
		t.Errorf("a white point of nought was accepted: %v", err)
	}
}

func TestACurveClampsWhatIsOutsideItsRange(t *testing.T) {
	if got := GammaCurve(2.2).At(-1); got != 0 {
		t.Errorf("below the range = %v, want 0", got)
	}
	if got := GammaCurve(2.2).At(2); got != 1 {
		t.Errorf("above the range = %v, want 1", got)
	}
	s := SampledCurve{0, 0.5, 1}
	if got := s.At(2); got != 1 {
		t.Errorf("sampled above the range = %v, want 1", got)
	}
	if got := s.At(0.5); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("sampled at the middle point = %v, want 0.5", got)
	}
	if got := (SampledCurve{0.25}).At(0.9); got != 0.9 {
		t.Errorf("a one-point curve = %v, want the identity", got)
	}
}

// TestEveryWayAProfileCanBeMalformedInATagThatIsRead. The cases above cover
// the header and the tag table; these are the tags themselves, one refusal
// per reachable path.
func TestEveryWayAProfileCanBeMalformedInATagThatIsRead(t *testing.T) {
	grey := func(mut func(*iccBuilder)) []byte {
		b := &iccBuilder{space: "GRAY", pcs: "XYZ "}
		b.xyz("wtpt", 0.9642, 1.0, 0.8249)
		b.gamma("kTRC", 2.2)
		mut(b)
		return b.bytes()
	}
	rgb := func(mut func(*iccBuilder)) []byte {
		b := &iccBuilder{space: "RGB ", pcs: "XYZ "}
		b.xyz("rXYZ", 0.4, 0.2, 0.0)
		b.xyz("gXYZ", 0.3, 0.7, 0.1)
		b.xyz("bXYZ", 0.2, 0.1, 0.7)
		b.gamma("rTRC", 2.2)
		b.gamma("gTRC", 2.2)
		b.gamma("bTRC", 2.2)
		mut(b)
		return b.bytes()
	}
	// Replacing a tag's data in place, after the builder has laid it out.
	retype := func(b []byte, sig, typ string) []byte {
		c := append([]byte(nil), b...)
		n := int(binary.BigEndian.Uint32(c[128:]))
		for i := range n {
			e := 132 + i*12
			if string(c[e:e+4]) == sig {
				off := binary.BigEndian.Uint32(c[e+4:])
				copy(c[off:off+4], typ)
				return c
			}
		}
		t.Fatalf("tag %s not in the profile", sig)
		return nil
	}
	shrink := func(b []byte, sig string, size uint32) []byte {
		c := append([]byte(nil), b...)
		n := int(binary.BigEndian.Uint32(c[128:]))
		for i := range n {
			e := 132 + i*12
			if string(c[e:e+4]) == sig {
				binary.BigEndian.PutUint32(c[e+8:], size)
				return c
			}
		}
		t.Fatalf("tag %s not in the profile", sig)
		return nil
	}
	claim := func(b []byte, sig string, count uint32) []byte {
		c := append([]byte(nil), b...)
		n := int(binary.BigEndian.Uint32(c[128:]))
		for i := range n {
			e := 132 + i*12
			if string(c[e:e+4]) == sig {
				off := binary.BigEndian.Uint32(c[e+4:])
				binary.BigEndian.PutUint32(c[off+8:], count)
				return c
			}
		}
		t.Fatalf("tag %s not in the profile", sig)
		return nil
	}
	for name, tc := range map[string]struct {
		profile []byte
		want    error
	}{
		"a grey curve of an unknown type":       {retype(grey(func(*iccBuilder) {}), "kTRC", "zzzz"), ErrICCNotArithmetic},
		"a grey curve too short for its header": {shrink(grey(func(*iccBuilder) {}), "kTRC", 10), ErrICCMalformed},
		// One kTRC only: two tags of the same signature would leave the
		// corrupted one unread, which is a test that proves nothing.
		"a grey curve claiming more points than it has": {claim(func() []byte {
			b := &iccBuilder{space: "GRAY", pcs: "XYZ "}
			b.xyz("wtpt", 0.9642, 1.0, 0.8249)
			b.sampled("kTRC", []uint16{0, 1000, 65535})
			return b.bytes()
		}(), "kTRC", 9999), ErrICCMalformed},
		"a grey profile with no white point": {func() []byte {
			b := &iccBuilder{space: "GRAY", pcs: "XYZ "}
			b.gamma("kTRC", 2.2)
			return b.bytes()
		}(), ErrICCMalformed},
		"a colour curve of an unknown type": {retype(rgb(func(*iccBuilder) {}), "gTRC", "zzzz"), ErrICCNotArithmetic},
		"a colour profile missing one curve": {func() []byte {
			b := &iccBuilder{space: "RGB ", pcs: "XYZ "}
			b.xyz("rXYZ", 0.4, 0.2, 0.0)
			b.xyz("gXYZ", 0.3, 0.7, 0.1)
			b.xyz("bXYZ", 0.2, 0.1, 0.7)
			b.gamma("rTRC", 2.2)
			b.gamma("gTRC", 2.2)
			return b.bytes()
		}(), ErrICCNotArithmetic},
	} {
		_, err := ReadICC(tc.profile)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
}

// TestTheSRGBCurveWrittenAsAFormulaIsTheSRGBCurve is the witness this file can
// give itself: shape 3 with sRGB's own coefficients is not an approximation of
// [SRGBToLinear], it is the same function, and a profile that writes it that
// way deserves the same answer to the last bit.
func TestTheSRGBCurveWrittenAsAFormulaIsTheSRGBCurve(t *testing.T) {
	srgb := ParametricCurve{Shape: 3,
		G: 2.4, A: 1 / 1.055, B: 0.055 / 1.055, C: 1 / 12.92, D: 0.04045}
	worst := 0.0
	for i := range 10001 {
		x := float64(i) / 10000
		if d := math.Abs(srgb.At(x) - SRGBToLinear(x)); d > worst {
			worst = d
		}
	}
	if worst > 1e-15 {
		t.Errorf("worst disagreement with SRGBToLinear over 10001 points: %g", worst)
	}
}

// TestEveryParametricShapeIsReadAsItsOwnFormula walks the five, each at a
// point either side of where it breaks, because the shapes differ in WHERE
// they break and not only in their coefficients.
func TestEveryParametricShapeIsReadAsItsOwnFormula(t *testing.T) {
	for _, c := range []struct {
		shape  int
		params []float64
		at     float64
		want   float64
	}{
		// Every coefficient here is a whole number of 1/65536, because that
		// is what s15Fixed16 can hold: 0.1 comes back as 0.100006 and the
		// disagreement would be the format's, not the reading's.
		{0, []float64{2}, 0.5, 0.25},
		{1, []float64{2, 2, -0.5}, 0.75, 1},
		{1, []float64{2, 2, -0.5}, 0.1, 0},
		{2, []float64{2, 2, -0.5, 0.125}, 0.75, 1},
		{2, []float64{2, 2, -0.5, 0.125}, 0.1, 0.125},
		{3, []float64{2, 1, 0, 0.5, 0.25}, 0.5, 0.25},
		{3, []float64{2, 1, 0, 0.5, 0.25}, 0.125, 0.0625},
		// A curve that leans DOWNWARDS: past its break the base of the power
		// turns negative, and a negative base under a fractional exponent is
		// not a number. It is nought, which is what the curve is describing.
		{1, []float64{2, -1, 0.5}, 0.75, 0},
		{4, []float64{2, 1, 0, 0.5, 0.25, 0.125, 0.0625}, 0.5, 0.375},
		{4, []float64{2, 1, 0, 0.5, 0.25, 0.125, 0.0625}, 0.125, 0.125},
	} {
		b := &iccBuilder{space: "RGB ", pcs: "XYZ "}
		b.xyz("rXYZ", 0.4, 0.2, 0)
		b.xyz("gXYZ", 0.3, 0.7, 0.1)
		b.xyz("bXYZ", 0.2, 0.1, 0.7)
		tag := paraTag(c.shape, c.params...)
		b.raw2("rTRC", tag)
		b.raw2("gTRC", tag)
		b.raw2("bTRC", tag)
		p, err := ReadICC(b.bytes())
		if err != nil {
			t.Fatalf("shape %d: %v", c.shape, err)
		}
		got := p.(*ICCMatrixTRC).Curves[0].At(c.at)
		if math.Abs(got-c.want) > 1e-12 {
			t.Errorf("shape %d at %.3f = %.6f, want %.6f", c.shape, c.at, got, c.want)
		}
	}
}

func TestAParametricCurveThatIsNotOneOfTheFive(t *testing.T) {
	for _, c := range []struct {
		name string
		tag  []byte
		want error
	}{
		{"a sixth shape", paraTag(5, 1), ErrICCNotArithmetic},
		{"shorter than its own header", paraTag(0)[:10], ErrICCMalformed},
		{"fewer coefficients than its shape needs", paraTag(4, 1, 1, 1), ErrICCMalformed},
	} {
		b := &iccBuilder{space: "RGB ", pcs: "XYZ "}
		b.xyz("rXYZ", 0.4, 0.2, 0)
		b.xyz("gXYZ", 0.3, 0.7, 0.1)
		b.xyz("bXYZ", 0.2, 0.1, 0.7)
		b.raw2("rTRC", c.tag)
		b.raw2("gTRC", c.tag)
		b.raw2("bTRC", c.tag)
		if _, err := ReadICC(b.bytes()); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

// paraTag writes a parametricCurveType of the shape and coefficients given.
// It writes exactly the coefficients it is handed, so a test can hand it too
// few on purpose.
func paraTag(shape int, params ...float64) []byte {
	d := make([]byte, 12+len(params)*4)
	copy(d, "para")
	binary.BigEndian.PutUint16(d[8:], uint16(shape))
	for i, v := range params {
		binary.BigEndian.PutUint32(d[12+i*4:], uint32(int32(math.Round(v*65536))))
	}
	return d
}
