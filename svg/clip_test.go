// Copyright (c) 2026, the go-gfx authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file.

package svg

import "testing"

// opaque reports whether a pixel carries full ink, and clear whether it carries
// none. Anti-aliased edges sit between the two, so tests sample well inside a
// region rather than on its boundary.
func opaque(t *testing.T, r *result64, x, y int, why string) {
	t.Helper()
	if c := r.at(x, y); c.A < 250 {
		t.Errorf("%s: (%d,%d) alpha = %d, want opaque", why, x, y, c.A)
	}
}

func clear(t *testing.T, r *result64, x, y int, why string) {
	t.Helper()
	if c := r.at(x, y); c.A > 5 {
		t.Errorf("%s: (%d,%d) alpha = %d, want nothing", why, x, y, c.A)
	}
}

// TestAClipKeepsOnlyWhatItCovers is the whole point of clip-path: the shape is
// painted through the clip's silhouette and nowhere else. Before this, a
// clip-path was ignored and the shape covered everything it was meant to be
// cut down to.
func TestAClipKeepsOnlyWhatItCovers(t *testing.T) {
	r := rasterise(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
	  <defs><clipPath id="c"><circle cx="50" cy="50" r="20"/></clipPath></defs>
	  <rect width="100" height="100" fill="#FF0000" clip-path="url(#c)"/>
	</svg>`)
	opaque(t, r, 50, 50, "inside the clip")
	clear(t, r, 5, 5, "outside the clip")
}

// TestAClipCanPunchAHole covers the shape a ring needs: an outer boundary with
// an inner one taken out of it, which is what clip-rule="evenodd" decides.
func TestAClipCanPunchAHole(t *testing.T) {
	r := rasterise(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
	  <defs><clipPath id="c"><path clip-rule="evenodd"
	    d="M0 0H100V100H0Z M50 30 A20 20 0 1 0 50 70 A20 20 0 1 0 50 30 Z"/></clipPath></defs>
	  <rect width="100" height="100" fill="#FF0000" clip-path="url(#c)"/>
	</svg>`)
	opaque(t, r, 5, 5, "outside the hole")
	clear(t, r, 50, 50, "inside the hole")
}

// TestClipsNestByIntersection: a clip on a group and a clip on the shape inside
// it both apply, and what survives is the overlap. Replacing rather than
// intersecting would let a child widen what its parent had already narrowed.
func TestClipsNestByIntersection(t *testing.T) {
	r := rasterise(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
	  <defs>
	    <clipPath id="left"><rect width="50" height="100"/></clipPath>
	    <clipPath id="top"><rect width="100" height="50"/></clipPath>
	  </defs>
	  <g clip-path="url(#left)">
	    <rect width="100" height="100" fill="#FF0000" clip-path="url(#top)"/>
	  </g>
	</svg>`)
	opaque(t, r, 25, 25, "top-left quadrant, in both clips")
	clear(t, r, 75, 25, "right of the group's clip")
	clear(t, r, 25, 75, "below the shape's clip")
}

// TestAClipThatDoesNotResolveLeavesTheShapeAlone: a reference to something that
// is not there must not silently delete the artwork. Painting it whole is what
// this package did before clips existed, and is the safer of the two failures.
func TestAClipThatDoesNotResolveLeavesTheShapeAlone(t *testing.T) {
	r := rasterise(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
	  <rect width="100" height="100" fill="#FF0000" clip-path="url(#absent)"/>
	</svg>`)
	opaque(t, r, 50, 50, "unresolved clip")
	opaque(t, r, 5, 5, "unresolved clip")
}

// TestObjectBoundingBoxClipIsNotHonoured pins a documented limitation rather
// than a wish: those units are fractions of the clipped shape's bounding box,
// which is not known where a clip is inherited by a whole group. The clip is
// dropped, so the shape is painted whole.
func TestObjectBoundingBoxClipIsNotHonoured(t *testing.T) {
	r := rasterise(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
	  <defs><clipPath id="c" clipPathUnits="objectBoundingBox">
	    <rect width="0.5" height="1"/></clipPath></defs>
	  <rect width="100" height="100" fill="#FF0000" clip-path="url(#c)"/>
	</svg>`)
	opaque(t, r, 75, 50, "the clip is dropped, not applied")
}

// TestAClipFollowsTheTransformInForce: clipPathUnits is userSpaceOnUse, so the
// clip's coordinates are read in the user space of whatever references it. A
// clip cached without the transform in its key would be reused at the wrong
// place.
func TestAClipFollowsTheTransformInForce(t *testing.T) {
	r := rasterise(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
	  <defs><clipPath id="c"><rect width="20" height="20"/></clipPath></defs>
	  <rect width="100" height="100" fill="#FF0000" clip-path="url(#c)"/>
	  <g transform="translate(60,60)">
	    <rect width="100" height="100" fill="#0000FF" clip-path="url(#c)"/>
	  </g>
	</svg>`)
	opaque(t, r, 10, 10, "the untranslated clip")
	opaque(t, r, 70, 70, "the same clip, translated")
	clear(t, r, 40, 40, "between the two")
}

