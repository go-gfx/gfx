// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package iso

import (
	"image/color"

	"github.com/go-gfx/gfx/geometry"
)

// Dimension is a solid's extent in grid units along each world axis: W along +X,
// H along +Y and D (depth/height) along +Z.
type Dimension struct{ W, H, D float64 }

// Brick is an axis-aligned box occupying [Pos, Pos+Dim]. Viewed isometrically it
// shows three faces — the +X side (right), the +Y side (left) and the top —
// each filled from Color darkened by the matching [Shading] factor.
type Brick struct {
	Pos     Vec3
	Dim     Dimension
	Color   color.RGBA
	Shading Shading
}

// Faces returns the box's three visible faces: right (+X), left (+Y) then top.
// The three faces of a convex box seen from above never overlap, so the order is
// cosmetic.
func (b Brick) Faces(p *Projection) []Face {
	sh := b.Shading.resolve()
	x0, y0, z0 := b.Pos.X, b.Pos.Y, b.Pos.Z
	x1, y1, z1 := x0+b.Dim.W, y0+b.Dim.H, z0+b.Dim.D
	return []Face{
		shadeFace(p, sh.Right, b.Color, // +X face
			V(x1, y0, z0), V(x1, y1, z0), V(x1, y1, z1), V(x1, y0, z1)),
		shadeFace(p, sh.Left, b.Color, // +Y face
			V(x0, y1, z0), V(x1, y1, z0), V(x1, y1, z1), V(x0, y1, z1)),
		shadeFace(p, sh.Top, b.Color, // top face
			V(x0, y0, z1), V(x1, y0, z1), V(x1, y1, z1), V(x0, y1, z1)),
	}
}

// Depth returns the box's near-corner sort key.
func (b Brick) Depth(p *Projection) float64 { return p.Depth(b.Pos) }

func (b Brick) render(p *Projection, r *renderer) { r.fillFaces(b.Faces(p)) }

// Cube is the special case of a [Brick] that is Size units on every axis.
type Cube struct {
	Pos     Vec3
	Size    float64
	Color   color.RGBA
	Shading Shading
}

// asBrick expands the cube to the equivalent equal-sided brick.
func (c Cube) asBrick() Brick {
	return Brick{Pos: c.Pos, Dim: Dimension{c.Size, c.Size, c.Size}, Color: c.Color, Shading: c.Shading}
}

// Faces returns the cube's three visible faces (see [Brick.Faces]).
func (c Cube) Faces(p *Projection) []Face { return c.asBrick().Faces(p) }

// Depth returns the cube's near-corner sort key.
func (c Cube) Depth(p *Projection) float64 { return p.Depth(c.Pos) }

func (c Cube) render(p *Projection, r *renderer) { r.fillFaces(c.Faces(p)) }

// Pyramid is a solid with a Dim.W x Dim.H rectangular base at Pos and an apex
// centred Dim.D above it. Viewed isometrically it shows its two front triangular
// faces — the +X side (right) and the +Y side (left) — meeting at the apex.
type Pyramid struct {
	Pos     Vec3
	Dim     Dimension
	Color   color.RGBA
	Shading Shading
}

// Faces returns the pyramid's two visible triangular faces: right (+X) then left
// (+Y).
func (py Pyramid) Faces(p *Projection) []Face {
	sh := py.Shading.resolve()
	x0, y0, z0 := py.Pos.X, py.Pos.Y, py.Pos.Z
	x1, y1 := x0+py.Dim.W, y0+py.Dim.H
	apex := V((x0+x1)/2, (y0+y1)/2, z0+py.Dim.D)
	return []Face{
		shadeFace(p, sh.Right, py.Color, V(x1, y0, z0), V(x1, y1, z0), apex), // +X face
		shadeFace(p, sh.Left, py.Color, V(x0, y1, z0), V(x1, y1, z0), apex),  // +Y face
	}
}

// Depth returns the pyramid's near-corner sort key.
func (py Pyramid) Depth(p *Projection) float64 { return p.Depth(py.Pos) }

func (py Pyramid) render(p *Projection, r *renderer) { r.fillFaces(py.Faces(p)) }

// SlopeDir names which grid edge of a [Slope] is raised to full height; the
// opposite edge stays at the base, so the top ramps down toward it.
type SlopeDir int

const (
	// SlopeE raises the +X (east) edge.
	SlopeE SlopeDir = iota
	// SlopeW raises the -X (west) edge.
	SlopeW
	// SlopeN raises the -Y (north) edge.
	SlopeN
	// SlopeS raises the +Y (south) edge.
	SlopeS
)

// Slope is a wedge: a Dim.W x Dim.H base at Pos whose top is a flat ramp, raised
// to Dim.D along the [SlopeDir] edge and meeting the base at the opposite edge.
// It shows the ramp (top), the +X side (right) and the +Y side (left); whichever
// side collapses to the base edge has zero area and is skipped.
type Slope struct {
	Pos     Vec3
	Dim     Dimension
	Dir     SlopeDir
	Color   color.RGBA
	Shading Shading
}

