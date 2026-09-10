package raster_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/go-gfx/gfx/raster"
)

// The sizes are a page-sized scan, which is the case that matters: a PDF that
// places one photograph is one call to FromImage over several million pixels.
const benchW, benchH = 1200, 1600

func benchSources() map[string]image.Image {
	r := image.Rect(0, 0, benchW, benchH)
	ycbcr := image.NewYCbCr(r, image.YCbCrSubsampleRatio420)
	for i := range ycbcr.Y {
		ycbcr.Y[i] = uint8(i)
	}
	for i := range ycbcr.Cb {
		ycbcr.Cb[i] = uint8(i * 3)
		ycbcr.Cr[i] = uint8(i * 5)
	}
	rgba := image.NewRGBA(r)
	for i := range rgba.Pix {
		rgba.Pix[i] = uint8(i)
	}
	for i := 3; i < len(rgba.Pix); i += 4 {
		// Premultiplied means no channel may exceed alpha.
		if rgba.Pix[i] < rgba.Pix[i-1] {
			rgba.Pix[i] = 255
		}
	}
	gray := image.NewGray(r)
	for i := range gray.Pix {
		gray.Pix[i] = uint8(i)
	}
	cmyk := image.NewCMYK(r)
	for i := range cmyk.Pix {
		cmyk.Pix[i] = uint8(i)
	}
	pal := image.NewPaletted(r, color.Palette{
		color.RGBA{1, 2, 3, 255}, color.RGBA{4, 5, 6, 128}, color.RGBA{7, 8, 9, 0},
	})
	for i := range pal.Pix {
		pal.Pix[i] = uint8(i % 3)
	}
	nrgba := image.NewNRGBA(r)
	for i := range nrgba.Pix {
		nrgba.Pix[i] = uint8(i)
	}
	return map[string]image.Image{
		"YCbCr": ycbcr, "RGBA": rgba, "Gray": gray,
		"CMYK": cmyk, "Paletted": pal, "NRGBA": nrgba,
	}
}

func BenchmarkFromImage(b *testing.B) {
	for name, src := range benchSources() {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				raster.FromImage(src)
			}
		})
	}
}
