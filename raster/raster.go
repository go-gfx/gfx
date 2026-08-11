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

// FromImage converts any image.Image into a straight-alpha Image. An
// *image.NRGBA (already straight) is copied directly; anything else is
// converted pixel by pixel through the non-premultiplied colour model, so a
// premultiplied *image.RGBA source is un-premultiplied rather than mis-read.
func FromImage(src image.Image) *Image {
	b := src.Bounds()
	dst := New(b.Dx(), b.Dy())
	if nr, ok := src.(*image.NRGBA); ok {
		for y := 0; y < dst.H; y++ {
			so := nr.PixOffset(b.Min.X, b.Min.Y+y)
			do := y * dst.W * 4
			copy(dst.Pix[do:do+dst.W*4], nr.Pix[so:so+dst.W*4])
		}
		return dst
	}
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = c.R, c.G, c.B, c.A
			i += 4
		}
	}
	return dst
}

// ToNRGBA returns the buffer as a standard-library *image.NRGBA, whose bytes are
// also straight, so this is a direct copy and the round trip through the
// standard image package is lossless.
func (p *Image) ToNRGBA() *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, p.W, p.H))
	copy(dst.Pix, p.Pix)
	return dst
}
