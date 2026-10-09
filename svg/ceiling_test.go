// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package svg

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"strings"
	"testing"
)

// claimingPNG is a real one-pixel PNG whose IHDR has been rewritten to claim w
// by h, with the chunk's CRC recomputed so every decoder accepts the header.
// The pixel data is still one pixel.
//
// ⛔ This is what the attack looks like: not large, not malformed, and nothing
// about it suspicious until something multiplies its two numbers together.
func claimingPNG(t *testing.T, w, h uint32) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	// 8 signature, then IHDR: 4 length, 4 type, 13 data (width, height first),
	// 4 CRC over type+data.
	put32(b[16:], w)
	put32(b[20:], h)
	put32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	return b
}

func put32(b []byte, v uint32) {
	b[0], b[1], b[2], b[3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
}

// svgWithImage wraps a picture in the smallest document that draws one.
func svgWithImage(raw []byte) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100">` +
		`<image x="0" y="0" width="100" height="100" href="data:image/png;base64,` +
		base64.StdEncoding.EncodeToString(raw) + `"/></svg>`
}

// heldDuring is how much the heap grew across f, with a collection either side.
// Coarse, which is all this needs: the difference is between megabytes and
// thousands of them.
func heldDuring(f func()) float64 {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	if after.HeapAlloc < before.HeapAlloc {
		return 0
	}
	return float64(after.HeapAlloc-before.HeapAlloc) / (1 << 20)
}

func TestASurfaceTheDocumentAsksForIsBounded(t *testing.T) {
	// ⛔ The worst of the two, and the one nothing about the input hints at. A
	// hundred-odd bytes of text; four bytes a pixel on the other side.
	// Measured before this check: 6103.5 MiB allocated and rasterised without
	// complaint.
	doc := `<svg xmlns="http://www.w3.org/2000/svg" width="40000" height="40000">` +
		`<rect width="10" height="10" fill="red"/></svg>`
	if len(doc) > 200 {
		t.Fatalf("the fixture is %d bytes, which is not the point being made", len(doc))
	}
	var err error
	held := heldDuring(func() { _, err = Rasterize(doc, Options{Scale: 1}) })
	if err == nil {
		t.Fatal("a 40000x40000 surface was allocated")
	}
	if !strings.Contains(err.Error(), "pixels") {
		t.Errorf("it was refused as %q, which is not a refusal about its size", err)
	}
	if held > 32 {
		t.Errorf("refusing it held %.1f MiB: the ceiling is being applied after the "+
			"allocation it is meant to bound", held)
	}
}

func TestAnOrdinarySurfaceIsStillRasterised(t *testing.T) {
	// ⛔ The side a ceiling gets wrong. Tested only from above, a limit is
	// satisfied by refusing everything. Five thousand square is twenty-five
	// megapixels — a large poster, and well inside the default.
	doc := `<svg xmlns="http://www.w3.org/2000/svg" width="5000" height="5000">` +
		`<rect width="10" height="10" fill="red"/></svg>`
	res, err := Rasterize(doc, Options{Scale: 1})
	if err != nil {
		t.Fatalf("an ordinary large document was refused: %v", err)
	}
	if res.Image.W != 5000 || res.Image.H != 5000 {
		t.Errorf("surface %dx%d", res.Image.W, res.Image.H)
	}
}

func TestScaleCountsTowardsTheCeiling(t *testing.T) {
	// Scale is OURS and the document's width is not, but the surface is their
	// product — so a modest document at a large scale is the same allocation
	// and must meet the same ceiling.
	doc := `<svg xmlns="http://www.w3.org/2000/svg" width="4000" height="4000">` +
		`<rect width="10" height="10"/></svg>`
	if _, err := Rasterize(doc, Options{Scale: 1}); err != nil {
		t.Fatalf("4000x4000 at scale 1 was refused: %v", err)
	}
	if _, err := Rasterize(doc, Options{Scale: 4}); err == nil {
		t.Error("the same document at scale 4 — 256 megapixels — was allocated")
	}
}

func TestTheCeilingCanBeRaisedAndLowered(t *testing.T) {
	doc := `<svg xmlns="http://www.w3.org/2000/svg" width="1000" height="1000">` +
		`<rect width="10" height="10"/></svg>`
	if _, err := Rasterize(doc, Options{Scale: 1, MaxPixels: 999_999}); err == nil {
		t.Error("a million pixels passed a ceiling of 999 999")
	}
	if _, err := Rasterize(doc, Options{Scale: 1, MaxPixels: 1_000_000}); err != nil {
		t.Errorf("exactly at the ceiling was refused: %v", err)
	}
	// ⛔ Zero means the default, not "no pixels allowed". Options{} is the
	// ordinary case: a caller who said nothing about size. Reading it as a
	// ceiling of zero would refuse every document, over a limit nobody set.
	if _, err := Rasterize(doc, Options{Scale: 1}); err != nil {
		t.Errorf("a document was refused when no ceiling was given: %v", err)
	}
}