// TestFillRuleEvenOdd: two sub-paths wound the same way merge under the
// non-zero rule and cancel under even-odd. Filling was hard-wired to non-zero
// before, so no document could describe a hole.
func TestFillRuleEvenOdd(t *testing.T) {
	const d = "M10 10H90V90H10Z M30 30H70V70H30Z"
	solid := rasterise(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
	  <path d="`+d+`" fill="#FF0000"/></svg>`)
	opaque(t, solid, 50, 50, "non-zero is the default and merges the two")

	holed := rasterise(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
	  <path d="`+d+`" fill="#FF0000" fill-rule="evenodd"/></svg>`)
	opaque(t, holed, 20, 50, "the ring itself")
	clear(t, holed, 50, 50, "even-odd takes the middle out")
}

// TestStrokeLinecap: SVG's initial cap is butt, and asking for round or square
// carries the stroke past its end point. The rasteriser could always do this;
// nothing read the attribute, so every stroke came out round.
func TestStrokeLinecap(t *testing.T) {
	doc := func(cap string) string {
		return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
		  <path d="M20 50H80" stroke="#FF0000" stroke-width="10" fill="none"` + cap + `/></svg>`
	}
	clear(t, rasterise(t, doc(``)), 17, 50, "no attribute: butt is the initial value")
	clear(t, rasterise(t, doc(` stroke-linecap="butt"`)), 17, 50, "butt stops at the end point")
	opaque(t, rasterise(t, doc(` stroke-linecap="round"`)), 17, 50, "round carries past it")
	opaque(t, rasterise(t, doc(` stroke-linecap="square"`)), 17, 50, "square carries past it")
}

// TestStrokeLinejoin: a miter carries the outer edges to where they cross, a
// bevel cuts the corner off.
func TestStrokeLinejoin(t *testing.T) {
	doc := func(join string) string {
		return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
		  <path d="M20 20H80V80" stroke="#FF0000" stroke-width="20" fill="none"` + join + `/></svg>`
	}
	opaque(t, rasterise(t, doc(``)), 88, 12, "no attribute: miter is the initial value")
	opaque(t, rasterise(t, doc(` stroke-linejoin="miter"`)), 88, 12, "the miter reaches the crossing")
	clear(t, rasterise(t, doc(` stroke-linejoin="bevel"`)), 88, 12, "the bevel cuts it away")
}

// TestOneClipServesEveryShapeThatNamesIt: a clip's coverage is kept under its
// id and the transform in force, so a document that cuts ten shapes with one
// <clipPath> rasterises it once. Both shapes still have to come out cut.
func TestOneClipServesEveryShapeThatNamesIt(t *testing.T) {
	r := rasterise(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
	  <defs><clipPath id="c"><rect width="50" height="100"/></clipPath></defs>
	  <rect width="100" height="50" fill="#FF0000" clip-path="url(#c)"/>
	  <rect y="50" width="100" height="50" fill="#0000FF" clip-path="url(#c)"/>
	</svg>`)
	opaque(t, r, 25, 25, "first shape, inside the clip")
	opaque(t, r, 25, 75, "second shape, inside the clip")
	clear(t, r, 75, 25, "first shape, outside the clip")
	clear(t, r, 75, 75, "second shape, outside the clip")
}

// TestAClipPathCarriesItsOwnRuleAndTransform: clip-rule and transform on the
// <clipPath> element apply to everything inside it. A ring cut from one path
// and moved into place needs both, and taking them only from the shapes would
// silently drop them.
func TestAClipPathCarriesItsOwnRuleAndTransform(t *testing.T) {
	r := rasterise(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
	  <defs><clipPath id="c" clip-rule="evenodd" transform="translate(50 0)">
	    <path d="M0 0H50V100H0Z M10 40H40V60H10Z"/></clipPath></defs>
	  <rect width="100" height="100" fill="#FF0000" clip-path="url(#c)"/>
	</svg>`)
	opaque(t, r, 55, 10, "where the clip's transform put it")
	clear(t, r, 5, 10, "where it would have been without the transform")
	clear(t, r, 75, 50, "the hole evenodd takes out")
}

