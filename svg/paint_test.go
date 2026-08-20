// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package svg

import (
	"image/color"
	"testing"
)

// rasterise renders doc at 1 device pixel per user unit and returns the surface.
func rasterise(t *testing.T, doc string) *result64 {
	t.Helper()
	res, err := Rasterize(doc, Options{Scale: 1})
	if err != nil {
		t.Fatalf("Rasterize: %v", err)
	}
	return &result64{res.Image.Pix, res.Image.W, res.Image.H}
}

type result64 struct {
	pix  []uint8
	w, h int
}

func (r *result64) at(x, y int) color.RGBA {
	i := 4 * (y*r.w + x)
	return color.RGBA{r.pix[i], r.pix[i+1], r.pix[i+2], r.pix[i+3]}
}

func (r *result64) colours() int {
	seen := map[color.RGBA]struct{}{}
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			seen[r.at(x, y)] = struct{}{}
		}
	}
	return len(seen)
}

// A linearGradient in the default objectBoundingBox units is placed across the
// filled shape's own box, so the two ends of the box carry the two stop colours.
func TestLinearGradientObjectBoundingBox(t *testing.T) {
	res := rasterise(t, `<svg viewBox="0 0 40 40">
	  <defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0">
	    <stop offset="0" stop-color="#ff0000"/><stop offset="1" stop-color="#0000ff"/>
	  </linearGradient></defs>
	  <rect width="40" height="40" fill="url(#g)"/>
	</svg>`)
	left, right := res.at(1, 20), res.at(38, 20)
	if left.R < 200 || left.B > 60 {
		t.Errorf("bord gauche = %v, attendu proche du rouge", left)
	}
	if right.B < 200 || right.R > 60 {
		t.Errorf("bord droit = %v, attendu proche du bleu", right)
	}
	if n := res.colours(); n < 10 {
		t.Errorf("%d couleurs — un dégradé devrait en produire beaucoup plus", n)
	}
}

// userSpaceOnUse places the gradient by the current transform instead, so it does
// not follow the shape.
func TestLinearGradientUserSpaceOnUse(t *testing.T) {
	res := rasterise(t, `<svg viewBox="0 0 40 40">
	  <defs><linearGradient id="g" gradientUnits="userSpaceOnUse" x1="0" y1="0" x2="40" y2="0">
	    <stop offset="0" stop-color="#ff0000"/><stop offset="1" stop-color="#0000ff"/>
	  </linearGradient></defs>
	  <rect x="20" y="0" width="20" height="40" fill="url(#g)"/>
	</svg>`)
	// The rect covers the RIGHT half of user space, so it only shows the far end
	// of the gradient: no red anywhere in it.
	if c := res.at(21, 20); c.R > 160 {
		t.Errorf("bord gauche du rectangle = %v — le dégradé ne devrait pas repartir du rouge", c)
	}
}

func TestRadialGradient(t *testing.T) {
	res := rasterise(t, `<svg viewBox="0 0 40 40">
	  <defs><radialGradient id="g" cx="0.5" cy="0.5" r="0.5">
	    <stop offset="0" stop-color="#ffffff"/><stop offset="1" stop-color="#000000"/>
	  </radialGradient></defs>
	  <rect width="40" height="40" fill="url(#g)"/>
	</svg>`)
	mid, corner := res.at(20, 20), res.at(1, 1)
	if mid.R < 200 {
		t.Errorf("centre = %v, attendu clair", mid)
	}
	if corner.R > 80 {
		t.Errorf("coin = %v, attendu sombre", corner)
	}
}

// Percentage offsets, stop-opacity, and the named stop colours.
func TestGradientStopForms(t *testing.T) {
	res := rasterise(t, `<svg viewBox="0 0 40 40">
	  <defs><linearGradient id="g" x1="0" y1="0" x2="100%" y2="0">
	    <stop offset="0%" stop-color="black"/>
	    <stop offset="50%" stop-color="white" stop-opacity="1"/>
	    <stop offset="100%" stop-color="pas-une-couleur"/>
	  </linearGradient></defs>
	  <rect width="40" height="40" fill="url(#g)"/>
	</svg>`)
	if res.at(1, 20).R > 60 {
		t.Errorf("début = %v, attendu noir", res.at(1, 20))
	}
	if res.at(20, 20).R < 180 {
		t.Errorf("milieu = %v, attendu blanc", res.at(20, 20))
	}
}

