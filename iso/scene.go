// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package iso

import (
	"image/color"
	"sort"

	"github.com/go-gfx/gfx/geometry"
	"github.com/go-gfx/gfx/raster"
	"github.com/go-gfx/gfx/vector"
)

// renderer is the shared drawing context handed to each shape's render method:
// the destination surface plus one reused vector rasterizer, so a whole scene
// amortises to near-zero allocation.
type renderer struct {
	dst *raster.Image
	rz  *vector.Rasterizer
}

// fill rasterizes one face polygon and composites it in its flat colour with
// anti-aliased edges. A polygon that encloses no area (a face collapsed to an
// edge, e.g. the short side of a slope) rasterizes to no coverage and is left
// undrawn.
func (r *renderer) fill(f Face) {
	path := vector.NewPath()
	path.MoveTo(f.Poly[0].X, f.Poly[0].Y)
	for _, pt := range f.Poly[1:] {
		path.LineTo(pt.X, pt.Y)
	}
	path.Close()
	cov, ox, oy, w, h, ok := r.rz.Fill(path, vector.NonZero, r.dst.W, r.dst.H)
	if !ok {
		return
	}
	vector.Composite(r.dst, cov, ox, oy, w, h, vector.SolidPaint{Color: f.Color})
}

// fillFaces fills a slice of faces in order.
func (r *renderer) fillFaces(faces []Face) {
	for _, f := range faces {
		r.fill(f)
	}
}

// stroke rasterizes a round-capped line between two screen points and
// composites it in colour c. A degenerate or fully off-surface segment draws
// nothing.
func (r *renderer) stroke(a, b geometry.Point, width float64, c color.RGBA) {
	path := vector.NewPath().MoveTo(a.X, a.Y).LineTo(b.X, b.Y)
	cov, ox, oy, w, h, ok := r.rz.Stroke(path, width, r.dst.W, r.dst.H)
	if !ok {
		return
	}
	vector.Composite(r.dst, cov, ox, oy, w, h, vector.SolidPaint{Color: c})
}

// Scene is an ordered collection of isometric [Shape]s sharing one [Projection].
// Render draws them back-to-front so nearer solids correctly overlap farther
// ones. The zero Scene is not usable; make one with [NewScene].
type Scene struct {
	Proj   *Projection
	shapes []Shape
}

// NewScene returns an empty Scene that projects its shapes through p.
func NewScene(p *Projection) *Scene { return &Scene{Proj: p} }

// Add appends shapes to the scene, preserving insertion order (which breaks ties
// between shapes at equal depth). It returns the scene so calls chain.
func (s *Scene) Add(shapes ...Shape) *Scene {
	s.shapes = append(s.shapes, shapes...)
	return s
}

// Shapes returns the scene's shapes in insertion order.
func (s *Scene) Shapes() []Shape { return s.shapes }

// Render draws every shape onto dst in ascending [Projection.Depth] order — the
// painter's algorithm — so a shape nearer the viewer is composited over the ones
// behind it. Ties keep insertion order (a stable sort). The sort orders a copy,
// leaving the scene's own insertion order untouched.
func (s *Scene) Render(dst *raster.Image) {
	order := make([]Shape, len(s.shapes))
	copy(order, s.shapes)
	sort.SliceStable(order, func(i, j int) bool {
		return order[i].Depth(s.Proj) < order[j].Depth(s.Proj)
	})
	r := &renderer{dst: dst, rz: &vector.Rasterizer{}}
	for _, sh := range order {
		sh.render(s.Proj, r)
	}
}
