// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package vector

import (
	"math"
	"testing"
)

// These mirror go-widgets/painter's FillPath / StrokePath benchmarks on the
// realistic 512x512 icon/scene surface, but measure only the coverage stage this
// package owns (composite stays with the consumer). Because the algorithm was
// moved verbatim, the coverage-stage cost — and its steady-state zero allocation
// from the reused scratch — is unchanged from the pre-extraction painter. Run:
//
//	go test ./vector/ -bench . -benchmem -run '^$'

func benchCircle(cx, cy, r float64) *Path {
	const k = 0.5522847498307936
	o := r * k
	return NewPath().
		MoveTo(cx+r, cy).
		CubicTo(cx+r, cy+o, cx+o, cy+r, cx, cy+r).
		CubicTo(cx-o, cy+r, cx-r, cy+o, cx-r, cy).
		CubicTo(cx-r, cy-o, cx-o, cy-r, cx, cy-r).
		CubicTo(cx+o, cy-r, cx+r, cy-o, cx+r, cy).
		Close()
}

func benchPolygon(cx, cy, r float64, n int) *Path {
	p := NewPath()
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		x, y := cx+r*math.Cos(a), cy+r*math.Sin(a)
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	return p.Close()
}

func benchStar(cx, cy, rOuter, rInner float64, points int) *Path {
	p := NewPath()
	for i := 0; i < points*2; i++ {
		r := rOuter
		if i%2 == 1 {
			r = rInner
		}
		a := math.Pi * float64(i) / float64(points)
		x, y := cx+r*math.Sin(a), cy-r*math.Cos(a)
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	return p.Close()
}

func benchPolyline(n int, w, h float64) *Path {
	p := NewPath()
	for i := 0; i < n; i++ {
		x := w * float64(i) / float64(n-1)
		y := h / 2
		if i%2 == 1 {
			y = h / 4
		}
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	return p
}

const benchSurf = 512

func BenchmarkFillCircleNonZero(b *testing.B) {
	var rz Rasterizer
	pth := benchCircle(256, 256, 240)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rz.Fill(pth, NonZero, benchSurf, benchSurf)
	}
}

func BenchmarkFillPolygonNonZero(b *testing.B) {
	var rz Rasterizer
	pth := benchPolygon(256, 256, 240, 256)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rz.Fill(pth, NonZero, benchSurf, benchSurf)
	}
}

func BenchmarkFillStarNonZero(b *testing.B) {
	var rz Rasterizer
	pth := benchStar(256, 256, 240, 96, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rz.Fill(pth, NonZero, benchSurf, benchSurf)
	}
}

func BenchmarkFillStarEvenOdd(b *testing.B) {
	var rz Rasterizer
	pth := benchStar(256, 256, 240, 96, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rz.Fill(pth, EvenOdd, benchSurf, benchSurf)
	}
}

func BenchmarkStrokePolyline(b *testing.B) {
	var rz Rasterizer
	pth := benchPolyline(64, 512, 512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rz.Stroke(pth, 6, benchSurf, benchSurf)
	}
}

func BenchmarkStrokeCircle(b *testing.B) {
	var rz Rasterizer
	pth := benchCircle(256, 256, 200)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rz.Stroke(pth, 4, benchSurf, benchSurf)
	}
}