// A gradient that declares no usable stop falls back to the flat colour rather
// than painting nothing.
func TestGradientWithoutStops(t *testing.T) {
	res := rasterise(t, `<svg viewBox="0 0 10 10">
	  <defs><linearGradient id="g"></linearGradient></defs>
	  <rect width="10" height="10" fill="url(#g)"/>
	</svg>`)
	if c := res.at(5, 5); c.A != 255 {
		t.Errorf("centre = %v, attendu peint à plat", c)
	}
}

// An unresolvable reference, and an outright unknown value, must leave the shape
// UNPAINTED. Inheriting instead is what rendered every gradient logo as a black
// square: the fill was not understood, so the black root colour flooded it.
func TestUnresolvedPaintDoesNotFloodWithBlack(t *testing.T) {
	for _, fill := range []string{"url(#absent)", "rgb(1,2,3)", "chartreuse"} {
		res := rasterise(t, `<svg viewBox="0 0 10 10"><rect width="10" height="10" fill="`+fill+`"/></svg>`)
		if c := res.at(5, 5); c.A != 0 {
			t.Errorf("fill=%q → %v, attendu transparent", fill, c)
		}
	}
}

func TestParsePaintRef(t *testing.T) {
	for _, c := range []struct {
		in   string
		want string
		ok   bool
	}{
		{"url(#g)", "g", true},
		{"url( #g )", "g", true},
		{"url('#g')", "g", true},
		{"url(#)", "", false},
		{"url(g)", "", false},
		{"url(#g", "", false},
		{"#g", "", false},
	} {
		got, ok := parsePaintRef(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("parsePaintRef(%q) = (%q, %v), attendu (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// ── traits ──────────────────────────────────────────────────────────────────

// A stroked path with no fill draws its outline and nothing else — the shape of
// every glyph in this fleet's logos.
func TestStrokeOnlyPath(t *testing.T) {
	res := rasterise(t, `<svg viewBox="0 0 40 40">
	  <g fill="none" stroke="#ffffff" stroke-width="6" stroke-linecap="round">
	    <path d="M8 20 H32"/>
	  </g>
	</svg>`)
	if c := res.at(20, 20); c.R != 255 || c.A != 255 {
		t.Errorf("sur le trait = %v, attendu blanc opaque", c)
	}
	if c := res.at(20, 4); c.A != 0 {
		t.Errorf("hors du trait = %v, attendu transparent", c)
	}
}

// The stroke width is a length in user units, so a scaled document scales it too.
func TestStrokeWidthFollowsTheTransform(t *testing.T) {
	doc := `<svg viewBox="0 0 40 40"><g fill="none" stroke="#ffffff" stroke-width="4"><path d="M0 20 H40"/></g></svg>`
	thin, err := Rasterize(doc, Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	thick, err := Rasterize(doc, Options{Scale: 4})
	if err != nil {
		t.Fatal(err)
	}
	count := func(pix []uint8) int {
		n := 0
		for i := 3; i < len(pix); i += 4 {
			if pix[i] > 128 {
				n++
			}
		}
		return n
	}
	// Four times the scale covers sixteen times the area, so the painted pixel
	// count must grow by about that much — not stay put, which is what an
	// unscaled width would do.
	if a, b := count(thin.Image.Pix), count(thick.Image.Pix); b < a*8 {
		t.Errorf("pixels peints: %d à l'échelle 1, %d à l'échelle 4 — la largeur ne suit pas la transformation", a, b)
	}
}

// A shape can be both filled and stroked, and the outline wins on the border.
func TestFillAndStroke(t *testing.T) {
	res := rasterise(t, `<svg viewBox="0 0 40 40">
	  <rect x="10" y="10" width="20" height="20" fill="#ff0000" stroke="#0000ff" stroke-width="4"/>
	</svg>`)
	if c := res.at(20, 20); c.R < 200 {
		t.Errorf("intérieur = %v, attendu rouge", c)
	}
	if c := res.at(20, 10); c.B < 200 {
		t.Errorf("bord = %v, attendu bleu", c)
	}
}

func TestStrokeNoneAndZeroWidth(t *testing.T) {
	for _, g := range []string{`stroke="none" stroke-width="6"`, `stroke="#fff" stroke-width="0"`} {
		res := rasterise(t, `<svg viewBox="0 0 20 20"><g fill="none" `+g+`><path d="M2 10 H18"/></g></svg>`)
		if c := res.at(10, 10); c.A != 0 {
			t.Errorf("%s → %v, attendu rien de peint", g, c)
		}
	}
}

// ── coins arrondis ──────────────────────────────────────────────────────────

// <rect rx> rounds the corners: the corner pixel is outside the shape while the
// centre of the edge is inside. Without rx every rounded-square logo in this
// fleet came out sharp.
func TestRectRoundedCorners(t *testing.T) {
	res := rasterise(t, `<svg viewBox="0 0 40 40"><rect width="40" height="40" rx="12" fill="#ff0000"/></svg>`)
	if c := res.at(0, 0); c.A != 0 {
		t.Errorf("coin = %v, attendu hors de la forme", c)
	}
	if c := res.at(20, 0); c.A == 0 {
		t.Errorf("milieu du bord haut = %v, attendu dans la forme", c)
	}
	if c := res.at(20, 20); c.R < 200 {
		t.Errorf("centre = %v, attendu rouge", c)
	}
}

// ry alone, and a radius larger than half the side, which SVG clamps.
func TestRectRadiusFormsAndClamp(t *testing.T) {
	only := rasterise(t, `<svg viewBox="0 0 40 40"><rect width="40" height="40" ry="12" fill="#ff0000"/></svg>`)
	if c := only.at(0, 0); c.A != 0 {
		t.Errorf("ry seul: coin = %v, attendu arrondi", c)
	}
	huge := rasterise(t, `<svg viewBox="0 0 40 40"><rect width="40" height="40" rx="999" fill="#ff0000"/></svg>`)
	// Clamped to w/2 and h/2, the rect becomes a disc: the centre is filled and
	// the corner is not.
	if c := huge.at(20, 20); c.R < 200 {
		t.Errorf("rx énorme: centre = %v, attendu rempli", c)
	}
	if c := huge.at(0, 0); c.A != 0 {
		t.Errorf("rx énorme: coin = %v, attendu vide", c)
	}
}

func TestMatrixScale(t *testing.T) {
	if got := (matrix{2, 0, 0, 2, 0, 0}).scale(); got != 2 {
		t.Errorf("scale() = %v, attendu 2", got)
	}
	if got := (matrix{0, 3, -3, 0, 0, 0}).scale(); got != 3 {
		t.Errorf("scale() d'une rotation-échelle = %v, attendu 3", got)
	}
}

// A radial gradient in userSpaceOnUse units takes its radius through the
// transform too, so scaling the document scales the disc.
func TestRadialGradientUserSpaceOnUse(t *testing.T) {
	doc := `<svg viewBox="0 0 40 40">
	  <defs><radialGradient id="g" gradientUnits="userSpaceOnUse" cx="20" cy="20" r="10">
	    <stop offset="0" stop-color="#ffffff"/><stop offset="1" stop-color="#000000"/>
	  </radialGradient></defs>
	  <rect width="40" height="40" fill="url(#g)"/>
	</svg>`
	res := rasterise(t, doc)
	if c := res.at(20, 20); c.R < 200 {
		t.Errorf("centre = %v, attendu clair", c)
	}
	// Ten user units out from the centre is the far stop: dark.
	if c := res.at(31, 20); c.R > 90 {
		t.Errorf("bord du disque = %v, attendu sombre", c)
	}
}

// An attribute this subset cannot read leaves the SVG default in place instead of
// corrupting the gradient's geometry.
func TestGradientAttributeThatDoesNotParse(t *testing.T) {
	res := rasterise(t, `<svg viewBox="0 0 40 40">
	  <defs><linearGradient id="g" x1="pas-un-nombre" x2="1" y1="0" y2="0">
	    <stop offset="0" stop-color="#ff0000"/><stop offset="pas-un-nombre" stop-color="#0000ff"/>
	  </linearGradient></defs>
	  <rect width="40" height="40" fill="url(#g)"/>
	</svg>`)
	if c := res.at(5, 20); c.A != 255 {
		t.Errorf("la forme devrait rester peinte: %v", c)
	}
}

// A paint server may carry children that are not stops — a <title>, a <desc>, an
// animation — and they must be stepped over rather than read as colours.
func TestGradientIgnoresNonStopChildren(t *testing.T) {
	res := rasterise(t, `<svg viewBox="0 0 40 40">
	  <defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0">
	    <title>un titre</title>
	    <stop offset="0" stop-color="#ff0000"/>
	    <desc>une description</desc>
	    <stop offset="1" stop-color="#0000ff"/>
	  </linearGradient></defs>
	  <rect width="40" height="40" fill="url(#g)"/>
	</svg>`)
	if c := res.at(1, 20); c.R < 200 {
		t.Errorf("bord gauche = %v, attendu rouge", c)
	}
	if c := res.at(38, 20); c.B < 200 {
		t.Errorf("bord droit = %v, attendu bleu", c)
	}
}
