// Package raster is the shared pixel-buffer substrate of go-gfx: a densely
// packed 8-bit RGBA image the higher layers (resample, color, and the fleet's
// image-processing and rendering libraries) read and write.
//
// Image.Pix is row-major, four bytes (R, G, B, A) per pixel, with NO row
// padding — stride is always 4*W. The bytes are STRAIGHT (non-premultiplied):
// a pixel's colour is stored as-is, independent of its alpha, which is the
// representation image processing wants. This differs from the standard
// library's image.RGBA, whose bytes are premultiplied; FromImage and ToRGBA
// convert across that boundary correctly rather than copying bytes blindly.
package raster

import (
	"image"
	"image/color"
)

// Image is a densely packed, straight-alpha 8-bit RGBA pixel buffer of W by H
// pixels. Pix has length 4*W*H.
type Image struct {
	Pix  []uint8
	W, H int
}

// New returns a zeroed Image of w by h pixels (fully transparent black). w and h
// must be non-negative.
func New(w, h int) *Image {
	return &Image{Pix: make([]uint8, 4*w*h), W: w, H: h}
}

// Bounds returns the image's rectangle, anchored at the origin.
func (p *Image) Bounds() image.Rectangle { return image.Rect(0, 0, p.W, p.H) }

// PixOffset returns the index into Pix of the pixel at (x, y).
func (p *Image) PixOffset(x, y int) int { return (y*p.W + x) * 4 }

// At returns the straight-alpha colour of the pixel at (x, y).
func (p *Image) At(x, y int) color.RGBA {
	i := p.PixOffset(x, y)
	return color.RGBA{p.Pix[i], p.Pix[i+1], p.Pix[i+2], p.Pix[i+3]}
}

// Set writes the straight-alpha colour c to the pixel at (x, y).
func (p *Image) Set(x, y int, c color.RGBA) {
	i := p.PixOffset(x, y)
	p.Pix[i], p.Pix[i+1], p.Pix[i+2], p.Pix[i+3] = c.R, c.G, c.B, c.A
}

// FromImage converts any image.Image into a straight-alpha Image.
//
// The standard library's concrete image types are read through their own
// pixel buffers. The general path — At, then the non-premultiplied colour
// model — is correct but costs an interface value and a virtual call for
// every pixel: a 1200x1600 photograph, which is one page of one scanned
// document, went through it in 66 ms and 3.8 million heap allocations, two
// per pixel. A JPEG in a PDF decodes to *image.YCbCr, so that was the price
// of every picture a renderer drew.
//
// Each fast path below reproduces what the general path computes, byte for
// byte, and TestFromImageFastPathsMatchGeneral proves it over the whole input
// space of each. An image type this does not recognise still goes through the
// general path unchanged: a third-party image whose At returns a colour the
// model treats specially must keep getting the answer it gets today.
func FromImage(src image.Image) *Image {
	b := src.Bounds()
	dst := New(b.Dx(), b.Dy())
	switch s := src.(type) {
	case *image.NRGBA:
		// Already straight-alpha and densely packed the same way: a copy.
		for y := 0; y < dst.H; y++ {
			so := s.PixOffset(b.Min.X, b.Min.Y+y)
			do := y * dst.W * 4
			copy(dst.Pix[do:do+dst.W*4], s.Pix[so:so+dst.W*4])
		}
	case *image.RGBA:
		fromRGBA(dst, s, b)
	case *image.YCbCr:
		fromYCbCr(dst, s, b)
	case *image.Gray:
		fromGray(dst, s, b)
	case *image.CMYK:
		fromCMYK(dst, s, b)
	case *image.Paletted:
		fromPaletted(dst, s, b)
	default:
		i := 0
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
				dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = c.R, c.G, c.B, c.A
				i += 4
			}
		}
	}
	return dst
}

// unpremul divides one premultiplied 8-bit channel by its alpha, giving the
// straight value the colour model gives.
//
// The model works in sixteen bits: it widens both v and a by v|v<<8, which is
// v*257, divides (v*257*65535) by (a*257) and takes the top byte. The 257
// cancels exactly — it is a factor of both — so v*65535/a in 32 bits is the
// same integer, without the widening.
func unpremul(v, a uint8) uint8 {
	return uint8((uint32(v) * 0xffff / uint32(a)) >> 8)
}

