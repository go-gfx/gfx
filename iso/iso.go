// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

// Package iso is go-gfx's isometric projection and primitive-drawing layer: the
// pure-Go equivalent of obelisk.js. It turns positions and dimensions in a 3-D
// grid into 2-D screen polygons (a [Projection]), assembles them into shaded
// solids — [Cube], [Brick], [Pyramid], [Slope], [Line] and [Side] — and paints
// a depth-sorted [Scene] of them onto a raster surface with correct overlap.
//
// It is deliberately just geometry and rasterization: it computes each solid's
// visible face polygons in screen space and fills them through
// [github.com/go-gfx/gfx/vector], shading each face from one base colour with
// [github.com/go-gfx/gfx/color.Shade] (top brightest, sides progressively
// darker). It carries no widget, layout, event or windowing concern — a
// higher-level isometric-diagram widget and file exporters consume this layer.
//
// The world axes are right-handed with +X to the screen right-and-down, +Y to
// the left-and-down and +Z straight up; see [Vec3] and [Projection].
package iso

import (
	"image/color"

	gfxcolor "github.com/go-gfx/gfx/color"
	"github.com/go-gfx/gfx/geometry"
)

// Face is one filled, convex polygon in screen space: the projected outline of a
// single visible face of a solid, together with the colour to fill it. A [Scene]
// builds a path from Poly and fills it; the winding order of Poly does not
// matter under the non-zero fill rule.
type Face struct {
	Poly  []geometry.Point
	Color color.RGBA
}

// Shading holds the per-face brightness factors an isometric solid applies to
// its base colour: Top for the up-facing face, Left for the +Y side and Right
// for the +X side. Each is multiplied into the base colour by
// [github.com/go-gfx/gfx/color.Shade]. The convention is Top brightest (1),
// sides progressively darker, so the three faces of a cube read as one lit
// solid.
type Shading struct{ Top, Left, Right float64 }

// DefaultShading is the shading a solid uses when its own Shading is the zero
// value: the up face at full brightness and the two side faces darkened, with
// the +Y (left) face lighter than the +X (right) face.
var DefaultShading = Shading{Top: 1.0, Left: 0.75, Right: 0.55}

// resolve returns s unless it is the zero value, in which case it returns
// [DefaultShading]. A Top of 0 would paint the whole solid black, so it is taken
// as "unset" and the default is substituted.
func (s Shading) resolve() Shading {
	if s == (Shading{}) {
		return DefaultShading
	}
	return s
}

// Shape is a placeable isometric primitive a [Scene] can depth-order and draw.
// The concrete solids ([Cube], [Brick], [Pyramid], [Slope], [Side]) additionally
// expose a Faces method returning their projected face polygons, which an
// exporter can consume directly without rasterizing.
type Shape interface {
	// Depth returns the shape's painter's-algorithm sort key; smaller draws
	// first (farther from the viewer).
	Depth(p *Projection) float64
	// render draws the shape onto the renderer's surface.
	render(p *Projection, r *renderer)
}

// shadeFace projects the world-space polygon corners through p and pairs the
// resulting screen polygon with base shaded by factor. It is the shared builder
// every solid's Faces method uses.
func shadeFace(p *Projection, factor float64, base color.RGBA, corners ...Vec3) Face {
	poly := make([]geometry.Point, len(corners))
	for i, c := range corners {
		poly[i] = p.Project(c)
	}
	return Face{Poly: poly, Color: gfxcolor.Shade(base, factor)}
}
