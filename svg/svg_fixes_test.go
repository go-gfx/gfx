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

// The cases below are shaped after resvg's own conformance corpus
// (crates/resvg/tests/tests/painting/…), which is where the edge cases come
// from rather than from guesswork.

// TestStrokeLinecapHonoured: the initial value is butt, and the renderer must
// not substitute its own.
//
// It did: gfx/svg reached the stroker through vector.Stroke, whose convenience
// signature hardcodes RoundCap and RoundJoin. Iconoir asks for round on every
// path, so the packs in use looked right and the substitution stayed invisible.
func TestStrokeLinecapHonoured(t *testing.T) {
	const tmpl = `<svg viewBox="0 0 24 24" fill="none" stroke-width="4">` +
		`<path d="M8 12H16" stroke="currentColor"%s/></svg>`
	ink := color.RGBA{A: 255}
	width := func(attr string) int {
		p, err := Rasterize(fmt.Sprintf(tmpl, attr), Options{Scale: 4, Ink: ink})
		if err != nil {
			t.Fatalf("Rasterize(%q): %v", attr, err)
		}
		// The painted extent along x, which is what a cap changes.
		minX, maxX := p.Image.W, -1
		for y := 0; y < p.Image.H; y++ {
			for x := 0; x < p.Image.W; x++ {
				if p.Image.Pix[(y*p.Image.W+x)*4+3] > 0 {
					if x < minX {
						minX = x
					}
					if x > maxX {
						maxX = x
					}
				}
			}
		}
		return maxX - minX + 1
	}
	butt := width(` stroke-linecap="butt"`)
	round := width(` stroke-linecap="round"`)
	square := width(` stroke-linecap="square"`)
	def := width("")
	if butt >= round {
		t.Errorf("round cap did not extend the line: butt=%d round=%d", butt, round)
	}
	if butt >= square {
		t.Errorf("square cap did not extend the line: butt=%d square=%d", butt, square)
	}
	if def != butt {
		t.Errorf("the default cap painted %d px wide, butt paints %d — the initial value is butt", def, butt)
	}
}

// TestFillRuleHonoured: a self-overlapping star leaves its centre empty under
// evenodd and filled under nonzero. The initial value is nonzero.
func TestFillRuleHonoured(t *testing.T) {
	const tmpl = `<svg viewBox="0 0 100 100">` +
		`<path fill="#000"%s d="M50 5 L20 95 L95 37 L5 37 L80 95 Z"/></svg>`
	ink := func(attr string) int {
		p, err := Rasterize(fmt.Sprintf(tmpl, attr), Options{Scale: 2})
		if err != nil {
			t.Fatalf("Rasterize(%q): %v", attr, err)
		}
		return opaquePixels(p)
	}
	nonzero := ink(` fill-rule="nonzero"`)
	evenodd := ink(` fill-rule="evenodd"`)
	def := ink("")
	if evenodd >= nonzero {
		t.Errorf("evenodd did not leave the centre empty: evenodd=%d nonzero=%d", evenodd, nonzero)
	}
	if def != nonzero {
		t.Errorf("the default rule inked %d, nonzero inks %d — the initial value is nonzero", def, nonzero)
	}
}

// TestOpacityHonoured: fill-opacity and stroke-opacity scale coverage, accept
// the percentage form resvg's corpus uses, and clamp out-of-range values.
func TestOpacityHonoured(t *testing.T) {
	const tmpl = `<svg viewBox="0 0 24 24"><rect x="4" y="4" width="16" height="16" fill="#000"%s/></svg>`
	mean := func(attr string) float64 {
		p, err := Rasterize(fmt.Sprintf(tmpl, attr), Options{Scale: 4, Ink: color.RGBA{A: 255}})
		if err != nil {
			t.Fatalf("Rasterize(%q): %v", attr, err)
		}
		sum := 0.0
		for i := 3; i < len(p.Image.Pix); i += 4 {
			sum += float64(p.Image.Pix[i])
		}
		return sum / float64(len(p.Image.Pix)/4)
	}
	full := mean("")
	half := mean(` fill-opacity="0.5"`)
	pct := mean(` fill-opacity="50%"`)
	none := mean(` fill-opacity="0"`)
	over := mean(` fill-opacity="3"`)
	junk := mean(` fill-opacity="not-a-number"`)
	if half >= full || half == 0 {
		t.Errorf("fill-opacity 0.5 did not halve coverage: full=%.1f half=%.1f", full, half)
	}
	if pct != half {
		t.Errorf(`"50%%" and "0.5" must agree: %.1f vs %.1f`, pct, half)
	}
	if none != 0 {
		t.Errorf("fill-opacity 0 still painted: %.1f", none)
	}
	if over != full {
		t.Errorf("fill-opacity above 1 must clamp: %.1f vs %.1f", over, full)
	}
	if junk != full {
		t.Errorf("an unparsable opacity must leave the inherited value: %.1f vs %.1f", junk, full)
	}

	const stroked = `<svg viewBox="0 0 24 24" fill="none" stroke-width="4">` +
		`<path d="M4 5 L20 19" stroke="currentColor"%s/></svg>`
	sfull, err := Rasterize(fmt.Sprintf(stroked, ""), Options{Scale: 4, Ink: color.RGBA{A: 255}})
	if err != nil {
		t.Fatal(err)
	}
	shalf, err := Rasterize(fmt.Sprintf(stroked, ` stroke-opacity="0.5"`), Options{Scale: 4, Ink: color.RGBA{A: 255}})
	if err != nil {
		t.Fatal(err)
	}
	if opaquePixels(shalf) >= opaquePixels(sfull) {
		t.Errorf("stroke-opacity did not reduce coverage: %d vs %d", opaquePixels(shalf), opaquePixels(sfull))
	}
}

