// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"

	"github.com/go-gfx/gfx/raster"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

// ErrCannotEncode is returned for a format this can read but not write.
//
// Reading and writing are not symmetric and the gap is not an oversight. A
// reference decoder exists in pure Go for every format [Sniff] names; a
// reference ENCODER does not, and this package writes none of its own. Saying
// which way a format goes is part of its contract.
var ErrCannotEncode = fmt.Errorf("codec: no reference encoder for this format")

// Encode writes an image in the named format.
//
// The formats that can be written are PNG, JPEG, GIF, TIFF and BMP, each
// through the same reference library that reads it. WEBP, ICO, ICNS, PNM, QOI,
// JP2 and JBIG2 can be read here and not written: they return
// [ErrCannotEncode] rather than something in another format under the asked-for
// name.
//
// Alpha survives into PNG and TIFF, which carry it. JPEG, GIF and BMP do not,
// and what they are given is the image composited onto white — chosen rather
// than left to the encoder, because an encoder that drops the alpha channel
// puts the colour that was UNDER the transparency on the page, and for a page
// drawn on transparent ground that is black.
func Encode(w io.Writer, img *raster.Image, f Format) error {
	if img == nil {
		return fmt.Errorf("codec: nothing to encode")
	}
	switch f {
	case PNG:
		return png.Encode(w, nrgba(img))
	case TIFF:
		return tiff.Encode(w, nrgba(img), nil)
	case JPEG:
		return jpeg.Encode(w, onWhite(img), nil)
	case GIF:
		return gif.Encode(w, onWhite(img), nil)
	case BMP:
		return bmp.Encode(w, onWhite(img))
	default:
		return fmt.Errorf("%w: %s", ErrCannotEncode, f)
	}
}

// CanEncode says whether [Encode] can write a format, so a caller can offer
// what is possible rather than find out by failing.
func CanEncode(f Format) bool {
	switch f {
	case PNG, JPEG, GIF, TIFF, BMP:
		return true
	default:
		return false
	}
}

// nrgba presents the image to an encoder that carries alpha. The pixels are
// already straight alpha in the same order, so this is a header and not a copy
// of the pixels.
func nrgba(img *raster.Image) *image.NRGBA {
	return &image.NRGBA{
		Pix:    img.Pix,
		Stride: img.W * 4,
		Rect:   image.Rect(0, 0, img.W, img.H),
	}
}

// onWhite composites the image onto white for the encoders that carry no
// alpha.
func onWhite(img *raster.Image) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, img.W, img.H))
	for i := 0; i < img.W*img.H; i++ {
		a := uint32(img.Pix[i*4+3])
		for c := 0; c < 3; c++ {
			v := uint32(img.Pix[i*4+c])*a + 255*(255-a)
			out.Pix[i*4+c] = uint8((v + 127) / 255)
		}
		out.Pix[i*4+3] = 255
	}
	return out
}
