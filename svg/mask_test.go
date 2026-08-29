// Copyright (c) 2026, the go-gfx authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file.

package svg

import (
	"image/color"
	"testing"
)

const strokedDoc = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path d="M4 12 L20 12" stroke="currentColor" stroke-width="2" fill="none"/></svg>`

// TestAMaskIsCoverageAndNothingElse covers the whole point of the mask: the
// shape is kept and the colour is left to whoever paints it. An icon is drawn
// in the ink of the moment — a button's foreground, a theme's accent, the
// inverse under a dark menu bar — so a mask that carried a colour would have
// decided something that is not its to decide.
func TestAMaskIsCoverageAndNothingElse(t *testing.T) {
	m, err := RasterizeMask(strokedDoc, Options{Scale: 4})
	if err != nil {
		t.Fatal(err)
	}
	b := m.Bounds()
	if b.Dx() != 96 || b.Dy() != 96 {
		t.Fatalf("mask is %dx%d, want the viewBox at the scale asked for", b.Dx(), b.Dy())
	}

	var ink, clear, partial int
	for _, v := range m.Pix {
		switch {
		case v == 0:
			clear++
		case v == 255:
			ink++
		default:
			partial++
		}
	}
	if ink == 0 {
		t.Error("nothing was drawn")
	}
	if clear == 0 {
		t.Error("everything was drawn, so the stroke has no edges")
	}
	// Coverage, not a stencil: a rasteriser without anti-aliasing gives only
	// 0 and 255, and an icon drawn from that has stepped edges.
	if partial == 0 {
		t.Error("no partial coverage anywhere, so nothing is anti-aliased")
	}

	// The ink and paper a caller passes are not theirs to choose here, and
	// asking for red must not produce a different mask.
	red, err := RasterizeMask(strokedDoc, Options{Scale: 4,
		Ink: color.RGBA{R: 255, A: 255}, Paper: color.RGBA{B: 255, A: 255}})
	if err != nil {
		t.Fatal(err)
	}
	for i := range m.Pix {
		if m.Pix[i] != red.Pix[i] {
			t.Fatalf("a colour the caller asked for changed the mask at pixel %d", i)
		}
	}
}

// TestAMaskOfSomethingThatIsNotADocument covers the refusal being the same as
// the picture path's, since it is the same parser.
func TestAMaskOfSomethingThatIsNotADocument(t *testing.T) {
	for _, doc := range []string{"", "not xml at all", "<html></html>"} {
		if _, err := RasterizeMask(doc, Options{}); err == nil {
			t.Errorf("a mask was made from %q", doc)
		}
	}
}
