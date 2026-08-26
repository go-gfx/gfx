package raster_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/go-gfx/gfx/raster"
)

// general is what FromImage did before there were fast paths, kept here as the
// thing they have to agree with. A conversion that is merely fast is not the
// point: it has to give the same picture, and the only way to say that is to
// compute the answer both ways and compare the bytes.
func general(src image.Image) []uint8 {
	b := src.Bounds()
	out := make([]uint8, 4*b.Dx()*b.Dy())
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
			out[i], out[i+1], out[i+2], out[i+3] = c.R, c.G, c.B, c.A
			i += 4
		}
	}
	return out
}

// opaque wraps an image so that FromImage cannot recognise its concrete type
// and has to take the general path. The picture is the same one; only the way
// it is read differs.
type opaque struct{ image.Image }

func compare(t *testing.T, name string, src image.Image) {
	t.Helper()
	fast := raster.FromImage(src)
	slow := general(src)
	if len(fast.Pix) != len(slow) {
		t.Fatalf("%s: fast path produced %d bytes, general path %d", name, len(fast.Pix), len(slow))
	}
	for i := range slow {
		if fast.Pix[i] != slow[i] {
			t.Fatalf("%s: byte %d (pixel %d, channel %d) is %d on the fast path and %d on the general one",
				name, i, i/4, i%4, fast.Pix[i], slow[i])
		}
	}
	// The general path must also still be reachable and still agree, which is
	// what the wrapper checks.
	if wrapped := raster.FromImage(opaque{src}); !equalBytes(wrapped.Pix, slow) {
		t.Fatalf("%s: the general path disagrees with itself", name)
	}
}

func equalBytes(a, b []uint8) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestFromImageFastPathsMatchGeneral walks the whole input space of each fast
// path it can, and a wide sample of the ones it cannot.
func TestFromImageFastPathsMatchGeneral(t *testing.T) {
	// An *image.RGBA holds premultiplied bytes, so every (value, alpha) pair
	// that can legally occur is every pair where value <= alpha: 32 896 of
	// them, all of them checked.
	rgba := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for a := 0; a < 256; a++ {
		for v := 0; v < 256; v++ {
			p := rgba.PixOffset(v, a)
			w := uint8(v)
			if v > a {
				w = uint8(a)
			}
			rgba.Pix[p], rgba.Pix[p+1], rgba.Pix[p+2], rgba.Pix[p+3] = w, w/2, w/3, uint8(a)
		}
	}
	compare(t, "RGBA", rgba)

	// An image the standard library never builds, but a caller can: bytes
	// above their own alpha. The two paths still have to agree.
	broken := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for a := 0; a < 256; a++ {
		for v := 0; v < 256; v++ {
			p := broken.PixOffset(v, a)
			broken.Pix[p], broken.Pix[p+1], broken.Pix[p+2], broken.Pix[p+3] = uint8(v), uint8(255-v), uint8(v/2), uint8(a)
		}
	}
	compare(t, "RGBA out of range", broken)

	// Every one of the 256 grey levels.
	gray := image.NewGray(image.Rect(0, 0, 16, 16))
	for i := range gray.Pix {
		gray.Pix[i] = uint8(i)
	}
	compare(t, "Gray", gray)

	// Every luma against a spread of chroma, in each subsampling the decoder
	// produces, since the subsampling is what decides which pixels share a
	// chroma sample.
	for _, ratio := range []image.YCbCrSubsampleRatio{
		image.YCbCrSubsampleRatio444, image.YCbCrSubsampleRatio422,
		image.YCbCrSubsampleRatio420, image.YCbCrSubsampleRatio440,
		image.YCbCrSubsampleRatio411, image.YCbCrSubsampleRatio410,
	} {
		y := image.NewYCbCr(image.Rect(0, 0, 61, 37), ratio)
		for i := range y.Y {
			y.Y[i] = uint8(i * 7)
		}
		for i := range y.Cb {
			y.Cb[i] = uint8(i * 11)
			y.Cr[i] = uint8(255 - i*13)
		}
		compare(t, "YCbCr "+ratio.String(), y)
	}

	// A four-ink image, sampled across all four channels.
	cmyk := image.NewCMYK(image.Rect(0, 0, 64, 64))
	for i := range cmyk.Pix {
		cmyk.Pix[i] = uint8(i * 3)
	}
	compare(t, "CMYK", cmyk)

	// A palette with translucent and transparent entries, since those are the
	// ones the colour model treats specially.
	pal := image.NewPaletted(image.Rect(0, 0, 32, 32), color.Palette{
		color.RGBA{255, 0, 0, 255}, color.NRGBA{10, 20, 30, 128},
		color.RGBA{4, 4, 4, 8}, color.RGBA{0, 0, 0, 0},
		color.Gray{200}, color.CMYK{1, 2, 3, 4},
	})
	for i := range pal.Pix {
		pal.Pix[i] = uint8(i % 6)
	}
	compare(t, "Paletted", pal)

	// A straight-alpha image, which is the copy path.
	nrgba := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for i := range nrgba.Pix {
		nrgba.Pix[i] = uint8(i)
	}
	compare(t, "NRGBA", nrgba)
}

