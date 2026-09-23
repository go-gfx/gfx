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
	"os"
	"strings"
	"testing"
)

// lutProfileBytes builds the smallest ICC profile that carries a lookup table:
// a header saying what it is, and one tag per intent given.
func lutProfileBytes(version byte, class, space, pcs string, tags map[string][]byte) []byte {
	names := make([]string, 0, len(tags))
	for n := range tags {
		names = append(names, n)
	}
	// The tag table must be in a settled order for the bytes to be stable.
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	head := make([]byte, 132)
	head[8] = version
	copy(head[12:16], class)
	copy(head[16:20], space)
	copy(head[20:24], pcs)
	copy(head[36:40], "acsp")
	binary.BigEndian.PutUint32(head[128:132], uint32(len(names)))

	body := []byte{}
	table := make([]byte, 0, len(names)*12)
	off := uint32(132 + len(names)*12)
	for _, n := range names {
		e := make([]byte, 12)
		copy(e[0:4], n)
		binary.BigEndian.PutUint32(e[4:8], off+uint32(len(body)))
		binary.BigEndian.PutUint32(e[8:12], uint32(len(tags[n])))
		table = append(table, e...)
		body = append(body, tags[n]...)
	}
	out := append(append(head, table...), body...)
	binary.BigEndian.PutUint32(out[0:4], uint32(len(out)))
	return out
}

// mftTag builds an mft1 or mft2 tag whose grid is filled by at.
func mftTag(bits, in, out, grid int, at func(p []int) []float64) []byte {
	sig := "mft2"
	if bits == 8 {
		sig = "mft1"
	}
	b := make([]byte, 48)
	copy(b[0:4], sig)
	b[8], b[9], b[10] = byte(in), byte(out), byte(grid)
	// The identity matrix, as s15Fixed16 down the diagonal.
	for i := range 3 {
		binary.BigEndian.PutUint32(b[12+i*16:16+i*16], 0x00010000)
	}
	entries := 256
	if bits == 16 {
		entries = 2
		b = append(b, 0, 0, 0, 0)
		binary.BigEndian.PutUint16(b[48:50], uint16(entries))
		binary.BigEndian.PutUint16(b[50:52], uint16(entries))
	}
	put := func(v float64) {
		if bits == 8 {
			b = append(b, byte(math.Round(clamp01(v)*255)))
			return
		}
		var u [2]byte
		binary.BigEndian.PutUint16(u[:], uint16(math.Round(clamp01(v)*65535)))
		b = append(b, u[:]...)
	}
	ramp := func() {
		for i := range entries {
			put(float64(i) / float64(entries-1))
		}
	}
	for range in {
		ramp()
	}
	p := make([]int, in)
	var walk func(d int)
	walk = func(d int) {
		if d == in {
			for _, v := range at(p) {
				put(v)
			}
			return
		}
		for p[d] = range grid {
			walk(d + 1)
		}
	}
	walk(0)
	for range out {
		ramp()
	}
	return b
}

// inkTable is the transform the tests measure: a made-up press whose colour is
// a smooth function of its four inks, so that every answer can be checked
// against an engine that is not this one.
func inkTable(bits, grid int) []byte {
	return mftTag(bits, 4, 3, grid, func(p []int) []float64 {
		f := func(i int) float64 { return float64(p[i]) / float64(grid-1) }
		c, m, y, k := f(0), f(1), f(2), f(3)
		return []float64{
			clamp01(1 - 0.6*c - 0.2*m - 0.1*y - 0.9*k),
			clamp01(0.5 + 0.3*(m-c)),
			clamp01(0.5 + 0.3*(y-c)),
		}
	})
}

func inkProfile() []byte {
	return lutProfileBytes(2, "prtr", "CMYK", "Lab ", map[string][]byte{
		"A2B0": inkTable(16, 3),
		"A2B1": inkTable(16, 3),
	})
}

// readLut is the table a test wants out of some profile bytes, with the
// reading itself checked on the way past.
func readLut(t *testing.T, profile []byte, intent ICCIntent) *ICCLut {
	t.Helper()
	p, err := ReadICC(profile)
	if err != nil {
		t.Fatalf("ReadICC: %v", err)
	}
	lut, ok := p.(*ICCLutProfile)
	if !ok {
		t.Fatalf("read a %T, want a lookup-table profile", p)
	}
	return lut.Table(intent)
}