func TestAnEmbeddedPictureIsRefusedBeforeItsPixelsAreAllocated(t *testing.T) {
	// ⛔ The picture is base64 in an ATTRIBUTE, so nothing about the document's
	// size says what it will cost. Measured before this check: a 246-byte SVG
	// whose <image> claimed 20000x20000 held 1526.1 MiB and was then drawn into
	// a 100x100 surface.
	doc := svgWithImage(claimingPNG(t, 20000, 20000))
	if len(doc) > 400 {
		t.Fatalf("the fixture is %d bytes, which is not a bomb", len(doc))
	}
	var res *Result
	var err error
	held := heldDuring(func() { res, err = Rasterize(doc, Options{Scale: 1}) })
	if err != nil {
		t.Fatalf("the document itself was refused: %v", err)
	}
	if res.Image.W != 100 || res.Image.H != 100 {
		t.Errorf("surface %dx%d", res.Image.W, res.Image.H)
	}
	if held > 32 {
		t.Errorf("an <image> claiming four hundred megapixels held %.1f MiB on the way "+
			"to being skipped", held)
	}
}

func TestAnEmbeddedPictureUnderTheCeilingIsStillDrawn(t *testing.T) {
	// ⛔ Again the side that matters: skipping every embedded picture would
	// satisfy the test above and lose the feature. This one checks the pixel
	// landed, not merely that nothing failed — drawImage skips silently, so a
	// picture that was dropped and a picture that was drawn look identical from
	// the outside.
	var buf bytes.Buffer
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src.Set(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	res, err := Rasterize(svgWithImage(buf.Bytes()), Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	c := res.Image.At(50, 50)
	if c.R < 200 || c.G > 60 || c.B > 60 {
		t.Errorf("the middle of the surface is %v, and the picture drawn there was red: "+
			"an embedded picture under the ceiling was skipped", c)
	}
}

func TestAPictureWhoseHeaderReadsAndWhoseBodyDoesNotIsSkipped(t *testing.T) {
	// ⛔ Reading the header first splits one failure into two, and this is the
	// second: a picture that passes the ceiling by declaration and then cannot
	// be decoded. It is the shape of every truncated upload, and before the
	// header was consulted there was only one path here.
	//
	// Forty by forty: comfortably under the ceiling, so the ceiling is not what
	// stops it, while the pixel data is still a single pixel.
	doc := svgWithImage(claimingPNG(t, 40, 40))
	res, err := Rasterize(doc, Options{Scale: 1})
	if err != nil {
		t.Fatalf("a document holding a truncated picture was refused: %v", err)
	}
	// Nothing was drawn, so the surface is still the paper it started as.
	if c := res.Image.At(50, 50); c.A != 0 {
		t.Errorf("the middle of the surface is %v: a picture that could not be decoded "+
			"was drawn anyway", c)
	}
}

func TestAPictureWithAnUnreadableHeaderIsSkipped(t *testing.T) {
	// The header is read before anything is decoded, so a header that cannot be
	// read is now the first thing to fail. It is still a skip, as every other
	// decode failure in this renderer is.
	doc := svgWithImage([]byte("this is not a picture"))
	res, err := Rasterize(doc, Options{Scale: 1})
	if err != nil {
		t.Fatalf("a document holding an unreadable picture was refused: %v", err)
	}
	if res.Image.W != 100 {
		t.Errorf("surface %dx%d", res.Image.W, res.Image.H)
	}
}

func TestALineOfZeroLengthDrawsNothing(t *testing.T) {
	// ⛔ Not about ceilings. This statement was the module's one uncovered
	// line, and the 100% gate passed anyway because the total rounds up across
	// seven packages. A gate that passes by ROUNDING will one day pass over a
	// real gap, so the gap is closed rather than left to dilute.
	//
	// A <line> whose two ends coincide has no direction, so there is nothing to
	// stroke. It must be dropped rather than handed to the stroker, which would
	// have to invent one.
	doc := `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20">` +
		`<line x1="10" y1="10" x2="10" y2="10" stroke="black" stroke-width="4"/></svg>`
	res, err := Rasterize(doc, Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			if res.Image.At(x, y).A != 0 {
				t.Fatalf("a zero-length line painted pixel %d,%d", x, y)
			}
		}
	}
}

func TestTheDefaultCeilingIsTheNumberTheDocumentationQuotes(t *testing.T) {
	// ⛔ A change detector, deliberately. The default is quoted in this
	// package's doc comment, in the README, and as a figure in megabytes — so
	// moving it silently makes three statements false at once, and the only
	// one anybody would notice is the one that stops a legitimate document.
	//
	// The number is written out here rather than derived from the constant:
	// deriving it would make the judge move with the subject, which is how a
	// ceiling test passes a mutation that raised the ceiling.
	if DefaultMaxPixels != 40_000_000 {
		t.Errorf("DefaultMaxPixels is %d. If that was meant, update the doc comment, the "+
			"README, and the \"a hundred and sixty megabytes\" below.", DefaultMaxPixels)
	}
	// And what it permits, said the way a person has to think about it: four
	// bytes a pixel.
	if got := fmt.Sprintf("%d MiB", 40_000_000*4/(1<<20)); got != "152 MiB" {
		t.Errorf("forty million pixels is %s", got)
	}
}
