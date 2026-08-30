// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package svg

import (
	"fmt"
	"image/color"
	"testing"
)

// inkedPixels counts pixels with a non-zero alpha — i.e. something was painted.
func inkedPixels(p *Result) int {
	n := 0
	for i := 3; i < len(p.Image.Pix); i += 4 {
		if p.Image.Pix[i] != 0 {
			n++
		}
	}
	return n
}

// TestCompactDecimalCoords: "1.4.1" is two numbers (1.4 and .1), not one. A path
// written in that compact form must still fill.
func TestCompactDecimalCoords(t *testing.T) {
	// A filled quad whose coordinates use back-to-back decimals with no
	// separator: "M4 4 L28.5.5 28 28 4.4.4 Z" reads 8 numbers.
	doc := `<svg viewBox="0 0 32 32"><path fill="#000" d="M4 4L28.5.5 28 28 4.4.4Z"/></svg>`
	p, err := Rasterize(doc, Options{Scale: 4})
	if err != nil {
		t.Fatalf("Rasterize: %v", err)
	}
	if inkedPixels(p) == 0 {
		t.Fatal("a compact-decimal path painted nothing")
	}
}

// TestArcRenders: absolute and relative elliptical arcs fill, and the two arc
// flags may run straight into the next coordinate with no separator.
func TestArcRenders(t *testing.T) {
	for _, d := range []string{
		`M16 4A12 12 0 1 1 15.9 4Z`,            // absolute, near-full circle
		`M16 4a12 12 0 1 0 0.1 0Z`,             // relative
		`M4 16A12 12 0 016 16 12 12 0 0128 16`, // flags "01" unseparated
	} {
		doc := `<svg viewBox="0 0 32 32"><path fill="#000" d="` + d + `"/></svg>`
		p, err := Rasterize(doc, Options{Scale: 4})
		if err != nil {
			t.Fatalf("Rasterize(%q): %v", d, err)
		}
		if inkedPixels(p) == 0 {
			t.Errorf("arc path %q painted nothing", d)
		}
	}
}

// TestArcDegenerate: a zero-radius arc and a zero-length arc are skipped (they
// contribute no curve), so a path that is ONLY such an arc paints nothing but
// does not error.
func TestArcDegenerate(t *testing.T) {
	for _, d := range []string{
		`M4 4A0 0 0 0 1 28 28`,     // zero radius
		`M16 16A12 12 0 0 1 16 16`, // coincident endpoints
	} {
		doc := `<svg viewBox="0 0 32 32"><path fill="#000" d="` + d + `"/></svg>`
		if _, err := Rasterize(doc, Options{Scale: 2}); err != nil {
			t.Errorf("Rasterize(%q): %v", d, err)
		}
	}
}

// TestArcBadFlag: a flag that is neither 0 nor 1 makes the path malformed, so it
// is skipped without error (and paints nothing).
func TestArcBadFlag(t *testing.T) {
	doc := `<svg viewBox="0 0 32 32"><path fill="#000" d="M4 4A12 12 0 5 1 28 28Z"/></svg>`
	p, err := Rasterize(doc, Options{Scale: 2})
	if err != nil {
		t.Fatalf("Rasterize: %v", err)
	}
	if inkedPixels(p) != 0 {
		t.Fatal("a path with a bad arc flag should paint nothing")
	}
}

// TestNegativeViewBoxOrigin: a viewBox with a non-zero (negative) minimum places
// content correctly — the same shape drawn at the viewBox's own coordinates must
// fill, where before it landed off the raster.
func TestNegativeViewBoxOrigin(t *testing.T) {
	// A filled square in the visible region of a "0 -100 100 100" viewBox.
	doc := `<svg viewBox="0 -100 100 100"><path fill="#000" d="M10 -90H90V-10H10Z"/></svg>`
	p, err := Rasterize(doc, Options{Scale: 1, Ink: color.RGBA{0, 0, 0, 255}})
	if err != nil {
		t.Fatalf("Rasterize: %v", err)
	}
	if inkedPixels(p) == 0 {
		t.Fatal("content in a negative-origin viewBox painted nothing")
	}
	// The centre of the square (device ~ (50,50)) is inked.
	if pixAt(p, 50, 50).A == 0 {
		t.Fatal("the square's centre is not inked")
	}
}

// TestMalformedExponentSkipped: a number whose exponent has no digits ("1e")
// fails to parse, so the path is skipped without a crash.
func TestMalformedExponentSkipped(t *testing.T) {
	doc := `<svg viewBox="0 0 32 32"><path fill="#000" d="M1e 4H28V28H4Z"/></svg>`
	p, err := Rasterize(doc, Options{Scale: 2})
	if err != nil {
		t.Fatalf("Rasterize: %v", err)
	}
	if inkedPixels(p) != 0 {
		t.Fatal("a path with a malformed number should paint nothing")
	}
}