// TestDashArrayGuards pins the three guards resvg's corpus documents: a
// negative value voids the whole list, a list summing to zero or less draws
// solid, and "none" is solid.
func TestDashArrayGuards(t *testing.T) {
	const tmpl = `<svg viewBox="0 0 24 24" fill="none" stroke-width="2">` +
		`<path d="M2 12H22" stroke="currentColor"%s/></svg>`
	ink := func(attr string) int {
		p, err := Rasterize(fmt.Sprintf(tmpl, attr), Options{Scale: 4, Ink: color.RGBA{A: 255}})
		if err != nil {
			t.Fatalf("Rasterize(%q): %v", attr, err)
		}
		return inkedPixels(p)
	}
	solid := ink("")
	dashed := ink(` stroke-dasharray="2 2"`)
	if dashed >= solid {
		t.Errorf("a dash pattern did not remove ink: dashed=%d solid=%d", dashed, solid)
	}
	for _, attr := range []string{
		` stroke-dasharray="none"`,
		` stroke-dasharray=""`,
		` stroke-dasharray="20 40 -20"`, // negative: technically undefined
		` stroke-dasharray="0 0"`,       // zero sum
	} {
		if got := ink(attr); got != solid {
			t.Errorf("%s should draw solid: inked %d, solid inks %d", attr, got, solid)
		}
	}
	// An offset shifts the pattern, so it must reach the stroker.
	if ink(` stroke-dasharray="4 4"`) == ink(` stroke-dasharray="4 4" stroke-dashoffset="4"`) {
		t.Error("stroke-dashoffset had no effect")
	}
}

// TestStrokeLinejoinHonoured: the initial value is miter, so a sharp corner
// runs to a point; round and bevel cut it back. A join only shows on a corner
// sharp enough for the three to differ, hence the narrow chevron.
func TestStrokeLinejoinHonoured(t *testing.T) {
	const tmpl = `<svg viewBox="0 0 40 40" fill="none" stroke-width="6">` +
		`<path d="M6 34 L20 8 L34 34" stroke="currentColor"%s/></svg>`
	reach := func(attr string) int {
		p, err := Rasterize(fmt.Sprintf(tmpl, attr), Options{Scale: 4, Ink: color.RGBA{A: 255}})
		if err != nil {
			t.Fatalf("Rasterize(%q): %v", attr, err)
		}
		// The topmost painted row: a miter spike reaches higher than a cut corner.
		for y := 0; y < p.Image.H; y++ {
			for x := 0; x < p.Image.W; x++ {
				if p.Image.Pix[(y*p.Image.W+x)*4+3] > 0 {
					return y
				}
			}
		}
		return p.Image.H
	}
	miter := reach(` stroke-linejoin="miter"`)
	round := reach(` stroke-linejoin="round"`)
	bevel := reach(` stroke-linejoin="bevel"`)
	def := reach("")
	if miter >= round || miter >= bevel {
		t.Errorf("miter did not reach furthest: miter=%d round=%d bevel=%d", miter, round, bevel)
	}
	if def != miter {
		t.Errorf("the default join reached %d, miter reaches %d — the initial value is miter", def, miter)
	}
	// A miter limit low enough turns the spike into a bevel, so it must reach
	// the stroker; one below 1 is an error and leaves the default alone.
	if reach(` stroke-miterlimit="1"`) <= miter {
		t.Error("stroke-miterlimit did not cut the spike back")
	}
	if reach(` stroke-miterlimit="0.5"`) != miter {
		t.Error("a miter limit below 1 is an error and must be ignored")
	}
	if reach(` stroke-miterlimit="nonsense"`) != miter {
		t.Error("an unparsable miter limit must be ignored")
	}
}

// TestOpacityAndDashParsingEdges covers the value forms the renderer must
// tolerate without a shape to look at: they are parser branches, and a branch no
// test reaches is a branch nobody has verified.
func TestOpacityAndDashParsingEdges(t *testing.T) {
	if got := parseOpacity("-1", 0.5); got != 0 {
		t.Errorf("parseOpacity(-1) = %v, want it clamped to 0", got)
	}
	if got := parseOpacity("0.25", 1); got != 0.25 {
		t.Errorf("parseOpacity(0.25) = %v", got)
	}
	if got := parseDashArray("   ", 100); got != nil {
		t.Errorf("a whitespace-only dasharray must be solid, got %v", got)
	}
	if got := parseDashArray(",,", 100); got != nil {
		t.Errorf("separators with no numbers must be solid, got %v", got)
	}
	if got := parseDashArray("10%", 100); len(got) != 1 || got[0] != 10 {
		t.Errorf("a percentage dash must resolve against the viewport, got %v", got)
	}
}
