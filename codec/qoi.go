// Copyright (c) the go-gfx authors.
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"encoding/binary"
	"fmt"
	"image/color"

	"github.com/go-gfx/gfx/raster"
)

// decodeQOI decodes the "Quite OK Image" format (https://qoiformat.org): a 14-byte
// header ("qoif", width, height, channels, colorspace) followed by run/index/diff/
// luma/rgb/rgba chunks and an 8-byte end marker.
func decodeQOI(data []byte) (*raster.Image, error) {
	if len(data) < 14 || string(data[:4]) != "qoif" {
		return nil, fmt.Errorf("codec: not a QOI image")
	}
	w := int(binary.BigEndian.Uint32(data[4:8]))
	h := int(binary.BigEndian.Uint32(data[8:12]))
	if w <= 0 || h <= 0 || w > 1<<20 || h > 1<<20 {
		return nil, fmt.Errorf("codec: bad QOI dimensions %dx%d", w, h)
	}
	img := raster.New(w, h)
	var index [64]color.RGBA
	px := color.RGBA{0, 0, 0, 255}
	pos, run := 14, 0
	hashPos := func(c color.RGBA) int {
		return (int(c.R)*3 + int(c.G)*5 + int(c.B)*7 + int(c.A)*11) % 64
	}
	for i := 0; i < w*h; i++ {
		if run > 0 {
			run--
		} else {
			if pos >= len(data) {
				return nil, fmt.Errorf("codec: truncated QOI stream")
			}
			b := data[pos]
			pos++
			switch {
			case b == 0xFE: // QOI_OP_RGB
				if pos+3 > len(data) {
					return nil, fmt.Errorf("codec: truncated QOI RGB")
				}
				px.R, px.G, px.B = data[pos], data[pos+1], data[pos+2]
				pos += 3
			case b == 0xFF: // QOI_OP_RGBA
				if pos+4 > len(data) {
					return nil, fmt.Errorf("codec: truncated QOI RGBA")
				}
				px.R, px.G, px.B, px.A = data[pos], data[pos+1], data[pos+2], data[pos+3]
				pos += 4
			case b&0xC0 == 0x00: // QOI_OP_INDEX
				px = index[b&0x3F]
			case b&0xC0 == 0x40: // QOI_OP_DIFF
				px.R += (b>>4)&3 - 2
				px.G += (b>>2)&3 - 2
				px.B += b&3 - 2
			case b&0xC0 == 0x80: // QOI_OP_LUMA
				if pos >= len(data) {
					return nil, fmt.Errorf("codec: truncated QOI LUMA")
				}
				b2 := data[pos]
				pos++
				dg := (b & 0x3F) - 32
				px.R += dg - 8 + (b2>>4)&0x0F
				px.G += dg
				px.B += dg - 8 + b2&0x0F
			default: // QOI_OP_RUN (b&0xC0 == 0xC0)
				run = int(b & 0x3F) // bias of 1 is consumed by this pixel
			}
			index[hashPos(px)] = px
		}
		img.Set(i%w, i/w, px)
	}
	return img, nil
}
