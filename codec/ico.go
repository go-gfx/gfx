// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/go-gfx/gfx/raster"
)

// This file MUXES the Windows .ico container. It writes no pixels: every
// representation in the file it produces is a PNG encoded by the standard
// library, and what is added around them is a 6-byte header and a 16-byte
// directory entry each. That is the same "container plus reference codec"
// shape as the .icns demux next door, and the reason it does not contradict
// the rule that this package implements no encoder of its own -- there is no
// ICO codec here to implement, because an ICO is a wrapper.
//
// It exists because an .ico is the one icon a favicon, a Windows executable and
// a desktop shortcut all still ask for, and nothing in this fleet could write
// one: every caller that needed one reached outside Go for it.

// icoMaxSide is the largest side an ICO directory entry can name. The width and
// height are single bytes in which zero means 256, so 256 is the ceiling and
// there is no encoding for anything above it.
const icoMaxSide = 256

// EncodeICO writes an .ico container holding every image given, in the order
// given.
//
// An .ico is a container of independent representations, not one image, which
// is why this is its own entry point rather than a format for [Encode]: a
// caller hands it the 16-, 32- and 256-pixel renderings it wants Windows and
// browsers to choose between, and the chooser picks per context.
//
// Each representation is stored as a PNG, so alpha survives exactly. PNG inside
// ICO has been read by Windows since Vista and by every browser that matters;
// the alternative, a headerless DIB with a separate 1-bit AND mask, would mean
// writing pixel data here, which this package does not do.
//
// It errors on no images at all, on a nil image, and on a side above 256, which
// the container cannot name.
func EncodeICO(w io.Writer, imgs ...*raster.Image) error {
	if len(imgs) == 0 {
		return fmt.Errorf("codec: ico: nothing to encode")
	}
	payloads := make([][]byte, len(imgs))
	for i, img := range imgs {
		if img == nil {
			return fmt.Errorf("codec: ico: image %d is nil", i)
		}
		if img.W <= 0 || img.H <= 0 {
			return fmt.Errorf("codec: ico: image %d is %dx%d", i, img.W, img.H)
		}
		if img.W > icoMaxSide || img.H > icoMaxSide {
			return fmt.Errorf("codec: ico: image %d is %dx%d, above the %d-pixel ceiling the container can name",
				i, img.W, img.H, icoMaxSide)
		}
		var buf bytes.Buffer
		if err := pngEncode(&buf, nrgba(img)); err != nil {
			return fmt.Errorf("codec: ico: image %d: %w", i, err)
		}
		payloads[i] = buf.Bytes()
	}

	// ICONDIR, then one ICONDIRENTRY each, then the payloads: the offsets can
	// only be written once every payload's length is known.
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(&out, binary.LittleEndian, uint16(1)) // 1 = icon, 2 = cursor
	binary.Write(&out, binary.LittleEndian, uint16(len(imgs)))

	off := 6 + 16*len(imgs)
	for i, img := range imgs {
		// A side of 256 is written as 0: the field is a single byte.
		out.WriteByte(byte(img.W % icoMaxSide))
		out.WriteByte(byte(img.H % icoMaxSide))
		out.WriteByte(0)                                    // palette size, 0 for a true-colour image
		out.WriteByte(0)                                    // reserved
		binary.Write(&out, binary.LittleEndian, uint16(1))  // colour planes
		binary.Write(&out, binary.LittleEndian, uint16(32)) // bits per pixel
		binary.Write(&out, binary.LittleEndian, uint32(len(payloads[i])))
		binary.Write(&out, binary.LittleEndian, uint32(off))
		off += len(payloads[i])
	}
	for _, p := range payloads {
		out.Write(p)
	}
	_, err := w.Write(out.Bytes())
	return err
}