// TestArcWithoutMoveSkipped: an arc that starts a subpath with no preceding move
// is malformed and skipped.
func TestArcWithoutMoveSkipped(t *testing.T) {
	doc := `<svg viewBox="0 0 32 32"><path fill="#000" d="A12 12 0 0 1 28 28Z"/></svg>`
	p, err := Rasterize(doc, Options{Scale: 2})
	if err != nil {
		t.Fatalf("Rasterize: %v", err)
	}
	if inkedPixels(p) != 0 {
		t.Fatal("an arc with no preceding move should paint nothing")
	}
}

// opaquePixels counts pixels that are all but fully painted, which is what
// separates a filled interior from an anti-aliased outline.
func opaquePixels(p *Result) int {
	n := 0
	for i := 3; i < len(p.Image.Pix); i += 4 {
		if p.Image.Pix[i] > 200 {
			n++
		}
	}
	return n
}

// TestRootPresentationAttributesInherit: the root <svg> carries presentation
// attributes like any other element, and everything inside inherits them.
//
// Rasterize walks the root's CHILDREN, so for a while it never read the root's
// own attributes. Stroke-based icon packs put fill="none" on the root and
// stroke="currentColor" on each path; with the root's fill dropped, every closed
// path inherited the default fill and a magnifier rendered as a solid disc. The
// open handle path still stroked correctly, so the result looked deliberate
// rather than broken.
func TestRootPresentationAttributesInherit(t *testing.T) {
	// A ring: one closed path, stroked, with the paint state declared only on
	// the root — exactly the shape an icon pack ships.
	const ring = `<svg viewBox="0 0 24 24" fill="none" stroke-width="1.5">` +
		`<path d="M3 12C3 16.9706 7.02944 21 12 21C16.9706 21 21 16.9706 21 12C21 7.02944 16.9706 3 12 3C7.02944 3 3 7.02944 3 12Z" stroke="currentColor"/>` +
		`</svg>`
	got, err := Rasterize(ring, Options{Scale: 8, Ink: color.RGBA{A: 255}})
	if err != nil {
		t.Fatalf("Rasterize: %v", err)
	}
	if opaquePixels(got) == 0 {
		t.Fatal("the ring painted nothing at all")
	}

	// The same ring with the root's fill="none" removed, which is what the
	// renderer used to see. That one IS filled, and the contrast is the test:
	// comparing against a fraction of the canvas would only pin today's
	// anti-aliasing, while comparing the two answers pins the inheritance.
	const filled = `<svg viewBox="0 0 24 24" stroke-width="1.5">` +
		`<path d="M3 12C3 16.9706 7.02944 21 12 21C16.9706 21 21 16.9706 21 12C21 7.02944 16.9706 3 12 3C7.02944 3 3 7.02944 3 12Z" stroke="currentColor"/>` +
		`</svg>`
	ref, err := Rasterize(filled, Options{Scale: 8, Ink: color.RGBA{A: 255}})
	if err != nil {
		t.Fatalf("Rasterize: %v", err)
	}
	outline, disc := opaquePixels(got), opaquePixels(ref)
	if outline*2 >= disc {
		t.Errorf("root fill=\"none\" was not inherited: outline covers %d px, a filled disc %d — an outline must be a small fraction of the area it encloses",
			outline, disc)
	}
}

// TestRootStrokeWidthInherits: the root's stroke-width reaches a child that does
// not set one of its own. A pack that declares the width once, on the root, is
// otherwise drawn at the default weight.
func TestRootStrokeWidthInherits(t *testing.T) {
	const tmpl = `<svg viewBox="0 0 24 24" fill="none" stroke-width="%s">` +
		`<path d="M2 12H22" stroke="currentColor"/></svg>`
	thin, err := Rasterize(fmt.Sprintf(tmpl, "1"), Options{Scale: 8, Ink: color.RGBA{A: 255}})
	if err != nil {
		t.Fatalf("Rasterize: %v", err)
	}
	thick, err := Rasterize(fmt.Sprintf(tmpl, "6"), Options{Scale: 8, Ink: color.RGBA{A: 255}})
	if err != nil {
		t.Fatalf("Rasterize: %v", err)
	}
	if opaquePixels(thick) <= opaquePixels(thin) {
		t.Errorf("root stroke-width was not inherited: width 6 inked %d px, width 1 inked %d",
			opaquePixels(thick), opaquePixels(thin))
	}
}
