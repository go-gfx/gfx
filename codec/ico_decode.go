// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image/color"
	"image/png"
	"math"

	"github.com/go-gfx/gfx/raster"
)

// This file DECODES the Windows .ico container, including the headerless DIB
// representations, itself. Until 2026-10-31 it handed the file to
// github.com/sergeymakinen/go-ico, and that library cannot be fed bytes from a
// stranger:
//
//   - its DecodeAll, which this package called, decodes and KEEPS every
//     directory entry. The count is 16 bits and nothing stops 65535 entries
//     from pointing at the same embedded PNG, so one compressed picture plus a
//     megabyte of directory asks for 65535 decoded copies of it. A per-image
//     pixel ceiling cannot see that.
//   - on a reader that cannot seek backwards it allocates dwBytesInRes, a
//     32-bit size taken from the file, before reading it -- as does Microsoft's
//     own sample reader in "Icons" (John Hornick, 1995, MSDN ms997538).
//   - it decodes a 32-bit DIB through github.com/sergeymakinen/go-bmp, which
//     keeps the fourth byte as alpha unconditionally, and then applies the AND
//     mask on top. A 32-bit icon whose alpha bytes are all zero -- common in
//     icons written before alpha was used -- came out fully transparent
//     (measured), and one with real alpha was masked a second time.
//
// Here the directory is checked against the file before anything else, every
// representation is a sub-slice of the input (nothing is allocated from a
// size field), each one's dimensions are read from its header alone, and only
// the chosen representation is decoded. A DIB must hold all the bytes its
// header implies -- colour table, XOR rows and AND rows -- before its raster is
// allocated, so a DIB entry cannot ask for more than its own bytes describe.
// An embedded PNG's size is read from its IHDR, which the PNG specification
// (W3C PNG Third Edition, 11.2.1) requires to be the first chunk, 13 bytes,
// with width and height non-zero and at most 2^31-1 -- without a decoder,
// because png.DecodeConfig costs a kilobyte of garbage a call and a directory
// can name 65535 entries. The chosen one then goes to image/png, whose
// allocation is bounded by those dimensions and which checks the rest.
//
// References: the ICONDIR / ICONDIRENTRY / ICONIMAGE layout and "biHeight is
// the combined height of the XOR and AND masks" are from ms997538 above. How
// alpha and the AND mask combine is Chromium's, read from
// third_party/blink/renderer/platform/image-decoders/bmp/bmp_image_reader.cc:
// inside an ICO a 32-bit DIB's fourth byte IS alpha; if every alpha byte is
// zero the image is decoded opaque and the AND mask applies, otherwise the
// mask is skipped; at every other depth the AND mask applies, and a set bit
// makes the pixel fully transparent. Rows are padded to 32 bits.

var (
	errICONotContainer = errors.New("ico: not an icon container")
	errICOEntry        = errors.New("ico: directory entry outside the file")
	errICODIB          = errors.New("ico: unsupported or corrupt DIB representation")
	errICOPNG          = errors.New("ico: PNG representation without a valid leading IHDR")
)

// icoEntry is one representation, still encoded: a PNG stream, or a parsed DIB
// whose pixel bytes are sub-slices of the input.
type icoEntry struct {
	png  []byte
	dib  *icoDIB
	w, h int
}

// icoEntries walks the directory and reads every representation's size from
// its header alone.
func icoEntries(data []byte) ([]icoEntry, error) {
	if len(data) < 6 || binary.LittleEndian.Uint16(data[0:]) != 0 || binary.LittleEndian.Uint16(data[2:]) != 1 {
		return nil, errICONotContainer
	}
	n := int(binary.LittleEndian.Uint16(data[4:]))
	if n == 0 || 6+16*n > len(data) {
		return nil, errICOEntry
	}
	entries := make([]icoEntry, n) // n*16 bytes of directory are in the file
	for i := range entries {
		d := data[6+16*i:]
		size := uint64(binary.LittleEndian.Uint32(d[8:]))
		off := uint64(binary.LittleEndian.Uint32(d[12:]))
		if size == 0 || off+size > uint64(len(data)) {
			return nil, errICOEntry
		}
		p := data[off : off+size]
		if bytes.HasPrefix(p, pngMagic) {
			w, h, err := pngIHDRSize(p)
			if err != nil {
				return nil, err
			}
			entries[i] = icoEntry{png: p, w: w, h: h}
			continue
		}
		dib, err := parseICODIB(p)
		if err != nil {
			return nil, err
		}
		entries[i] = icoEntry{dib: dib, w: dib.w, h: dib.h}
	}
	return entries, nil
}

// pngIHDRSize reads a PNG's width and height from its leading IHDR chunk.
func pngIHDRSize(p []byte) (w, h int, err error) {
	if len(p) < 24 || binary.BigEndian.Uint32(p[8:]) != 13 || string(p[12:16]) != "IHDR" {
		return 0, 0, errICOPNG
	}
	uw, uh := binary.BigEndian.Uint32(p[16:]), binary.BigEndian.Uint32(p[20:])
	if uw == 0 || uh == 0 || uw > math.MaxInt32 || uh > math.MaxInt32 {
		return 0, 0, errICOPNG
	}
	return int(uw), int(uh), nil
}

// decodeICO decodes a Windows .ico container and returns the representation
// best matching targetSize (see pickIndex). Only that one is decoded.
func decodeICO(data []byte, targetSize int) (*raster.Image, error) {
	entries, err := icoEntries(data)
	if err != nil {
		return nil, err
	}
	sizes := make([]dim, len(entries))
	for i, e := range entries {
		sizes[i] = dim{e.w, e.h}
	}
	e := entries[pickIndex(sizes, targetSize)]
	if e.dib != nil {
		return e.dib.decode(), nil
	}
	img, err := png.Decode(bytes.NewReader(e.png))
	if err != nil {
		return nil, err
	}
	return raster.FromImage(img), nil
}

