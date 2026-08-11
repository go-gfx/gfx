package resample

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/go-gfx/gfx/raster"
)

// benchImage is a deterministic opaque RGBA source of the given size.
func benchImage(side int) *raster.Image {
	rng := rand.New(rand.NewSource(int64(side)))
	im := raster.New(side, side)
	for i := 0; i < len(im.Pix); i += 4 {
		im.Pix[i] = uint8(rng.Intn(256))
		im.Pix[i+1] = uint8(rng.Intn(256))
		im.Pix[i+2] = uint8(rng.Intn(256))
		im.Pix[i+3] = 255
	}
	return im
}

// BenchmarkResize times each mode on a representative thumbnail reduction
// (1024x1024 -> 256x256) and enlargement (256x256 -> 1024x1024). Mpix/s is over
// the source pixels processed, matching the Pillow harness in docs/perf.md.
func BenchmarkResize(b *testing.B) {
	for _, tc := range []struct {
		name        string
		src, dw, dh int
	}{
		{"down_1024_to_256", 1024, 256, 256},
		{"up_256_to_1024", 256, 1024, 1024},
	} {
		src := benchImage(tc.src)
		for _, m := range []struct {
			name string
			mode Mode
		}{{"Box", Box}, {"Bicubic", Bicubic}, {"Lanczos", Lanczos}} {
			b.Run(fmt.Sprintf("%s/%s", tc.name, m.name), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := Resize(src, tc.dw, tc.dh, m.mode); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(tc.src*tc.src)*float64(b.N)/1e6/b.Elapsed().Seconds(), "Mpix/s")
			})
		}
	}
}
