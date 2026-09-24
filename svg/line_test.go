// SPDX-License-Identifier: BSD-3-Clause

package svg

import (
	"image/color"
	"testing"
)

// ⛔ A shape absent from the element switch is not "unsupported": it is
// SILENTLY NOT PAINTED. The document renders, the file is written, nothing
// reports anything, and the drawing is simply missing a part of itself.
//
// <line> was in that position, and it is the second time this has happened --
// <ellipse> was absent until v0.24.0 and a logo's disc vanished. This one cost
// a brand banner: openweft's mark draws its glyph with four <line> elements,
// so the generated 1280x640 social banner came out with the wordmark and an
// empty space where the glyph belongs, and nothing in the pipeline said so.
//
// The assertion is about PIXELS, not about parsing: an element that is
// accepted and then paints nothing would pass a structural test.
func TestLineIsPainted(t *testing.T) {
	const doc = `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40" viewBox="0 0 40 40">
  <line x1="5" y1="20" x2="35" y2="20" stroke="#000000" stroke-width="6"/>
</svg>`
	px := at(t, doc, 20, 20)
	if px[0] > 128 {
		t.Fatalf("the middle of a stroked <line> is not painted (got %v): the element "+
			"is missing from the shape switch, so it is dropped without a word", px)
	}
	if above := at(t, doc, 20, 4); above[0] < 128 {
		t.Errorf("something is painted well above the line (got %v)", above)
	}
}

// TestLineWithoutStrokeIsNotPainted keeps the fix honest in the other
// direction: <line> has no interior, so a fill must not invent one.
func TestLineWithoutStrokeIsNotPainted(t *testing.T) {
	const doc = `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40" viewBox="0 0 40 40">
  <line x1="5" y1="20" x2="35" y2="20" fill="#000000"/>
</svg>`
	if px := at(t, doc, 20, 20); px[0] < 128 {
		t.Errorf("a <line> with no stroke painted something (got %v)", px)
	}
}

func at(t *testing.T, doc string, x, y int) []uint8 {
	t.Helper()
	r, err := Rasterize(doc, Options{Scale: 1, Ink: color.RGBA{0, 0, 0, 255}, Paper: color.RGBA{255, 255, 255, 255}})
	if err != nil {
		t.Fatal(err)
	}
	i := (y*r.Image.W + x) * 4
	return r.Image.Pix[i : i+4]
}