// TestAShapeInsideAClipCarriesItsOwnTransform: the shapes in a <clipPath> are
// transformed like any others, on top of whatever the clip itself carries.
func TestAShapeInsideAClipCarriesItsOwnTransform(t *testing.T) {
	r := rasterise(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
	  <defs><clipPath id="c">
	    <circle cx="0" cy="0" r="20" transform="translate(50 50)"/></clipPath></defs>
	  <rect width="100" height="100" fill="#FF0000" clip-path="url(#c)"/>
	</svg>`)
	opaque(t, r, 50, 50, "where the transform put the circle")
	clear(t, r, 5, 5, "where the circle sat before it")
}

// TestAGroupInsideAClipContributesItsChildren: a <clipPath> may hold containers,
// and what clips is the shapes underneath them. Skipping the group would leave
// the clip empty, which keeps nothing at all — the opposite of what it says.
func TestAGroupInsideAClipContributesItsChildren(t *testing.T) {
	r := rasterise(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
	  <defs><clipPath id="c"><g><circle cx="50" cy="50" r="20"/></g></clipPath></defs>
	  <rect width="100" height="100" fill="#FF0000" clip-path="url(#c)"/>
	</svg>`)
	opaque(t, r, 50, 50, "inside the circle the group holds")
	clear(t, r, 5, 5, "outside it")
}

// TestAClipThatCoversNoPixelKeepsNothing: an empty <clipPath>, and one holding
// only a shape that falls outside the canvas, both cut everything away. A clip
// that covers nothing is not the same as no clip at all, and treating the two
// alike would paint the shape over the whole page.
func TestAClipThatCoversNoPixelKeepsNothing(t *testing.T) {
	for _, c := range []struct{ why, defs string }{
		{"a clip with no shapes in it", `<clipPath id="c"></clipPath>`},
		{"a clip whose only shape is off the canvas",
			`<clipPath id="c"><circle cx="-500" cy="-500" r="10"/></clipPath>`},
	} {
		r := rasterise(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
		  <defs>`+c.defs+`</defs>
		  <rect width="100" height="100" fill="#FF0000" clip-path="url(#c)"/>
		</svg>`)
		clear(t, r, 50, 50, c.why)
	}
}

// TestAPathThatWindsTwiceStillOnlyOpensTheClipOnce: under the nonzero rule a
// path laid over itself rasterises to twice full coverage. Stored unclamped in
// the byte a mask keeps, that wraps round to nearly nothing and the clip closes
// where it should be widest.
func TestAPathThatWindsTwiceStillOnlyOpensTheClipOnce(t *testing.T) {
	r := rasterise(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
	  <defs><clipPath id="c"><path d="M10 10H70V70H10Z M10 10H70V70H10Z"/></clipPath></defs>
	  <rect width="100" height="100" fill="#FF0000" clip-path="url(#c)"/>
	</svg>`)
	opaque(t, r, 40, 40, "inside the doubly wound square")
	clear(t, r, 90, 90, "outside it")
}

// TestAClippedShapeIsNotWidenedToItsBoundingBox: the clip is applied over the
// shape's bounding box, and the corners of a circle's box carry no ink to
// begin with. They must still carry none afterwards.
func TestAClippedShapeIsNotWidenedToItsBoundingBox(t *testing.T) {
	r := rasterise(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
	  <defs><clipPath id="c"><rect width="100" height="100"/></clipPath></defs>
	  <circle cx="50" cy="50" r="30" fill="#FF0000" clip-path="url(#c)"/>
	</svg>`)
	opaque(t, r, 50, 50, "the middle of the circle")
	clear(t, r, 21, 21, "the corner of its bounding box")
}

// TestARuleNobodySpellsLeavesTheInheritedOne: fill-rule and clip-rule name one
// rule, and a value that is neither of the two it defines is not a reason to
// change it. Reading an unknown keyword as evenodd would punch a hole through
// every shape drawn with two subpaths.
func TestARuleNobodySpellsLeavesTheInheritedOne(t *testing.T) {
	r := rasterise(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
	  <defs><clipPath id="c"><path clip-rule="sideways"
	    d="M0 0H100V100H0Z M30 30H70V70H30Z"/></clipPath></defs>
	  <rect width="100" height="100" fill="#FF0000" clip-path="url(#c)"/>
	</svg>`)
	opaque(t, r, 5, 5, "the outer square")
	opaque(t, r, 50, 50, "the inner one, which nonzero fills and evenodd would not")
}