// icoMaxPixels keeps a DIB's raster, 4 bytes a pixel and up to 32 times its
// 1-bit data, inside an int: on a 32-bit target, past 64 MiB of input, it
// would not be. A variable so a 64-bit test can reach the refusal.
var icoMaxPixels uint64 = math.MaxInt / 4

// icoDIB is a headerless DIB representation whose every byte has been checked
// to be present.
type icoDIB struct {
	w, h    int
	topDown bool
	bpp     int
	palette []color.RGBA
	xor     []byte // h rows of xorRow bytes
	and     []byte // h rows of andRow bytes
	xorRow  int
	andRow  int
}

// parseICODIB reads a BITMAPINFOHEADER-led DIB and checks that the colour
// table, the XOR rows and the AND rows its header implies are all in p. Only
// uncompressed (BI_RGB) data at 1, 2, 4, 8, 24 or 32 bits is accepted, the
// depths x/image/bmp, which this replaces, decoded.
func parseICODIB(p []byte) (*icoDIB, error) {
	if len(p) < 40 {
		return nil, errICODIB
	}
	infoLen := uint64(binary.LittleEndian.Uint32(p[0:]))
	w := int64(int32(binary.LittleEndian.Uint32(p[4:])))
	hh := int64(int32(binary.LittleEndian.Uint32(p[8:])))
	planes := binary.LittleEndian.Uint16(p[12:])
	bpp := int(binary.LittleEndian.Uint16(p[14:]))
	compression := binary.LittleEndian.Uint32(p[16:])
	used := uint64(binary.LittleEndian.Uint32(p[32:]))
	if infoLen < 40 || infoLen > uint64(len(p)) || planes != 1 || compression != 0 || w <= 0 {
		return nil, errICODIB
	}
	d := &icoDIB{bpp: bpp}
	if hh < 0 {
		hh, d.topDown = -hh, true
	}
	if hh == 0 || hh%2 != 0 { // XOR and AND masks share biHeight
		return nil, errICODIB
	}
	pos := infoLen
	switch bpp {
	case 1, 2, 4, 8:
		if used == 0 {
			used = 1 << bpp
		}
		if used > 1<<bpp || pos+4*used > uint64(len(p)) {
			return nil, errICODIB
		}
		d.palette = make([]color.RGBA, used) // at most 256 entries
		for i := range d.palette {
			q := p[pos+4*uint64(i):]
			d.palette[i] = color.RGBA{q[2], q[1], q[0], 0xFF}
		}
		pos += 4 * used
	case 24, 32:
	default:
		return nil, errICODIB
	}
	// In 64 bits: w is below 2^31 and bpp at most 32, so nothing here wraps,
	// on a 32-bit target either.
	xorRow := (uint64(w)*uint64(bpp) + 31) / 32 * 4
	andRow := (uint64(w) + 31) / 32 * 4
	h := uint64(hh / 2)
	if h > (uint64(len(p))-pos)/(xorRow+andRow) {
		return nil, errICODIB
	}
	if uint64(w)*h > icoMaxPixels {
		return nil, errICODIB
	}
	// Every product below is now at most len(p), so it fits an int.
	d.w, d.h, d.xorRow, d.andRow = int(w), int(h), int(xorRow), int(andRow)
	d.xor = p[pos : pos+h*xorRow]
	d.and = p[pos+h*xorRow : pos+h*(xorRow+andRow)]
	return d, nil
}

// decode renders the DIB; it cannot fail, parseICODIB having checked every
// byte it reads.
func (d *icoDIB) decode() *raster.Image {
	img := raster.New(d.w, d.h)
	seenAlpha := false
	for row := 0; row < d.h; row++ {
		y := d.h - 1 - row // DIB rows run bottom-up unless biHeight was negative
		if d.topDown {
			y = row
		}
		src := d.xor[row*d.xorRow:]
		dst := img.Pix[4*d.w*y:]
		for x := 0; x < d.w; x++ {
			var c color.RGBA
			switch d.bpp {
			case 24:
				c = color.RGBA{src[3*x+2], src[3*x+1], src[3*x], 0xFF}
			case 32:
				c = color.RGBA{src[4*x+2], src[4*x+1], src[4*x], src[4*x+3]}
				seenAlpha = seenAlpha || c.A != 0
			default:
				bit := x * d.bpp
				idx := int(src[bit/8]>>(8-d.bpp-bit%8)) & (1<<d.bpp - 1)
				c = color.RGBA{0, 0, 0, 0xFF} // an index past a short table
				if idx < len(d.palette) {
					c = d.palette[idx]
				}
			}
			dst[4*x], dst[4*x+1], dst[4*x+2], dst[4*x+3] = c.R, c.G, c.B, c.A
		}
	}
	if seenAlpha {
		return img // a 32-bit DIB with real alpha: the AND mask is not used
	}
	for row := 0; row < d.h; row++ {
		y := d.h - 1 - row
		if d.topDown {
			y = row
		}
		mask := d.and[row*d.andRow:]
		dst := img.Pix[4*d.w*y:]
		for x := 0; x < d.w; x++ {
			if mask[x/8]>>(7-x%8)&1 != 0 {
				dst[4*x], dst[4*x+1], dst[4*x+2], dst[4*x+3] = 0, 0, 0, 0
			} else {
				dst[4*x+3] = 0xFF // a 32-bit DIB whose alpha bytes are all zero
			}
		}
	}
	return img
}
