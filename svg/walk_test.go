// Copyright (c) the go-gfx authors.
// SPDX-License-Identifier: BSD-3-Clause

package svg

import (
	"image/color"
	"strings"
	"testing"

	"github.com/go-gfx/gfx/raster"
	"github.com/go-gfx/gfx/vector"
)

// newPaper is a surface of the given size prefilled with paper, exactly as
// Rasterize prepares one.
func newPaper(w, h int, paper color.RGBA) *raster.Image {
	img := raster.New(w, h)
	if paper.A > 0 {
		for i := 0; i+3 < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = paper.R, paper.G, paper.B, paper.A
		}
	}
	return img
}

// drawShape paints one handed-over shape with the same rasteriser Rasterize
// uses, in the same order: fill first, then stroke.
//
// ⛔ It is written from the Shape alone — no access to the renderer's state —
// which is the point: if a consumer cannot reproduce the drawing from what the
// sink hands it, the sink is not handing over enough.
func drawShape(img *raster.Image, s Shape) {
	// ⛔ The colour goes down at FULL alpha and the opacity is applied to the
	// coverage, which is what paintFor does. Painting the colour's own alpha
	// as well would double it.
	opaque := func(c color.RGBA) vector.SolidPaint { c.A = 255; return vector.SolidPaint{Color: c} }

	var rz vector.Rasterizer
	if s.Fill != nil {
		if cov, ox, oy, w, h, ok := rz.Fill(s.Path, s.FillRule, img.W, img.H); ok {
			vector.Composite(img, scaleCov(cov, s.Fill.Opacity), ox, oy, w, h,
				opaque(s.Fill.Colour))
		}
	}
	if s.Stroke == nil {
		return
	}
	style := vector.StrokeStyle{
		Width:      s.StrokeWidth,
		Cap:        s.Cap,
		Join:       s.Join,
		MiterLimit: s.MiterLimit,
		Dash:       s.Dash,
		DashPhase:  s.DashPhase,
	}
	if cov, ox, oy, w, h, ok := rz.StrokeWith(s.Path, style, img.W, img.H); ok {
		vector.Composite(img, scaleCov(cov, s.Stroke.Opacity), ox, oy, w, h,
			opaque(s.Stroke.Colour))
	}
}

func collect(t *testing.T, doc string, opt Options) ([]Shape, *WalkResult) {
	t.Helper()
	var got []Shape
	res, err := Walk(doc, opt, SinkFunc(func(s Shape) { got = append(got, s) }))
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	return got, res
}

const twoShapes = `<svg viewBox="0 0 100 100">
  <rect x="10" y="10" width="30" height="20" fill="#ff0000"/>
  <path d="M60 20 L90 20 L75 50 Z" fill="none" stroke="#0000ff" stroke-width="4"
        stroke-linecap="round" stroke-linejoin="bevel"/>
</svg>`

func TestWalkHandsOverEveryShapeInPaintOrder(t *testing.T) {
	got, res := collect(t, twoShapes, Options{Scale: 1})
	if len(got) != 2 {
		t.Fatalf("%d shapes, want 2", len(got))
	}
	if res.Width != 100 || res.Height != 100 {
		t.Errorf("surface %dx%d, want 100x100", res.Width, res.Height)
	}
	if len(res.Unsupported) != 0 {
		t.Errorf("nothing here is unsupported, yet: %v", res.Unsupported)
	}

	// The rect: filled red, not stroked.
	if got[0].Fill == nil {
		t.Fatal("the rect came back unfilled")
	}
	if c := got[0].Fill.Colour; c.R != 0xff || c.G != 0 || c.B != 0 {
		t.Errorf("the rect is %v, want red", c)
	}
	if got[0].Stroke != nil {
		t.Error("the rect is not stroked")
	}

	// The path: stroked blue, not filled, with its own cap and join.
	if got[1].Fill != nil {
		t.Error(`fill="none" came back filled`)
	}
	if got[1].Stroke == nil {
		t.Fatal("the path came back unstroked")
	}
	if c := got[1].Stroke.Colour; c.B != 0xff || c.R != 0 {
		t.Errorf("the stroke is %v, want blue", c)
	}
	if got[1].StrokeWidth != 4 {
		t.Errorf("stroke width %v at scale 1, want 4", got[1].StrokeWidth)
	}
	if got[1].Cap != vector.RoundCap || got[1].Join != vector.BevelJoin {
		t.Errorf("cap %v join %v, want round and bevel", got[1].Cap, got[1].Join)
	}
}

