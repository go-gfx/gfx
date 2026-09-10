// Copyright (c) 2026, the go-gfx authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file.

package svg

import (
	"strings"

	"github.com/go-gfx/gfx/vector"
)

// clipMask is one resolved <clipPath>, as coverage in device pixels: 0 where
// the clip takes the ink away, 255 where it keeps all of it.
//
// Coverage is held at 8 bits because a clip IS a mask and nothing more — the
// same resolution [RasterizeMask] hands callers — and because the alternative
// is expensive: a full-surface float64 grid costs 134 MB on a 4096-pixel
// render where this costs 16.
type clipMask struct {
	ox, oy, w, h int
	a            []uint8 // w*h coverage, row-major
}

// at reports the coverage the clip keeps at a device pixel. Everything outside
// the mask's own box is clipped away, because a <clipPath> covers nothing
// there.
func (c *clipMask) at(x, y int) uint8 {
	x -= c.ox
	y -= c.oy
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return 0
	}
	return c.a[y*c.w+x]
}

// clipKey identifies a resolved clip. The transform is part of the identity:
// clipPathUnits="userSpaceOnUse" — the default, and the only one this subset
// honours — reads the clip's coordinates in the user space of whatever
// references it, so the same <clipPath> under two transforms is two masks.
type clipKey struct {
	id string
	m  matrix
}

// clipFor resolves a clip-path reference to a mask, rasterising the clip's
// shapes once and caching the result under the transform it was built for.
//
// It returns nil when the reference does not resolve to a <clipPath>, when the
// clip is empty, or when clipPathUnits is objectBoundingBox — honouring that
// unit needs the bounding box of the shape being clipped, which is not known
// where a clip is inherited by a whole group. A nil mask leaves the element
// unclipped, which is what this package did for every clip before.
func (r *renderer) clipFor(id string, st state) *clipMask {
	n, ok := r.clipDefs[id]
	if !ok {
		return nil
	}
	if n.attrOr("clipPathUnits", "userSpaceOnUse") != "userSpaceOnUse" {
		return nil
	}
	key := clipKey{id: id, m: st.m}
	if m, hit := r.clipCache[key]; hit {
		return m
	}
	m := r.buildClip(n, st)
	if r.clipCache == nil {
		r.clipCache = map[clipKey]*clipMask{}
	}
	r.clipCache[key] = m
	return m
}

// buildClip rasterises the shapes inside a <clipPath> and unions their
// coverage. The union is a per-pixel maximum: two clip shapes that overlap
// still only keep the ink once, where adding the two coverages would darken
// the seam between them.
func (r *renderer) buildClip(n *xnode, st state) *clipMask {
	cst := st
	cst.fillRule = vector.NonZero
	if v, ok := n.attr("clip-rule"); ok {
		cst.fillRule = parseFillRule(v, cst.fillRule)
	}
	if v, ok := n.attr("transform"); ok {
		cst.m = st.m.mul(parseTransform(v))
	}

	acc := make([]uint8, r.img.W*r.img.H)
	minX, minY := r.img.W, r.img.H
	maxX, maxY := 0, 0
	painted := false

	var walk func(k *xnode, s state)
	walk = func(k *xnode, s state) {
		ks := s
		if v, ok := k.attr("transform"); ok {
			ks.m = s.m.mul(parseTransform(v))
		}
		if v, ok := k.attr("clip-rule"); ok {
			ks.fillRule = parseFillRule(v, s.fillRule)
		}
		p, ok := r.shapePath(k, ks)
		if !ok {
			// A container inside a <clipPath> contributes its children; <use>
			// is not resolved here, matching the renderer at large.
			for i := range k.Children {
				walk(&k.Children[i], ks)
			}
			return
		}
		var rz vector.Rasterizer
		cov, ox, oy, w, h, ok := rz.Fill(p, ks.fillRule, r.img.W, r.img.H)
		if !ok {
			return
		}
		painted = true
		for y := 0; y < h; y++ {
			row := (oy + y) * r.img.W
			for x := 0; x < w; x++ {
				v := cov[y*w+x]
				if v <= 0 {
					continue
				}
				// Coverage is a geometric area, so a region the path winds
				// twice is inside once rather than twice and v cannot pass 1
				// by more than rounding. min keeps the conversion in range
				// without a branch no input can take.
				if u := uint8(min(v, 1)*255 + 0.5); u > acc[row+ox+x] {
					acc[row+ox+x] = u
				}
			}
		}
		if ox < minX {
			minX = ox
		}
		if oy < minY {
			minY = oy
		}
		if ox+w > maxX {
			maxX = ox + w
		}
		if oy+h > maxY {
			maxY = oy + h
		}
	}
	for i := range n.Children {
		walk(&n.Children[i], cst)
	}
	if !painted || maxX <= minX || maxY <= minY {
		return &clipMask{} // an empty clip keeps nothing
	}

	w, h := maxX-minX, maxY-minY
	out := &clipMask{ox: minX, oy: minY, w: w, h: h, a: make([]uint8, w*h)}
	for y := 0; y < h; y++ {
		copy(out.a[y*w:(y+1)*w], acc[(minY+y)*r.img.W+minX:(minY+y)*r.img.W+minX+w])
	}
	return out
}

// applyClips multiplies a shape's coverage by every clip in force, in place,
// and reports whether anything is left to composite. Clips nest by
// intersection, which is what multiplying coverages does.
func applyClips(clips []*clipMask, cov []float64, ox, oy, w, h int) bool {
	if len(clips) == 0 {
		return true
	}
	any := false
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			if cov[i] <= 0 {
				continue
			}
			for _, c := range clips {
				cov[i] *= float64(c.at(ox+x, oy+y)) / 255
				if cov[i] <= 0 {
					break
				}
			}
			if cov[i] > 0 {
				any = true
			}
		}
	}
	return any
}

// parseFillRule reads a fill-rule or clip-rule value, keeping the inherited
// rule for "inherit" and for anything it does not recognise.
func parseFillRule(v string, inherit vector.FillRule) vector.FillRule {
	switch strings.TrimSpace(v) {
	case "evenodd":
		return vector.EvenOdd
	case "nonzero":
		return vector.NonZero
	}
	return inherit
}
