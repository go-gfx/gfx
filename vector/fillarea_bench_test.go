package vector_test

import (
	"fmt"
	"testing"

	"github.com/go-gfx/gfx/vector"
)

// BenchmarkFillLargeArea is the shape a clip path takes: a rectangle covering
// most of the surface. A page renderer intersects one of those with the clip
// for every W operator, and a drawing with a few thousand of them spends its
// afternoon here.
func BenchmarkFillLargeArea(b *testing.B) {
	for _, size := range []int{256, 1024, 2048} {
		b.Run(fmt.Sprintf("%dx%d", size, size), func(b *testing.B) {
			p := vector.NewPath()
			p.MoveTo(1, 1)
			p.LineTo(float64(size-1), 1)
			p.LineTo(float64(size-1), float64(size-1))
			p.LineTo(1, float64(size-1))
			p.Close()
			var rz vector.Rasterizer
			b.ReportAllocs()
			for b.Loop() {
				rz.Fill(p, vector.NonZero, size, size)
			}
		})
	}
}
