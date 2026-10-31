// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

// Package codec is go-gfx's unified image-decoding registry: one Decode entry
// point that sniffs a byte slice's container format and dispatches to the
// appropriate REFERENCE decoder, returning the shared [raster.Image] substrate
// the rest of go-gfx consumes.
//
// It writes as well as reads, through [Encode], and the two are not
// symmetric: a reference DECODER exists in pure Go for every format below, a
// reference encoder for five of them. [CanEncode] says which.
//
// Most formats are handed to a pure-Go (CGO-free) reference library, and this
// package only sniffs the format, delegates, and converts the reference's
// image.Image into a straight-alpha [raster.Image]. A reference is kept only
// while what it allocates is bounded by the dimensions its header reports;
// one that a few bytes can make allocate more is replaced (ICO, 2026-10-31,
// see ico_decode.go), and Netpbm and QOI were always decoded here:
//
//	format  decoder                                    module
//	------  ----------------------------------------   ------------------------------------
//	PNG     image/png                                  standard library
//	JPEG    image/jpeg                                 standard library
//	GIF     image/gif                                  standard library
//	WEBP    golang.org/x/image/webp                    golang.org/x/image
//	TIFF    golang.org/x/image/tiff                    golang.org/x/image
//	BMP     golang.org/x/image/bmp                     golang.org/x/image
//	ICO     this package (DIB); image/png (PNG entries)  [see ico_decode.go]
//	ICNS    image/png (per embedded PNG representation)  standard library  [see icns.go]
//	PNM     this package                               [see pnm.go]
//	QOI     this package                               [see qoi.go]
//	JP2     github.com/go-images/jpeg2000              github.com/go-images/jpeg2000
//	JBIG2   github.com/tannevaled/gobig2 [fork]         github.com/tannevaled/gobig2
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
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"

	"github.com/tannevaled/gobig2"

	"github.com/go-gfx/gfx/raster"
	jpeg2000 "github.com/go-images/jpeg2000"
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
	PNM   // Netpbm: PBM/PGM/PPM, ASCII (P1–P3) and binary (P4–P6)
	QOI   // Quite OK Image
	JP2   // JPEG 2000: the JP2 container and the bare codestream alike
	JBIG2 // JBIG2: the bitonal ink layer of a scanned page
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
	case PNM:
		return "PNM"
	case QOI:
		return "QOI"
	case JP2:
		return "JP2"
	case JBIG2:
		return "JBIG2"
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
	case len(data) >= 4 && string(data[:4]) == "qoif":
		return QOI
	// JPEG 2000 arrives two ways. A JP2 file opens with the twelve-byte
	// signature box; a PDF's /JPXDecode stream is often the bare codestream,
	// which opens with SOC immediately followed by SIZ. Both are the same
	// picture and the same decoder.
	case len(data) >= 12 && string(data[:12]) == "\x00\x00\x00\x0cjP  \r\n\x87\n":
		return JP2
	case len(data) >= 4 && data[0] == 0xFF && data[1] == 0x4F && data[2] == 0xFF && data[3] == 0x51:
		return JP2
	// A JBIG2 FILE opens with this identifier. A PDF's /JBIG2Decode stream is
	// the headerless embedded form, which carries no signature at all and so
	// cannot be sniffed: a PDF consumer knows the format from the dictionary
	// and decodes it there.
	case len(data) >= 8 && string(data[:8]) == "\x97JB2\r\n\x1a\n":
		return JBIG2
	case len(data) >= 2 && data[0] == 'P' && data[1] >= '1' && data[1] <= '6':
		return PNM
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
	return DecodeWith(data, Options{})
}

// Options tunes [DecodeWith].
type Options struct {
	// TargetSize selects among the representations of ICO and ICNS, as in
	// [DecodeBest]; <= 0 selects the largest.
	TargetSize int

	// MaxPixels refuses an image larger than this, from its header, before
	// it is decoded. Zero means [DefaultMaxPixels].
	//
	// ⛔ The number it guards comes from the FILE, and a compressed format
	// describes a large image in a few bytes: a PNG declaring 100000x100000
	// is about 40 KB of zeros and 40 GB once decoded. A service decoding
	// files from strangers should set this to what it actually needs.
	MaxPixels int
}