// TestWalkAndRasterizeDrawTheSameShapes is the property that makes one walk
// with two endings worth having.
//
// ⛔ It re-rasterises the handed-over shapes and compares the PIXELS with what
// Rasterize produced for the same document. Comparing the shape structs with
// themselves would only prove Walk is consistent with Walk; the surface is the
// thing both are supposed to agree about, and one of them was written without
// any knowledge of the other.
func TestWalkAndRasterizeDrawTheSameShapes(t *testing.T) {
	const opaque = `<svg viewBox="0 0 60 40">
	  <rect x="4" y="4" width="20" height="14" fill="#2040c0"/>
	  <circle cx="40" cy="20" r="12" fill="#c04020"/>
	  <path d="M4 34 L56 34" stroke="#108020" stroke-width="3"/>
	</svg>`
	opt := Options{Scale: 2, Paper: color.RGBA{255, 255, 255, 255}}

	want, err := Rasterize(opaque, opt)
	if err != nil {
		t.Fatal(err)
	}

	shapes, res := collect(t, opaque, opt)
	if len(res.Unsupported) != 0 {
		t.Fatalf("the fixture uses something Walk cannot express: %v", res.Unsupported)
	}

	// Draw the handed-over shapes onto a fresh surface of the same size, with
	// the same paper, using the same rasteriser.
	got := newPaper(res.Width, res.Height, opt.Paper)
	for _, s := range shapes {
		drawShape(got, s)
	}

	if len(got.Pix) != len(want.Image.Pix) {
		t.Fatalf("surfaces differ in size: %d vs %d bytes", len(got.Pix), len(want.Image.Pix))
	}
	diff := 0
	for i := range got.Pix {
		if got.Pix[i] != want.Image.Pix[i] {
			diff++
		}
	}
	if diff != 0 {
		t.Errorf("%d of %d bytes differ between the walked shapes and the rasterised document",
			diff, len(got.Pix))
	}
}

func TestWalkReportsWhatItCannotExpress(t *testing.T) {
	// ⛔ Each of these draws something a vector consumer would silently LOSE,
	// and the whole point of the list is that it can refuse rather than ship a
	// document that opens and is wrong.
	for _, tc := range []struct {
		why  string
		doc  string
		want string
	}{
		{
			"a gradient fill",
			`<svg viewBox="0 0 10 10"><defs>
			   <linearGradient id="g"><stop offset="0" stop-color="#000"/>
			   <stop offset="1" stop-color="#fff"/></linearGradient></defs>
			 <rect width="10" height="10" fill="url(#g)"/></svg>`,
			"gradient",
		},
		{
			"a gradient stroke",
			`<svg viewBox="0 0 10 10"><defs>
			   <linearGradient id="g"><stop offset="0" stop-color="#000"/>
			   <stop offset="1" stop-color="#fff"/></linearGradient></defs>
			 <rect width="10" height="10" fill="none" stroke="url(#g)" stroke-width="2"/></svg>`,
			"gradient",
		},
		{
			"a clip path",
			`<svg viewBox="0 0 10 10"><defs>
			   <clipPath id="c"><rect width="5" height="10"/></clipPath></defs>
			 <rect width="10" height="10" fill="#000" clip-path="url(#c)"/></svg>`,
			"clip-path",
		},
		{
			"an embedded picture",
			`<svg viewBox="0 0 10 10"><image width="10" height="10"
			   href="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"/></svg>`,
			"image",
		},
	} {
		_, res := collect(t, tc.doc, Options{Scale: 1})
		found := false
		for _, u := range res.Unsupported {
			if u == tc.want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: Unsupported is %v, want it to name %q", tc.why, res.Unsupported, tc.want)
		}
	}

	// The control: a document using none of them reports nothing, so the list
	// is about the features and not about every document.
	_, res := collect(t, twoShapes, Options{Scale: 1})
	if len(res.Unsupported) != 0 {
		t.Errorf("a plain document reported %v", res.Unsupported)
	}
}

