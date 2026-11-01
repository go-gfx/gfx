// Copyright (c) the go-gfx authors.
// SPDX-License-Identifier: BSD-3-Clause

package svg

import (
	"image/color"
	"sort"

	"github.com/go-gfx/gfx/vector"
)

// Walking a document into something other than pixels.
//
// [Rasterize] draws an SVG onto a surface. A consumer that writes a VECTOR
// format — PDF, PostScript, another SVG — wants the same walk but not the
// pixels: the outlines, with the paint in force and the transform already
// applied. [Walk] is that, and the two share every line of parsing, inheritance
// and geometry, so a shape drawn one way is the shape drawn the other.
//
// ⛔ IT REPORTS WHAT IT COULD NOT EXPRESS, and that is the point rather than a
// caveat. A translation between vector languages that silently drops what it
// does not understand produces a file that OPENS and is quietly wrong, which is
// worse than one that was never attempted — go-pdfkit's SVGToPDF rasterised for
// exactly this reason. [WalkResult.Unsupported] names every feature the walk
// met and could not hand over, so a caller can fall back to drawing the
// document instead of shipping a half-translation.

// A Solid is a flat colour with the opacity in force.
//
// ⛔ THE TWO ARE NOT MULTIPLIED, and a consumer must not add them together
// either. [Rasterize] lays the colour down at FULL alpha and applies Opacity to
// the coverage instead — so a consumer reproducing the drawing paints Colour as
// opaque and carries Opacity separately, which in PDF is a /CA or /ca in an
// ExtGState rather than anything in the colour operator. Folding Opacity into
// the colour's own alpha paints it twice.
type Solid struct {
	Colour  color.RGBA
	Opacity float64 // 0..1, from fill-opacity or stroke-opacity
}

// A Shape is one outline with the paint that draws it. The path is in DEVICE
// space: the element's transform, the viewBox mapping and [Options.Scale] are
// already applied, so a consumer places it with no further arithmetic.
type Shape struct {
	Path     *vector.Path
	FillRule vector.FillRule

	// Fill is nil when the shape is not filled.
	Fill *Solid
	// Stroke is nil when the outline is not drawn.
	Stroke *Solid

	// StrokeWidth and the dash lengths are in DEVICE units, scaled by the same
	// transform as the path — a stroke width is a length in user units and
	// passes through the transform like any other.
	StrokeWidth float64
	Cap         vector.LineCap
	Join        vector.LineJoin
	MiterLimit  float64
	Dash        []float64
	DashPhase   float64
}

// A Sink receives the shapes of a walked document, in paint order: a shape
// handed over later is drawn over one handed over earlier.
type Sink interface {
	Shape(Shape)
}

// SinkFunc adapts a function to [Sink].
type SinkFunc func(Shape)

// Shape calls f.
func (f SinkFunc) Shape(s Shape) { f(s) }

// A WalkResult says what surface the shapes were laid on and what the walk
// could not express.
type WalkResult struct {
	// Width and Height are the device-space surface [Rasterize] would have
	// produced for the same document and options.
	Width, Height int

	// Unsupported names the features met and not handed to the sink, sorted
	// and without repeats: "gradient", "clip-path", "image".
	//
	// ⛔ A caller that must not be quietly wrong checks this is empty and
	// falls back to [Rasterize] when it is not. It is a list rather than a
	// boolean so that the fallback can be explained to somebody.
	Unsupported []string
}

// Walk parses doc and hands every shape it draws to sink, instead of drawing
// them. The sink is called in paint order.
//
// The same parsing, inheritance, viewBox mapping and geometry as [Rasterize] —
// it is the same walk with a different ending.
func Walk(doc string, opt Options, sink Sink) (*WalkResult, error) {
	r, st, w, h, err := prepare(doc, opt)
	if err != nil {
		return nil, err
	}
	r.sink = sink
	r.unsupported = map[string]bool{}
	for i := range r.root.Children {
		r.render(&r.root.Children[i], st)
	}
	out := &WalkResult{Width: w, Height: h}
	for k := range r.unsupported {
		out.Unsupported = append(out.Unsupported, k)
	}
	sort.Strings(out.Unsupported)
	return out, nil
}

// note records a feature the walk could not hand over.
func (r *renderer) note(what string) {
	if r.unsupported != nil {
		r.unsupported[what] = true
	}
}

// emit hands one shape to the sink, in place of rasterising it.
func (r *renderer) emit(p *vector.Path, st state) {
	sh := Shape{Path: p, FillRule: st.fillRule}

	if st.paint && st.fillOp > 0 {
		if st.fillRef != "" && r.paintExists(st.fillRef) {
			// A gradient cannot be handed over as a flat colour without
			// changing the picture, so the shape is reported as unsupported
			// rather than filled with an approximation of it.
			r.note("gradient")
		}
		sh.Fill = &Solid{Colour: st.fill, Opacity: st.fillOp}
	}
	if st.strokeOn && st.strokeW > 0 && st.strokeOp > 0 {
		if st.strokeRef != "" && r.paintExists(st.strokeRef) {
			r.note("gradient")
		}
		// The same scaling fillPath applies, so a consumer need not know the
		// transform to get the width right.
		scale := st.m.scale()
		sh.Stroke = &Solid{Colour: st.stroke, Opacity: st.strokeOp}
		sh.StrokeWidth = st.strokeW * scale
		sh.Cap, sh.Join, sh.MiterLimit = st.cap, st.join, st.miter
		sh.DashPhase = st.dashPhase * scale
		if len(st.dash) > 0 {
			sh.Dash = make([]float64, len(st.dash))
			for i, d := range st.dash {
				sh.Dash[i] = d * scale
			}
		}
	}
	if sh.Fill == nil && sh.Stroke == nil {
		return
	}
	r.sink.Shape(sh)
}