// DefaultMaxPixels is the largest image [Decode], [DecodeBest] and
// [DecodeWith] produce when no ceiling is given: forty million pixels, a
// hundred and sixty million bytes of RGBA (152 MiB), the ceiling svg and
// go-pdfkit/render use. A photograph from a 50-megapixel camera is above it
// and needs Options.MaxPixels.
const DefaultMaxPixels = 40_000_000

// ErrTooLarge reports an image whose header declares more pixels than the
// ceiling allows. Nothing has been decoded when it is returned.
var ErrTooLarge = errors.New("codec: image larger than the pixel ceiling")

// checkPixels refuses w x h above max, multiplied in int64: two numbers a
// file chose, multiplied in int on a 32-bit build, is how a ceiling is passed
// by overflowing past it.
func checkPixels(w, h int, max int64) error {
	if px := int64(w) * int64(h); px > max {
		return fmt.Errorf("%w: %dx%d is %d pixels, past the %d allowed", ErrTooLarge, w, h, px, max)
	}
	return nil
}

// DecodeWith is [DecodeBest] with a pixel ceiling, checked from each format's
// header before its decoder allocates anything.
func DecodeWith(data []byte, opt Options) (*raster.Image, error) {
	max := int64(opt.MaxPixels)
	if max <= 0 {
		max = DefaultMaxPixels
	}
	switch Sniff(data) {
	case PNG:
		return decodeSingle(png.Decode, png.DecodeConfig, data, max)
	case JPEG:
		return decodeSingle(jpeg.Decode, jpeg.DecodeConfig, data, max)
	case GIF:
		return decodeSingle(gif.Decode, gif.DecodeConfig, data, max)
	case WEBP:
		return decodeSingle(webp.Decode, webp.DecodeConfig, data, max)
	case TIFF:
		return decodeSingle(tiff.Decode, tiff.DecodeConfig, data, max)
	case BMP:
		return decodeSingle(bmp.Decode, bmp.DecodeConfig, data, max)
	case ICO:
		return decodeICO(data, opt.TargetSize, max)
	case ICNS:
		return decodeICNS(data, opt.TargetSize, max)
	case PNM:
		return decodePNM(data, max)
	case QOI:
		return decodeQOI(data, max)
	case JP2:
		// ⛔ A pixel count does not bound a JPEG 2000 decode: 16384
		// components may each be allocated at full size. The decoder's own
		// sample ceiling closes that; four samples a pixel is RGBA.
		j2k := func(r io.Reader) (image.Image, error) {
			return jpeg2000.DecodeWithOptions(r, jpeg2000.DecodeOptions{MaxSamples: 4 * max})
		}
		return decodeSingle(j2k, jpeg2000.DecodeConfig, data, max)
	case JBIG2:
		return decodeSingle(gobig2.Decode, gobig2.DecodeConfig, data, max)
	default:
		return nil, ErrUnknownFormat
	}
}

// DecodeBest is [Decode] with a target pixel size for the multi-representation
// formats. For ICO and ICNS it selects the representation whose larger side is
// the smallest that is still >= targetSize, falling back to the largest
// representation when none reaches the target; a targetSize <= 0 always selects
// the largest. targetSize is ignored for single-image formats, whose sole image
// is returned regardless.
func DecodeBest(data []byte, targetSize int) (*raster.Image, error) {
	return DecodeWith(data, Options{TargetSize: targetSize})
}

// decodeSingle reads the image's size with cfg, refuses it above max, then
// runs dec and converts the result to a straight-alpha [raster.Image]. Every
// decoder handed here allocates within the size its DecodeConfig reports (see
// the audit in ico_decode.go), which is what makes the check a bound.
func decodeSingle(dec func(io.Reader) (image.Image, error), cfg func(io.Reader) (image.Config, error), data []byte, max int64) (*raster.Image, error) {
	c, err := cfg(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if err := checkPixels(c.Width, c.Height, max); err != nil {
		return nil, err
	}
	img, err := dec(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return raster.FromImage(img), nil
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