func TestWalkScalesStrokeAndDashWithTheTransform(t *testing.T) {
	// A stroke width is a length in user units, so it passes through the
	// transform — and so do the dash lengths. At Scale 3 everything triples.
	const doc = `<svg viewBox="0 0 20 20">
	  <path d="M2 10 L18 10" stroke="#000" stroke-width="2"
	        stroke-dasharray="4 1" stroke-dashoffset="3"/></svg>`
	got, _ := collect(t, doc, Options{Scale: 3})
	if len(got) != 1 {
		t.Fatalf("%d shapes", len(got))
	}
	s := got[0]
	if s.StrokeWidth != 6 {
		t.Errorf("stroke width %v, want 6", s.StrokeWidth)
	}
	if len(s.Dash) != 2 || s.Dash[0] != 12 || s.Dash[1] != 3 {
		t.Errorf("dash %v, want [12 3]", s.Dash)
	}
	if s.DashPhase != 9 {
		t.Errorf("dash phase %v, want 9", s.DashPhase)
	}
}

func TestWalkGivesDeviceSpacePaths(t *testing.T) {
	// The transform is applied before the sink sees anything: a rect at user
	// (10,10) under scale 2 starts at device (20,20), and a consumer places it
	// with no further arithmetic.
	const doc = `<svg viewBox="0 0 50 50"><rect x="10" y="10" width="5" height="5" fill="#000"/></svg>`
	got, _ := collect(t, doc, Options{Scale: 2})
	if len(got) != 1 {
		t.Fatalf("%d shapes", len(got))
	}
	var first [2]float64
	n := 0
	got[0].Path.Each(func(op vector.PathOp, p ...[2]float64) {
		if n == 0 && op == vector.OpMove {
			first = p[0]
		}
		n++
	})
	if first != [2]float64{20, 20} {
		t.Errorf("the rect starts at %v, want [20 20]", first)
	}
}

func TestWalkRefusesWhatIsNotAnSVG(t *testing.T) {
	for _, doc := range []string{"", "not xml at all", "<html><body/></html>"} {
		if _, err := Walk(doc, Options{}, SinkFunc(func(Shape) {})); err == nil {
			t.Errorf("%q was accepted", doc)
		}
	}
	// And the pixel ceiling still applies, because the surface size is still
	// computed from the document even when nothing is allocated.
	huge := `<svg width="40000" height="40000" viewBox="0 0 40000 40000"><rect width="1" height="1"/></svg>`
	if _, err := Walk(huge, Options{Scale: 1}, SinkFunc(func(Shape) {})); err == nil ||
		!strings.Contains(err.Error(), "past the") {
		t.Errorf("a 40000x40000 surface was accepted by Walk: %v", err)
	}
}

// TestWalkSkipsAShapeThatPaintsNothing covers two different ways a shape ends
// up invisible, and they take different paths through the code.
//
// ⛔ The first case does NOT exercise the check in emit, which is what I
// assumed when I wrote it: with fill="none" and no stroke the renderer never
// calls fillPath at all, so the test passed without the line it was aimed at
// ever running. The coverage gate is what said so. The second case is painted
// — the renderer does call through — and is invisible only once the opacity is
// read, which is the case emit is actually there for.
func TestWalkSkipsAShapeThatPaintsNothing(t *testing.T) {
	for _, tc := range []struct{ why, doc string }{
		{
			"fill=none, never reaches emit",
			`<svg viewBox="0 0 10 10"><rect width="5" height="5" fill="none"/></svg>`,
		},
		{
			"fill-opacity=0, reaches emit and is dropped there",
			`<svg viewBox="0 0 10 10"><rect width="5" height="5" fill="#000" fill-opacity="0"/></svg>`,
		},
		{
			"a stroke of zero width",
			`<svg viewBox="0 0 10 10"><rect width="5" height="5" fill="none" stroke="#000" stroke-width="0"/></svg>`,
		},
	} {
		got, _ := collect(t, tc.doc, Options{Scale: 1})
		if len(got) != 0 {
			t.Errorf("%s: %d shapes handed over for a document that paints nothing", tc.why, len(got))
		}
	}
}