// TestFromImageSubImage checks the fast paths honour an origin that is not
// zero: a sub-image shares the parent's pixel buffer, and reading it from the
// wrong offset would copy the wrong part of the picture without failing.
func TestFromImageSubImage(t *testing.T) {
	r := image.Rect(0, 0, 20, 20)
	sub := image.Rect(5, 7, 15, 19)

	rgba := image.NewRGBA(r)
	for i := range rgba.Pix {
		rgba.Pix[i] = uint8(i)
	}
	for i := 3; i < len(rgba.Pix); i += 4 {
		rgba.Pix[i] = 0xff
	}
	compare(t, "RGBA sub", rgba.SubImage(sub))

	gray := image.NewGray(r)
	for i := range gray.Pix {
		gray.Pix[i] = uint8(i)
	}
	compare(t, "Gray sub", gray.SubImage(sub))

	cmyk := image.NewCMYK(r)
	for i := range cmyk.Pix {
		cmyk.Pix[i] = uint8(i)
	}
	compare(t, "CMYK sub", cmyk.SubImage(sub))

	nrgba := image.NewNRGBA(r)
	for i := range nrgba.Pix {
		nrgba.Pix[i] = uint8(i)
	}
	compare(t, "NRGBA sub", nrgba.SubImage(sub))

	pal := image.NewPaletted(r, color.Palette{color.RGBA{1, 2, 3, 255}, color.RGBA{9, 8, 7, 200}})
	for i := range pal.Pix {
		pal.Pix[i] = uint8(i % 2)
	}
	compare(t, "Paletted sub", pal.SubImage(sub))

	y := image.NewYCbCr(r, image.YCbCrSubsampleRatio420)
	for i := range y.Y {
		y.Y[i] = uint8(i * 3)
	}
	for i := range y.Cb {
		y.Cb[i] = uint8(i * 5)
		y.Cr[i] = uint8(i * 7)
	}
	compare(t, "YCbCr sub", y.SubImage(sub))
}

// TestFromImageEmptyPalette is the regression: image.Paletted.At returns a nil
// colour when the image has no palette, and the colour model dereferenced it.
// A two-by-two picture brought the process down.
func TestFromImageEmptyPalette(t *testing.T) {
	p := image.NewPaletted(image.Rect(0, 0, 2, 2), nil)
	img := raster.FromImage(p)
	if len(img.Pix) != 16 {
		t.Fatalf("want 16 bytes, got %d", len(img.Pix))
	}
	for i, v := range img.Pix {
		if v != 0 {
			t.Fatalf("byte %d is %d, want transparent black throughout", i, v)
		}
	}
}

// TestFromImagePaletteIndexOutOfRange covers the other half of the same
// defect: indices the palette is too short to have. Reading them through
// image.Paletted.At is an index out of range from inside the standard
// library, so the general path cannot be the oracle here; the answer this
// gives is the same as for an image with no palette at all.
func TestFromImagePaletteIndexOutOfRange(t *testing.T) {
	p := image.NewPaletted(image.Rect(0, 0, 2, 1), color.Palette{color.RGBA{1, 2, 3, 255}})
	p.Pix[0] = 0
	p.Pix[1] = 200
	img := raster.FromImage(p)
	if got := img.Pix[:4]; got[0] != 1 || got[1] != 2 || got[2] != 3 || got[3] != 255 {
		t.Fatalf("indexed pixel is %v, want the palette's only colour", got)
	}
	if got := img.Pix[4:8]; got[0] != 0 || got[1] != 0 || got[2] != 0 || got[3] != 0 {
		t.Fatalf("out-of-range index gave %v, want transparent black", got)
	}
}

// TestFromImageOversizePalette covers a palette with more than 256 colours,
// which a byte index cannot reach past: the table is filled and the rest
// ignored, rather than overrunning it.
func TestFromImageOversizePalette(t *testing.T) {
	var pal color.Palette
	for i := 0; i < 300; i++ {
		pal = append(pal, color.RGBA{uint8(i), uint8(i / 2), 7, 255})
	}
	p := image.NewPaletted(image.Rect(0, 0, 2, 1), pal)
	p.Pix[0] = 0
	p.Pix[1] = 255
	img := raster.FromImage(p)
	if img.Pix[0] != 0 || img.Pix[4] != 255 {
		t.Fatalf("oversize palette gave %v", img.Pix[:8])
	}
}
