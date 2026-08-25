// Copyright (c) the go-gfx/gfx authors.
// SPDX-License-Identifier: BSD-3-Clause

package svg

import (
	"image/color"
	"testing"
)

// centre reports the pixel at the middle of a 40x40 document.
func centre(t *testing.T, doc string) []uint8 {
	t.Helper()
	r, err := Rasterize(doc, Options{Scale: 1, Ink: color.RGBA{0, 0, 0, 255}, Paper: color.RGBA{255, 255, 255, 255}})
	if err != nil {
		t.Fatal(err)
	}
	i := (20*r.Image.W + 20) * 4
	return r.Image.Pix[i : i+4]
}

const bigSquare = `M 5 5 L 35 5 L 35 35 L 5 35 Z`

// Some elements hold content for later use and are never drawn where they stand.
// Painting them is not a small error: pgf wraps every clipped picture in a
// <clipPath> whose shape is the picture's own frame, so drawing it laid an
// opaque plate over the whole figure — a pgfplots axis came out as a black
// rectangle with the data drawn on top.
func TestElementsThatDefineAreNotPainted(t *testing.T) {
	for _, el := range []string{"defs", "clipPath", "mask", "symbol", "marker", "pattern"} {
		t.Run(el, func(t *testing.T) {
			doc := `<svg xmlns="http://www.w3.org/2000/svg" width="40pt" height="40pt" viewBox="0 0 40 40">` +
				`<g fill="black"><` + el + ` id="x"><path d="` + bigSquare + `"/></` + el + `></g></svg>`
			if px := centre(t, doc); px[0] < 200 {
				t.Errorf("le contenu d'un <%s> a été peint : %v", el, px)
			}
		})
	}
}

// The same shape outside them is still painted, so the rule skips the wrapper
// rather than the shape.
func TestAShapeOutsideThemIsStillPainted(t *testing.T) {
	doc := `<svg xmlns="http://www.w3.org/2000/svg" width="40pt" height="40pt" viewBox="0 0 40 40">` +
		`<g fill="black"><path d="` + bigSquare + `"/></g></svg>`
	if px := centre(t, doc); px[0] > 200 {
		t.Errorf("une forme ordinaire n'a pas été peinte : %v", px)
	}
}

// A gradient lives in <defs> and is collected separately from painting, so
// skipping <defs> while drawing must not lose it.
func TestAGradientInDefsStillPaints(t *testing.T) {
	doc := `<svg xmlns="http://www.w3.org/2000/svg" width="40pt" height="40pt" viewBox="0 0 40 40">` +
		`<defs><linearGradient id="g1" x1="0" y1="0" x2="1" y2="0">` +
		`<stop offset="0" stop-color="#f00"/><stop offset="1" stop-color="#f00"/>` +
		`</linearGradient></defs>` +
		`<path d="` + bigSquare + `" fill="url(#g1)"/></svg>`
	px := centre(t, doc)
	if px[0] < 200 || px[1] > 100 {
		t.Errorf("le dégradé défini dans <defs> ne peint pas rouge : %v", px)
	}
}