// readProfile is the whole profile, for the tests that are about the profile
// rather than one of its tables.
func readProfile(t *testing.T, profile []byte) *ICCLutProfile {
	t.Helper()
	p, err := ReadICC(profile)
	if err != nil {
		t.Fatalf("ReadICC: %v", err)
	}
	lut, ok := p.(*ICCLutProfile)
	if !ok {
		t.Fatalf("read a %T, want a lookup-table profile", p)
	}
	return lut
}

func TestALookupTableProfileIsReadAsOne(t *testing.T) {
	if p := os.Getenv("GFX_ICC_DUMP"); p != "" {
		// So that an outside engine can be pointed at the same bytes.
		if err := os.WriteFile(p, inkProfile(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	lut := readProfile(t, inkProfile())
	if lut.Inputs != 4 {
		t.Fatalf("inputs = %d, want 4", lut.Inputs)
	}
	tb := lut.Table(ICCRelativeColorimetric)
	if tb.Outputs != 3 || tb.Grid != 3 || tb.PCS != ICCPCSLabV2 {
		t.Fatalf("outputs %d grid %d pcs %d, want 3, 3, LabV2", tb.Outputs, tb.Grid, tb.PCS)
	}
}

// TestTheTableAgreesWithLittleCMS is the measurement this file exists for.
// The values on the right are what little-cms 2.19 -- the engine poppler calls
// -- makes of the same profile, asked for media-relative colorimetric with its
// optimiser off, which is how poppler asks.
//
// They are not expected to agree to the last bit: little-cms carries its
// pipeline in 16-bit fixed point, so a hundredth of a CIELAB unit is its
// resolution rather than a disagreement.
func TestTheTableAgreesWithLittleCMS(t *testing.T) {
	lut := readLut(t, inkProfile(), ICCRelativeColorimetric)
	for _, c := range []struct {
		cmyk [4]float64
		want Lab
	}{
		{[4]float64{0, 0, 0, 0}, Lab{L: 100.3906}},
		{[4]float64{1, 0, 0, 0}, Lab{L: 40.1563, A: -76.8008, B: -76.8008}},
		{[4]float64{0, 1, 0, 0}, Lab{L: 80.3125, A: 76.7969}},
		{[4]float64{0, 0, 1, 0}, Lab{L: 90.3523, B: 76.7969}},
		{[4]float64{0, 0, 0, 1}, Lab{L: 10.0383}},
		{[4]float64{0.20, 0.40, 0.60, 0.10}, Lab{L: 65.2528, A: 15.3594, B: 30.7188}},
		{[4]float64{0.735, 0.1225, 0.88, 0.03}, Lab{L: 42.8232, A: -47.0391, B: 11.1367}},
		{[4]float64{0.5, 0.5, 0.5, 0.5}, Lab{L: 10.0383}},
	} {
		got := XYZToLabWP(lut.XYZ(c.cmyk[:]), D50)
		for i, pair := range [][2]float64{{got.L, c.want.L}, {got.A, c.want.A}, {got.B, c.want.B}} {
			if math.Abs(pair[0]-pair[1]) > 0.01 {
				t.Errorf("CMYK %v: component %d = %.4f, little-cms says %.4f", c.cmyk, i, pair[0], pair[1])
			}
		}
	}
}

// TestEveryTetrahedronReadsAPlaneExactly walks a point through all six
// orderings of the three fractions -- which is what picks the tetrahedron --
// over a table that is a plane. An affine table has one right answer whichever
// tetrahedron the point falls in, so a branch reading the wrong corners cannot
// hide here.
func TestEveryTetrahedronReadsAPlaneExactly(t *testing.T) {
	plane := func(p []int) []float64 {
		x, y, z := float64(p[0]), float64(p[1]), float64(p[2])
		return []float64{0.1 + 0.5*x, 0.2 + 0.3*y, 0.3 + 0.4*z}
	}
	profile := lutProfileBytes(4, "scnr", "RGB ", "Lab ", map[string][]byte{
		"A2B0": mftTag(16, 3, 3, 2, plane),
	})
	lut := readLut(t, profile, ICCPerceptual)
	for _, in := range [][]float64{
		{0.7, 0.5, 0.2}, {0.7, 0.2, 0.5}, {0.5, 0.2, 0.7},
		{0.5, 0.7, 0.2}, {0.2, 0.7, 0.5}, {0.2, 0.5, 0.7},
	} {
		got := lut.Eval(in)
		want := []float64{0.1 + 0.5*in[0], 0.2 + 0.3*in[1], 0.3 + 0.4*in[2]}
		for i := range want {
			if math.Abs(got[i]-want[i]) > 1e-4 {
				t.Errorf("at %v: output %d = %.6f, want %.6f", in, i, got[i], want[i])
			}
		}
	}
}

// TestATableOfOneOrTwoInputsIsReadToo covers the shapes little-cms reads
// linearly and bilinearly rather than by cutting a cell into tetrahedra.
func TestATableOfOneOrTwoInputsIsReadToo(t *testing.T) {
	for _, n := range []int{1, 2} {
		fill := func(p []int) []float64 {
			out := []float64{0.1, 0.2, 0.3}
			for i, v := range p {
				out[i] += 0.5 * float64(v)
			}
			return out
		}
		profile := lutProfileBytes(4, "scnr", "GRAY", "Lab ", map[string][]byte{
			"A2B0": mftTag(16, n, 3, 2, fill),
		})
		lut := readLut(t, profile, ICCPerceptual)
		in := []float64{0.25, 0.75}[:n]
		got := lut.Eval(in)
		want := []float64{0.1, 0.2, 0.3}
		for i := range in {
			want[i] += 0.5 * in[i]
		}
		for i := range want {
			if math.Abs(got[i]-want[i]) > 1e-4 {
				t.Errorf("%d inputs, output %d = %.6f, want %.6f", n, i, got[i], want[i])
			}
		}
	}
}

// TestAnEightBitTableIsReadAsSuch checks the other width, whose curves are
// fixed at 256 entries and whose CIELAB is the straightforward encoding at
// every version of the format.
func TestAnEightBitTableIsReadAsSuch(t *testing.T) {
	profile := lutProfileBytes(2, "prtr", "CMYK", "Lab ", map[string][]byte{
		"A2B0": inkTable(8, 3),
	})
	lut := readLut(t, profile, ICCPerceptual)
	if lut.PCS != ICCPCSLabV4 {
		t.Fatalf("pcs = %d, want the straightforward CIELAB encoding", lut.PCS)
	}
	// White paper: no ink at all is L* = 100.
	got := XYZToLabWP(lut.XYZ([]float64{0, 0, 0, 0}), D50)
	if math.Abs(got.L-100) > 0.5 {
		t.Errorf("no ink reads L* = %.3f, want 100", got.L)
	}
}

// TestFewerValuesThanChannelsReadAsNought guards the caller that hands over a
// short slice: the missing channels are nought rather than a panic.
func TestFewerValuesThanChannelsReadAsNought(t *testing.T) {
	lut := readLut(t, inkProfile(), ICCPerceptual)
	if got, want := lut.Eval(nil), lut.Eval([]float64{0, 0, 0, 0}); got[0] != want[0] {
		t.Errorf("no values read %v, four noughts read %v", got, want)
	}
}

func TestAProfileInXYZIsReadInXYZ(t *testing.T) {
	profile := lutProfileBytes(4, "scnr", "RGB ", "XYZ ", map[string][]byte{
		"A2B0": mftTag(16, 3, 3, 2, func(p []int) []float64 {
			return []float64{0.5, 0.5, 0.5}
		}),
	})
	lut := readLut(t, profile, ICCPerceptual)
	if lut.PCS != ICCPCSXYZ {
		t.Fatalf("pcs = %d, want XYZ", lut.PCS)
	}
	c := lut.XYZ([]float64{0.5, 0.5, 0.5})
	// A half written into sixteen bits is 0x8000, and 0x8000 is exactly
	// what this encoding means by 1.0. The round number on the way in is
	// the round number on the way out, which is the encoding's whole point.
	if math.Abs(c.X-1) > 1e-9 {
		t.Errorf("X = %.9f, want 1", c.X)
	}
	// And the way in, which encodes rather than decodes.
	if got := lut.Device(Lab{L: 100}); len(got) != 3 {
		t.Errorf("Device gave %d values, want 3", len(got))
	}
}

func TestAVersionFourTableUsesTheOtherLabEncoding(t *testing.T) {
	for _, c := range []struct {
		version byte
		want    ICCPCS
	}{{2, ICCPCSLabV2}, {4, ICCPCSLabV4}} {
		profile := lutProfileBytes(c.version, "prtr", "CMYK", "Lab ", map[string][]byte{
			"A2B0": inkTable(16, 3),
		})
		if got := readLut(t, profile, ICCPerceptual).PCS; got != c.want {
			t.Errorf("version %d reads as pcs %d, want %d", c.version, got, c.want)
		}
	}
}

// TestTheWayBackEncodesTheColourItIsGiven checks Device against the decoding
// it must invert: what the table gives back for a colour it was asked to
// encode should be that colour again, the grid being a plain ramp here.
func TestTheWayBackEncodesTheColourItIsGiven(t *testing.T) {
	for _, c := range []struct {
		version byte
		want    ICCPCS
	}{{2, ICCPCSLabV2}, {4, ICCPCSLabV4}} {
		profile := lutProfileBytes(c.version, "prtr", "CMYK", "Lab ", map[string][]byte{
			"A2B0": mftTag(16, 3, 3, 2, func(p []int) []float64 {
				return []float64{float64(p[0]), float64(p[1]), float64(p[2])}
			}),
		})
		lut := readLut(t, profile, ICCPerceptual)
		if lut.PCS != c.want {
			t.Fatalf("version %d reads as pcs %d", c.version, lut.PCS)
		}
		got := lut.Device(Lab{L: 50, A: 20, B: -30})
		back := XYZToLabWP(lut.XYZ(got), D50)
		for i, pair := range [][2]float64{{back.L, 50}, {back.A, 20}, {back.B, -30}} {
			if math.Abs(pair[0]-pair[1]) > 0.02 {
				t.Errorf("version %d, component %d came back as %.4f, want %.4f",
					c.version, i, pair[0], pair[1])
			}
		}
	}
}

// pressProfile is a press whose darkest ink is a long way from black, which is
// what makes black point compensation do anything at all.
func pressProfile() []byte {
	forward := mftTag(16, 4, 3, 2, func(p []int) []float64 {
		ink := 0.0
		for _, v := range p {
			ink += float64(v) / 4
		}
		// Full ink measures L* = 20, not 0: a press cannot reach black.
		return []float64{1 - 0.8*ink, 0.5, 0.5}
	})
	// The way back asks for every ink there is when it is asked for black.
	back := mftTag(16, 3, 4, 2, func(p []int) []float64 {
		v := 1 - float64(p[0])
		return []float64{v, v, v, v}
	})
	return lutProfileBytes(4, "prtr", "CMYK", "Lab ", map[string][]byte{
		"A2B0": forward, "A2B1": forward, "B2A0": back,
	})
}

func TestBlackPointCompensationCarriesTheDarkestInkToBlack(t *testing.T) {
	lut := readProfile(t, pressProfile())
	if lut.Black.Y <= 0 {
		t.Fatalf("black point = %+v, want a colour above nought", lut.Black)
	}
	full := []float64{1, 1, 1, 1}
	r, g, b := lut.ToSRGB(ICCRelativeColorimetric, full)
	rc, gc, bc := lut.ToSRGBCompensated(ICCRelativeColorimetric, full)
	if rc >= r || gc >= g || bc >= b {
		t.Errorf("full ink reads %.4f %.4f %.4f plain and %.4f %.4f %.4f compensated, want darker",
			r, g, b, rc, gc, bc)
	}
	if rc > 0.05 {
		t.Errorf("full ink compensated to %.4f, want it carried to black", rc)
	}
	// White must stay where it is: compensation moves the black end only.
	none := []float64{0, 0, 0, 0}
	rw, _, _ := lut.ToSRGB(ICCRelativeColorimetric, none)
	rwc, _, _ := lut.ToSRGBCompensated(ICCRelativeColorimetric, none)
	// Not to the last bit: compensation anchors D50 itself, and this
	// profile's paper is a fraction off D50, so it slides by a fraction too.
	if math.Abs(rw-rwc) > 1e-3 {
		t.Errorf("no ink reads %.6f plain and %.6f compensated, want them within a thousandth", rw, rwc)
	}
}

func TestAProfileWithNoWayBackCompensatesNothing(t *testing.T) {
	lut := readProfile(t, inkProfile())
	if lut.Black != (XYZ{}) {
		t.Fatalf("black point = %+v, want the zero value with no B2A0 to read", lut.Black)
	}
	in := []float64{0.3, 0.4, 0.5, 0.6}
	r, g, b := lut.ToSRGB(ICCRelativeColorimetric, in)
	rc, gc, bc := lut.ToSRGBCompensated(ICCRelativeColorimetric, in)
	if r != rc || g != gc || b != bc {
		t.Errorf("compensated %.6f %.6f %.6f against plain %.6f %.6f %.6f", rc, gc, bc, r, g, b)
	}
}

// TestABlackPointAtTheWhitePointChangesNothing is the guard against dividing
// by nothing, reached by a profile that says its black IS its white.
func TestABlackPointAtTheWhitePointChangesNothing(t *testing.T) {
	lut := readProfile(t, inkProfile())
	lut.Black = XYZ(D50)
	in := []float64{0.2, 0.2, 0.2, 0.2}
	r, g, b := lut.ToSRGB(ICCRelativeColorimetric, in)
	rc, gc, bc := lut.ToSRGBCompensated(ICCRelativeColorimetric, in)
	if r != rc || g != gc || b != bc {
		t.Errorf("compensated %.6f %.6f %.6f against plain %.6f %.6f %.6f", rc, gc, bc, r, g, b)
	}
}

func TestABlackPointNeedsBothDirections(t *testing.T) {
	good := readLut(t, pressProfile(), ICCRelativeColorimetric)
	other := readLut(t, inkProfile(), ICCRelativeColorimetric)
	for _, c := range []struct {
		name            string
		toDevice, toPCS *ICCLut
	}{
		{"no way back", nil, good},
		{"no way in", good, nil},
		{"two tables that do not meet", other, good},
	} {
		if got := iccBlackPoint(c.toDevice, c.toPCS); got != (XYZ{}) {
			t.Errorf("%s: black point = %+v, want the zero value", c.name, got)
		}
	}
}

// TestAnAbsurdBlackPointIsCapped covers the ceiling on lightness: a profile
// whose way back answers white must not be taken at its word, or the picture
// comes out inverted.
func TestAnAbsurdBlackPointIsCapped(t *testing.T) {
	forward := mftTag(16, 4, 3, 2, func(p []int) []float64 { return []float64{1, 0.5, 0.5} })
	back := mftTag(16, 3, 4, 2, func(p []int) []float64 { return []float64{0, 0, 0, 0} })
	profile := lutProfileBytes(4, "prtr", "CMYK", "Lab ", map[string][]byte{
		"A2B0": forward, "A2B1": forward, "B2A0": back,
	})
	lut := readProfile(t, profile)
	want := LabToXYZWP(Lab{L: 50}, D50)
	if math.Abs(lut.Black.Y-want.Y) > 1e-9 {
		t.Errorf("black point Y = %.9f, want it capped at L* = 50, which is %.9f", lut.Black.Y, want.Y)
	}
}

func TestAnIntentTheProfileDoesNotCarryFallsBack(t *testing.T) {
	lut := readProfile(t, inkProfile())
	if lut.Tables[ICCSaturation] != nil {
		t.Fatal("this profile was built without a saturation table")
	}
	if lut.Table(ICCSaturation) != lut.Table(ICCPerceptual) {
		t.Error("an absent intent did not fall back on one the profile carries")
	}
	if lut.Table(-1) != lut.Table(ICCPerceptual) {
		t.Error("an intent outside the three did not fall back either")
	}
	if got := (&ICCLutProfile{}).Table(ICCPerceptual); got != nil {
		t.Errorf("a profile with no table at all gave %v, want nil", got)
	}
}

func TestALookupTableProfileConvertsToSRGB(t *testing.T) {
	lut := readProfile(t, inkProfile())
	// No ink is paper, and paper is white.
	r, g, b := lut.ToSRGB(ICCRelativeColorimetric, []float64{0, 0, 0, 0})
	for i, v := range []float64{r, g, b} {
		if v < 0.99 {
			t.Errorf("no ink: component %d = %.4f, want white", i, v)
		}
	}
	// Full ink is dark.
	r, _, _ = lut.ToSRGB(ICCRelativeColorimetric, []float64{1, 1, 1, 1})
	if r > 0.2 {
		t.Errorf("full ink read %.4f, want it dark", r)
	}
	// And the same through the table rather than through the profile.
	r2, _, _ := lut.Table(ICCRelativeColorimetric).ToSRGB([]float64{1, 1, 1, 1})
	if r != r2 {
		t.Errorf("profile gave %.6f and its table gave %.6f", r, r2)
	}
}

func TestAProfileWhoseTablesCannotBeReadIsDeclined(t *testing.T) {
	// A version 4 lutAtoBType, which this package does not read.
	mAB := make([]byte, 32)
	copy(mAB[0:4], "mAB ")
	profile := lutProfileBytes(4, "prtr", "CMYK", "Lab ", map[string][]byte{"A2B0": mAB})
	if _, err := ReadICC(profile); !errors.Is(err, ErrICCNotArithmetic) {
		t.Errorf("err = %v, want ErrICCNotArithmetic", err)
	}
}

// TestOneUnreadableIntentDoesNotLoseTheOthers is why the tags are tried one at
// a time rather than all or nothing.
func TestOneUnreadableIntentDoesNotLoseTheOthers(t *testing.T) {
	mAB := make([]byte, 32)
	copy(mAB[0:4], "mAB ")
	profile := lutProfileBytes(2, "prtr", "CMYK", "Lab ", map[string][]byte{
		"A2B0": mAB, "A2B1": inkTable(16, 3),
	})
	lut := readProfile(t, profile)
	if lut.Tables[ICCPerceptual] != nil {
		t.Error("the unreadable intent was kept")
	}
	if lut.Tables[ICCRelativeColorimetric] == nil {
		t.Error("the readable intent was lost with it")
	}
}

// TestAWayBackThatCannotBeReadIsNotAnError covers the other half: B2A0 is read
// for the black point alone, so a profile whose way back is a shape this
// package declines is still a profile it can use.
func TestAWayBackThatCannotBeReadIsNotAnError(t *testing.T) {
	mBA := make([]byte, 32)
	copy(mBA[0:4], "mBA ")
	profile := lutProfileBytes(2, "prtr", "CMYK", "Lab ", map[string][]byte{
		"A2B0": inkTable(16, 3), "B2A0": mBA,
	})
	if lut := readProfile(t, profile); lut.Black != (XYZ{}) {
		t.Errorf("black point = %+v, want the zero value", lut.Black)
	}
}

func TestAMalformedTableIsAMalformedProfile(t *testing.T) {
	good := inkTable(16, 3)
	for _, c := range []struct {
		name string
		tag  []byte
	}{
		{"shorter than its own header", good[:40]},
		{"no input channels", withByte(good, 8, 0)},
		{"more input channels than anything has", withByte(good, 8, 9)},
		{"fewer output channels than a colour", withByte(good, 9, 2)},
		{"more output channels than anything has", withByte(good, 9, 9)},
		{"a grid of one point", withByte(good, 10, 1)},
		{"a grid too big for the bytes that follow", withByte(good, 10, 200)},
		{"one entry in its input curve", withUint16(good, 48, 1)},
		{"one entry in its output curve", withUint16(good, 50, 1)},
		{"more entries than bytes", withUint16(good, 48, 4096)},
	} {
		profile := lutProfileBytes(2, "prtr", "CMYK", "Lab ", map[string][]byte{"A2B0": c.tag})
		if _, err := ReadICC(profile); !errors.Is(err, ErrICCMalformed) {
			t.Errorf("%s: err = %v, want ErrICCMalformed", c.name, err)
		}
	}
}

func TestAConnectionSpaceThatIsNeitherIsMalformed(t *testing.T) {
	profile := lutProfileBytes(2, "prtr", "CMYK", "RGB ", map[string][]byte{"A2B0": inkTable(16, 3)})
	if _, err := ReadICC(profile); !errors.Is(err, ErrICCMalformed) {
		t.Errorf("err = %v, want ErrICCMalformed", err)
	}
}

func withByte(b []byte, at int, v byte) []byte {
	out := append([]byte(nil), b...)
	out[at] = v
	return out
}

func withUint16(b []byte, at int, v uint16) []byte {
	out := append([]byte(nil), b...)
	binary.BigEndian.PutUint16(out[at:at+2], v)
	return out
}

// TestARefusalNamesWhatItRefused. A caller that meets one of these has to fall
// back, and what it can SAY about the fallback is the difference between a
// known limit and a shrug. Each message is checked for the artefact it names,
// because a refusal that cannot name what it refused is indistinguishable from
// a refusal to look.
func TestARefusalNamesWhatItRefused(t *testing.T) {
	opaque := func(sig string) []byte {
		d := make([]byte, 40)
		copy(d, sig)
		return d
	}
	for _, c := range []struct {
		name    string
		profile []byte
		names   string
	}{
		{"a version 4 lookup table", lutProfileBytes(4, "prtr", "CMYK", "Lab ",
			map[string][]byte{"A2B0": opaque("mAB ")}), `"mAB " lookup table`},
		{"a parametric curve of a shape ICC does not define", lutProfileBytes(2, "mntr", "RGB ", "XYZ ", map[string][]byte{
			"rXYZ": xyzTag(0.4, 0.2, 0), "gXYZ": xyzTag(0.3, 0.7, 0.1), "bXYZ": xyzTag(0.2, 0.1, 0.7),
			"rTRC": paraTag(9, 1), "gTRC": paraTag(9, 1), "bTRC": paraTag(9, 1),
		}), "parametric curve of shape 9"},
		{"a curve type nothing knows", lutProfileBytes(2, "mntr", "RGB ", "XYZ ", map[string][]byte{
			"rXYZ": xyzTag(0.4, 0.2, 0), "gXYZ": xyzTag(0.3, 0.7, 0.1), "bXYZ": xyzTag(0.2, 0.1, 0.7),
			"rTRC": opaque("zzzz"), "gTRC": opaque("zzzz"), "bTRC": opaque("zzzz"),
		}), `"zzzz" curve`},
		{"no colorants at all", lutProfileBytes(2, "mntr", "RGB ", "XYZ ",
			map[string][]byte{"desc": opaque("desc")}), "no colorants and no lookup table"},
		{"colorants with a curve missing", lutProfileBytes(2, "mntr", "RGB ", "XYZ ", map[string][]byte{
			"rXYZ": xyzTag(0.4, 0.2, 0), "gXYZ": xyzTag(0.3, 0.7, 0.1), "bXYZ": xyzTag(0.2, 0.1, 0.7),
			"rTRC": opaque("curv"),
		}), "no gTRC curve"},
	} {
		_, err := ReadICC(c.profile)
		if !errors.Is(err, ErrICCNotArithmetic) {
			t.Errorf("%s: err = %v, want ErrICCNotArithmetic", c.name, err)
			continue
		}
		if !strings.Contains(err.Error(), c.names) {
			t.Errorf("%s: err = %q, want it to name %q", c.name, err, c.names)
		}
	}
}

// xyzTag is an XYZType tag, whose numbers are s15Fixed16.
func xyzTag(x, y, z float64) []byte {
	d := make([]byte, 20)
	copy(d, "XYZ ")
	for i, v := range []float64{x, y, z} {
		binary.BigEndian.PutUint32(d[8+i*4:], uint32(int32(math.Round(v*65536))))
	}
	return d
}
