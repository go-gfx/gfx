// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package iso

import (
	"math"

	"github.com/go-gfx/gfx/geometry"
)

// Vec3 is a point in the isometric grid's right-handed world space: X runs to
// the screen right-and-down, Y to the screen left-and-down, and Z straight up.
// Coordinates are continuous float64 grid units — a cube one unit on a side
// occupies [X, X+1] x [Y, Y+1] x [Z, Z+1].
type Vec3 struct{ X, Y, Z float64 }

// V is shorthand for Vec3{x, y, z}.
func V(x, y, z float64) Vec3 { return Vec3{x, y, z} }

// Projection maps world-space grid coordinates to 2-D screen pixels and back.
//
// One world unit along +X projects to the screen vector (TileW/2, TileH/2)
// (right and down); one unit along +Y projects to (-TileW/2, TileH/2) (left and
// down); one unit along +Z projects to (0, -ZScale) (straight up). Origin is the
// screen pixel that world (0,0,0) lands on. The X and Y screen axes therefore
// each sit atan(TileH/TileW) below the horizontal, so the tile aspect ratio sets
// the viewing angle directly.
//
// The zero Projection is degenerate (zero tile size); construct one with [New],
// [NewDefault] or [NewFromAngle].
type Projection struct {
	Origin       geometry.Point // screen pixel of world (0,0,0)
	TileW, TileH float64        // screen width and height of one grid cell's diamond footprint
	ZScale       float64        // screen pixels one +Z unit rises
}

// New returns a Projection with an explicit tile footprint and vertical scale.
// TileW and TileH are the full screen width and height of one grid cell's
// diamond; ZScale is how many screen pixels one unit of height rises. All three
// should be positive for a well-formed view.
func New(origin geometry.Point, tileW, tileH, zScale float64) *Projection {
	return &Projection{Origin: origin, TileW: tileW, TileH: tileH, ZScale: zScale}
}

// NewDefault returns the classic 2:1 "pixel-art" isometric projection anchored
// at origin: a 64x32 tile with a 32-pixel height unit.
//
// The 2:1 width:height ratio places the ground axes at atan(1/2) ≈ 26.57° below
// the horizontal — the arrangement obelisk.js and most isometric tile art use,
// because a one-unit ground step is then exactly two pixels across for every one
// down, so anti-aliasing is unnecessary for crisp edges. ZScale is set to
// TileW/2 (equivalently TileH, since the tile is 2:1) so a unit cube's vertical
// side faces are as tall as the tile's top diamond is wide on the half-axis,
// which is the proportion that reads as a cube rather than a squat box.
func NewDefault(origin geometry.Point) *Projection {
	return New(origin, 64, 32, 32)
}

// NewFromAngle returns a Projection whose ground axes sit angleDeg below the
// horizontal, deriving TileH from TileW as TileW*tan(angleDeg). Passing 26.565°
// reproduces the 2:1 [NewDefault] tile; 30° gives the "true" isometric tile
// whose three cube faces are congruent. ZScale is taken as given.
func NewFromAngle(origin geometry.Point, tileW, angleDeg, zScale float64) *Projection {
	tileH := tileW * math.Tan(angleDeg*math.Pi/180)
	return New(origin, tileW, tileH, zScale)
}

// Project maps a world-space point to its screen pixel.
func (p *Projection) Project(v Vec3) geometry.Point {
	hx := p.TileW / 2
	hy := p.TileH / 2
	return geometry.Pt(
		p.Origin.X+(v.X-v.Y)*hx,
		p.Origin.Y+(v.X+v.Y)*hy-v.Z*p.ZScale,
	)
}

// Unproject is the inverse of [Project] for a known height z: it recovers the
// world X and Y whose projection at that z lands on screen point s. A screen
// pixel alone is ambiguous in 3-D (a whole ray of world points projects onto
// it), so the caller supplies the z-plane to intersect — for click-picking a
// tile on the ground that is z = 0. Project and Unproject round-trip: for any v,
// Unproject(Project(v), v.Z) == v up to floating-point rounding.
func (p *Projection) Unproject(s geometry.Point, z float64) Vec3 {
	hx := p.TileW / 2
	hy := p.TileH / 2
	dx := s.X - p.Origin.X              // = (X - Y) * hx
	dy := s.Y - p.Origin.Y + z*p.ZScale // = (X + Y) * hy
	a := dx / hx                        // X - Y
	b := dy / hy                        // X + Y
	return Vec3{X: (a + b) / 2, Y: (b - a) / 2, Z: z}
}

// Depth returns the painter's-algorithm sort key for a world point: world points
// with a smaller X+Y+Z sit farther from the viewer and must be drawn first, so a
// [Scene] fills its shapes in ascending Depth order for correct overlap. Height
// counts toward depth so a stacked solid correctly draws over the one beneath it.
func (p *Projection) Depth(v Vec3) float64 { return v.X + v.Y + v.Z }