// fromRGBA un-premultiplies an *image.RGBA. The two extremes are the model's
// own special cases, not shortcuts: at full alpha it returns the bytes
// untouched, and at zero alpha it returns transparent black whatever colour
// the source held there.
func fromRGBA(dst *Image, s *image.RGBA, b image.Rectangle) {
	i := 0
	for y := 0; y < dst.H; y++ {
		row := s.Pix[s.PixOffset(b.Min.X, b.Min.Y+y):]
		for x := 0; x < dst.W; x++ {
			r, g, bl, a := row[x*4], row[x*4+1], row[x*4+2], row[x*4+3]
			switch a {
			case 0xff:
				dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = r, g, bl, 0xff
			case 0:
				dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = 0, 0, 0, 0
			default:
				dst.Pix[i] = unpremul(r, a)
				dst.Pix[i+1] = unpremul(g, a)
				dst.Pix[i+2] = unpremul(bl, a)
				dst.Pix[i+3] = a
			}
			i += 4
		}
	}
}

// fromYCbCr converts the planar form a JPEG decodes to. It is opaque, so
// there is no alpha to undo; the chroma planes may be shared between pixels,
// which is what COffset accounts for.
func fromYCbCr(dst *Image, s *image.YCbCr, b image.Rectangle) {
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			co := s.COffset(x, y)
			r, g, bl := color.YCbCrToRGB(s.Y[s.YOffset(x, y)], s.Cb[co], s.Cr[co])
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = r, g, bl, 0xff
			i += 4
		}
	}
}

// fromGray spreads one channel across three. A grey image is opaque.
func fromGray(dst *Image, s *image.Gray, b image.Rectangle) {
	i := 0
	for y := 0; y < dst.H; y++ {
		row := s.Pix[s.PixOffset(b.Min.X, b.Min.Y+y):]
		for x := 0; x < dst.W; x++ {
			v := row[x]
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = v, v, v, 0xff
			i += 4
		}
	}
}

// fromCMYK converts the four-ink form a print-bound JPEG decodes to, which is
// also opaque.
func fromCMYK(dst *Image, s *image.CMYK, b image.Rectangle) {
	i := 0
	for y := 0; y < dst.H; y++ {
		row := s.Pix[s.PixOffset(b.Min.X, b.Min.Y+y):]
		for x := 0; x < dst.W; x++ {
			r, g, bl := color.CMYKToRGB(row[x*4], row[x*4+1], row[x*4+2], row[x*4+3])
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = r, g, bl, 0xff
			i += 4
		}
	}
}

// fromPaletted converts the palette once — 256 entries at most, against one
// per pixel — and then indexes it.
//
// A palette shorter than the indices used is a broken image, and so is a
// Paletted with no palette at all: image.Paletted.At returns a nil colour for
// the second, which the colour model then dereferences, so asking this to
// convert an empty-palette image used to bring the process down. A pixel this
// cannot name is transparent black, which is what an image with no colours in
// it should look like.
func fromPaletted(dst *Image, s *image.Paletted, b image.Rectangle) {
	var table [256]color.NRGBA
	for j, c := range s.Palette {
		if j >= len(table) {
			break
		}
		table[j] = color.NRGBAModel.Convert(c).(color.NRGBA)
	}
	n := len(s.Palette)
	if n > len(table) {
		n = len(table)
	}
	i := 0
	for y := 0; y < dst.H; y++ {
		row := s.Pix[s.PixOffset(b.Min.X, b.Min.Y+y):]
		for x := 0; x < dst.W; x++ {
			var c color.NRGBA
			if idx := int(row[x]); idx < n {
				c = table[idx]
			}
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = c.R, c.G, c.B, c.A
			i += 4
		}
	}
}

// ToNRGBA returns the buffer as a standard-library *image.NRGBA, whose bytes are
// also straight, so this is a direct copy and the round trip through the
// standard image package is lossless.
func (p *Image) ToNRGBA() *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, p.W, p.H))
	copy(dst.Pix, p.Pix)
	return dst
}