// Faces returns the wedge's faces: right (+X), left (+Y) then the ramp (top).
func (s Slope) Faces(p *Projection) []Face {
	sh := s.Shading.resolve()
	x0, y0, z0 := s.Pos.X, s.Pos.Y, s.Pos.Z
	x1, y1, z1 := x0+s.Dim.W, y0+s.Dim.H, z0+s.Dim.D

	// topZ is the height of the ramp above corner (cx, cy): full at the raised
	// edge, base at the opposite one.
	topZ := func(cx, cy float64) float64 {
		switch s.Dir {
		case SlopeE:
			if cx == x1 {
				return z1
			}
		case SlopeW:
			if cx == x0 {
				return z1
			}
		case SlopeN:
			if cy == y0 {
				return z1
			}
		case SlopeS:
			if cy == y1 {
				return z1
			}
		}
		return z0
	}
	t00, t10 := topZ(x0, y0), topZ(x1, y0)
	t11, t01 := topZ(x1, y1), topZ(x0, y1)

	return []Face{
		shadeFace(p, sh.Right, s.Color, // +X face, base up to the ramp edge at x1
			V(x1, y0, z0), V(x1, y1, z0), V(x1, y1, t11), V(x1, y0, t10)),
		shadeFace(p, sh.Left, s.Color, // +Y face, base up to the ramp edge at y1
			V(x0, y1, z0), V(x1, y1, z0), V(x1, y1, t11), V(x0, y1, t01)),
		shadeFace(p, sh.Top, s.Color, // the ramp itself
			V(x0, y0, t00), V(x1, y0, t10), V(x1, y1, t11), V(x0, y1, t01)),
	}
}

// Depth returns the wedge's near-corner sort key.
func (s Slope) Depth(p *Projection) float64 { return p.Depth(s.Pos) }

func (s Slope) render(p *Projection, r *renderer) { r.fillFaces(s.Faces(p)) }

// SidePlane names the plane a [Side] wall lies in: SideXZ spans the X and Z axes
// (a wall facing along Y), SideYZ spans Y and Z (facing along X).
type SidePlane int

const (
	// SideXZ is a wall spanning X (width) and Z (height) at a fixed Y; it faces
	// +Y and is shaded as a left face.
	SideXZ SidePlane = iota
	// SideYZ is a wall spanning Y (width) and Z (height) at a fixed X; it faces
	// +X and is shaded as a right face.
	SideYZ
)

// Side is a single flat rectangular wall: W units wide along its plane's ground
// axis and H units tall along Z, anchored at Pos. It is the standalone face
// obelisk draws as SideX / SideY — a panel with no thickness.
type Side struct {
	Pos     Vec3
	W, H    float64
	Plane   SidePlane
	Color   color.RGBA
	Shading Shading
}

// Faces returns the wall's single face.
func (s Side) Faces(p *Projection) []Face {
	sh := s.Shading.resolve()
	x0, y0, z0 := s.Pos.X, s.Pos.Y, s.Pos.Z
	if s.Plane == SideYZ { // spans Y and Z at fixed X -> right (+X) shading
		return []Face{shadeFace(p, sh.Right, s.Color,
			V(x0, y0, z0), V(x0, y0+s.W, z0), V(x0, y0+s.W, z0+s.H), V(x0, y0, z0+s.H))}
	}
	// SideXZ: spans X and Z at fixed Y -> left (+Y) shading
	return []Face{shadeFace(p, sh.Left, s.Color,
		V(x0, y0, z0), V(x0+s.W, y0, z0), V(x0+s.W, y0, z0+s.H), V(x0, y0, z0+s.H))}
}

// Depth returns the wall's anchor sort key.
func (s Side) Depth(p *Projection) float64 { return p.Depth(s.Pos) }

func (s Side) render(p *Projection, r *renderer) { r.fillFaces(s.Faces(p)) }

// Line is a straight segment between two world points, drawn as a stroked line
// Width pixels thick (1 when Width <= 0). It is the axis / edge primitive — no
// shading, just Color.
type Line struct {
	From, To Vec3
	Color    color.RGBA
	Width    float64
}

// Segment returns the line's projected screen endpoints.
func (l Line) Segment(p *Projection) (a, b geometry.Point) {
	return p.Project(l.From), p.Project(l.To)
}

// Depth returns the sort key of the segment's midpoint.
func (l Line) Depth(p *Projection) float64 {
	mid := Vec3{(l.From.X + l.To.X) / 2, (l.From.Y + l.To.Y) / 2, (l.From.Z + l.To.Z) / 2}
	return p.Depth(mid)
}

func (l Line) render(p *Projection, r *renderer) {
	w := l.Width
	if w <= 0 {
		w = 1
	}
	a, b := l.Segment(p)
	r.stroke(a, b, w, l.Color)
}
