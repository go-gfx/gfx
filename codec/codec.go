// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

// Package codec is go-gfx's unified image-decoding registry: one Decode entry
// point that sniffs a byte slice's container format and dispatches to the
// appropriate REFERENCE decoder, returning the shared [raster.Image] substrate
// the rest of go-gfx consumes.
//
// It reimplements NO decoder. Every format is handed to a battle-tested pure-Go
// (CGO-free) reference library, and this package only sniffs the format,
// delegates, and converts the reference's image.Image into a straight-alpha
// [raster.Image]:
//
//	format  reference decoder                          module
//	------  ----------------------------------------   ------------------------------------
//	PNG     image/png                                  standard library
//	JPEG    image/jpeg                                 standard library
//	GIF     image/gif                                  standard library
//	WEBP    golang.org/x/image/webp                    golang.org/x/image
//	TIFF    golang.org/x/image/tiff                    golang.org/x/image
//	BMP     golang.org/x/image/bmp                     golang.org/x/image
//	ICO     github.com/sergeymakinen/go-ico            github.com/sergeymakinen/go-ico
//	ICNS    image/png (per embedded PNG representation)  standard library  [see icns.go]
//
// ICNS is the single format with no clean pure-Go decode reference (see the
// verdict in icns.go): its container is demuxed here by a bounds-checked TLV
// walk and each embedded PNG representation is decoded by the standard library —
// so no image decoder is reimplemented, only the container is unpacked, exactly
// as go-ico unpacks the .ico container around the standard BMP/PNG decoders.
//
// [Decode] returns the primary (largest, most detailed) image of a container.
// [DecodeBest] additionally lets a caller target a pixel size for the
// multi-representation formats (ICO, ICNS), picking the smallest representation
// at least as large as the target, or the largest available when none reaches
// it. All decoders are pure-Go; the package never shells out and never needs
// CGO.
package codec

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"

	"github.com/go-gfx/gfx/raster"
	ico "github.com/sergeymakinen/go-ico"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
	"golang.org/x/image/webp"
)

// Format identifies a decodable image-container format recognised by [Sniff].
type Format int

// The container formats codec can decode. Unknown is the zero value returned
// when a byte slice matches no known signature.
const (
	Unknown Format = iota
	PNG
	JPEG
	GIF
	WEBP
	TIFF
	BMP
	ICO
	ICNS
)

// String returns the format's short uppercase name (e.g. "PNG"), or "unknown".
func (f Format) String() string {
	switch f {
	case PNG:
		return "PNG"
	case JPEG:
		return "JPEG"
	case GIF:
		return "GIF"
	case WEBP:
		return "WEBP"
	case TIFF:
		return "TIFF"
	case BMP:
		return "BMP"
	case ICO:
		return "ICO"
	case ICNS:
		return "ICNS"
	default:
		return "unknown"
	}
}

// ErrUnknownFormat is returned by [Decode] and [DecodeBest] when the input's
// leading bytes match no format the registry knows how to decode.
var ErrUnknownFormat = fmt.Errorf("codec: unrecognised image format")

// Sniff identifies the container format of data from its magic bytes, without
// decoding it. It returns [Unknown] when the input is too short or matches no
// known signature.
func Sniff(data []byte) Format {
	switch {
	case len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n":
		return PNG
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return JPEG
	case len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a"):
		return GIF
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return WEBP
	case len(data) >= 4 && (string(data[:4]) == "II*\x00" || string(data[:4]) == "MM\x00*"):
		return TIFF
	case len(data) >= 2 && data[0] == 'B' && data[1] == 'M':
		return BMP
	case len(data) >= 4 && string(data[:4]) == "\x00\x00\x01\x00":
		return ICO
	case len(data) >= 4 && string(data[:4]) == "icns":
		return ICNS
	default:
		return Unknown
	}
}

// Decode sniffs data's format, delegates to the matching reference decoder, and
// returns the image as a straight-alpha [raster.Image]. For multi-representation
// containers (ICO, ICNS) it returns the largest, most detailed representation;
// use [DecodeBest] to target a size. It returns [ErrUnknownFormat] for an
// unrecognised input and the reference decoder's own error for a corrupt one.
func Decode(data []byte) (*raster.Image, error) {
	return DecodeBest(data, 0)
}

// DecodeBest is [Decode] with a target pixel size for the multi-representation
// formats. For ICO and ICNS it selects the representation whose larger side is
// the smallest that is still >= targetSize, falling back to the largest
// representation when none reaches the target; a targetSize <= 0 always selects
// the largest. targetSize is ignored for single-image formats, whose sole image
// is returned regardless.
func DecodeBest(data []byte, targetSize int) (*raster.Image, error) {
	switch Sniff(data) {
	case PNG:
		return decodeSingle(png.Decode, data)
	case JPEG:
		return decodeSingle(jpeg.Decode, data)
	case GIF:
		return decodeSingle(gif.Decode, data)
	case WEBP:
		return decodeSingle(webp.Decode, data)
	case TIFF:
		return decodeSingle(tiff.Decode, data)
	case BMP:
		return decodeSingle(bmp.Decode, data)
	case ICO:
		return decodeICO(data, targetSize)
	case ICNS:
		return decodeICNS(data, targetSize)
	default:
		return nil, ErrUnknownFormat
	}
}

// decodeSingle runs a standard image.Image decoder over data and converts the
// result to a straight-alpha [raster.Image].
func decodeSingle(dec func(io.Reader) (image.Image, error), data []byte) (*raster.Image, error) {
	img, err := dec(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return raster.FromImage(img), nil
}

// decodeICO decodes a Windows .ico container via the reference codec
// github.com/sergeymakinen/go-ico and returns the representation best matching
// targetSize. DecodeAll yields every stored icon; the choice is made by
// pickIndex over their pixel sizes.
func decodeICO(data []byte, targetSize int) (*raster.Image, error) {
	imgs, err := ico.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	sizes := make([]dim, len(imgs))
	for i, m := range imgs {
		b := m.Bounds()
		sizes[i] = dim{b.Dx(), b.Dy()}
	}
	return raster.FromImage(imgs[pickIndex(sizes, targetSize)]), nil
}

// dim is a representation's pixel width and height, used for size selection.
type dim struct{ w, h int }

// long returns the larger of a dimension's two sides.
func (d dim) long() int {
	if d.h > d.w {
		return d.h
	}
	return d.w
}

// pickIndex chooses, among sizes, the index of the representation that best
// matches target. A target <= 0 selects the one with the largest longer side.
// Otherwise it selects the smallest longer side that is still >= target, and
// falls back to the largest when no representation reaches the target. sizes
// must be non-empty (the decoders error out on an empty container).
func pickIndex(sizes []dim, target int) int {
	largest, largestLong := 0, sizes[0].long()
	for i, s := range sizes {
		if s.long() > largestLong {
			largest, largestLong = i, s.long()
		}
	}
	if target <= 0 {
		return largest
	}
	adequate, adequateLong := -1, 0
	for i, s := range sizes {
		if l := s.long(); l >= target && (adequate < 0 || l < adequateLong) {
			adequate, adequateLong = i, l
		}
	}
	if adequate >= 0 {
		return adequate
	}
	return largest
}
