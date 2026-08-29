// Copyright (c) 2026, the go-gfx authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file.

package svg

import (
	"image"
	"image/color"
)

// RasterizeMask renders a document to coverage alone: an 8-bit alpha mask,
// with no colour in it.
//
// It is what an ICON is. A glyph or a line-art icon is drawn in whatever ink
// the caller decides at the moment it is painted — a button's foreground, a
// theme's accent, the inverse under a dark menu bar — so rasterising it to
// pixels of a fixed colour throws away the one thing the caller still needs to
// choose. A mask keeps the shape and leaves the colour to them.
//
// Without this, every caller wanting a mask grew its own SVG parser and
// rasteriser next to this one; there were three in this fleet.
func RasterizeMask(doc string, opt Options) (*image.Alpha, error) {
	// Coverage is what the renderer already computes before it composites,
	// and compositing opaque ink onto nothing preserves it exactly: the
	// alpha of the result IS the coverage. Reusing the one renderer keeps a
	// mask and a picture of the same document in agreement — two code paths
	// would drift, and the drift would be sub-pixel and invisible.
	opt.Ink = color.RGBA{A: 255}
	opt.Paper = color.RGBA{}
	res, err := Rasterize(doc, opt)
	if err != nil {
		return nil, err
	}
	img := res.Image
	m := image.NewAlpha(image.Rect(0, 0, img.W, img.H))
	for y := 0; y < img.H; y++ {
		src := y * img.W * 4
		dst := y * m.Stride
		for x := 0; x < img.W; x++ {
			m.Pix[dst+x] = img.Pix[src+x*4+3]
		}
	}
	return m, nil
}
