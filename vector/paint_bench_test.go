// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package vector

import (
	"image/color"
	"testing"

	"github.com/go-gfx/gfx/raster"
)

// These measure the colour-bearing companion stage the rasterizer now offers:
// pushing a Paint (solid or gradient) through a coverage grid onto a pixel
// buffer. They complete the pipeline the Fill benchmarks start (coverage) with
// the composite the consumer used to hand-roll. Run:
//
//	go test ./vector/ -bench Composite -benchmem -run '^$'

func benchFillGrid(b *testing.B) (cov []float64, ox, oy, w, h int) {
	b.Helper()
	var rz Rasterizer
	cov, ox, oy, w, h, ok := rz.Fill(benchCircle(256, 256, 240), NonZero, benchSurf, benchSurf)
	if !ok {
		b.Fatal("fill produced no coverage")
	}
	// Detach from the rasterizer's reused scratch so the benchmark loop is stable.
	return append([]float64(nil), cov...), ox, oy, w, h
}

func BenchmarkCompositeSolid(b *testing.B) {
	cov, ox, oy, w, h := benchFillGrid(b)
	dst := raster.New(benchSurf, benchSurf)
	p := SolidPaint{color.RGBA{200, 40, 60, 255}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Composite(dst, cov, ox, oy, w, h, p)
	}
}

func BenchmarkCompositeLinearGradient(b *testing.B) {
	cov, ox, oy, w, h := benchFillGrid(b)
	dst := raster.New(benchSurf, benchSurf)
	p := NewLinearGradient(16, 16, 496, 496, Pad,
		Stop{0, color.RGBA{0, 0, 0, 255}}, Stop{1, color.RGBA{255, 255, 255, 255}})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Composite(dst, cov, ox, oy, w, h, p)
	}
}

func BenchmarkCompositeRadialGradient(b *testing.B) {
	cov, ox, oy, w, h := benchFillGrid(b)
	dst := raster.New(benchSurf, benchSurf)
	p := NewRadialGradient(256, 256, 240, Pad,
		Stop{0, color.RGBA{255, 0, 0, 255}}, Stop{1, color.RGBA{0, 0, 255, 255}})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Composite(dst, cov, ox, oy, w, h, p)
	}
}
