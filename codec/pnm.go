// Copyright (c) the go-gfx authors.
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"fmt"
	"image/color"

	"github.com/go-gfx/gfx/raster"
)

// decodePNM decodes a Netpbm image — the ASCII forms P1 (bitmap), P2 (graymap),
// P3 (pixmap) and the binary forms P4/P5/P6 — into a straight-alpha raster.Image.
// The header is "P<n>" followed by whitespace-separated width, height and (except
// for bitmaps) a maximum sample value; comments run from '#' to end of line and
// may appear anywhere in the header.
func decodePNM(data []byte) (*raster.Image, error) {
	if len(data) < 2 || data[0] != 'P' || data[1] < '1' || data[1] > '6' {
		return nil, fmt.Errorf("codec: not a Netpbm image")
	}
	kind := data[1] - '0'
	binary := kind >= 4
	p := &pnmParser{data: data, pos: 2}
	w, err := p.headerInt()
	if err != nil {
		return nil, err
	}
	h, err := p.headerInt()
	if err != nil {
		return nil, err
	}
	maxval := 1
	if kind != 1 && kind != 4 { // graymaps and pixmaps carry a max sample value
		if maxval, err = p.headerInt(); err != nil {
			return nil, err
		}
	}
	if w <= 0 || h <= 0 || maxval <= 0 || maxval > 65535 {
		return nil, fmt.Errorf("codec: bad Netpbm header %dx%d maxval %d", w, h, maxval)
	}
	// Exactly one whitespace byte separates the binary header from the raster.
	if binary {
		p.pos++
	}
	img := raster.New(w, h)
	scale := func(v int) uint8 {
		if maxval == 255 {
			return uint8(v)
		}
		return uint8(v * 255 / maxval)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b uint8
			switch kind {
			case 1: // ASCII bitmap: 1 = black
				v, err := p.asciiInt()
				if err != nil {
					return nil, err
				}
				if v == 0 {
					r, g, b = 255, 255, 255
				}
			case 2: // ASCII graymap
				v, err := p.asciiInt()
				if err != nil {
					return nil, err
				}
				r, g, b = scale(v), scale(v), scale(v)
			case 3: // ASCII pixmap
				rr, e1 := p.asciiInt()
				gg, e2 := p.asciiInt()
				bb, e3 := p.asciiInt()
				if e1 != nil || e2 != nil || e3 != nil {
					return nil, fmt.Errorf("codec: truncated Netpbm pixmap")
				}
				r, g, b = scale(rr), scale(gg), scale(bb)
			case 4: // binary bitmap: MSB-first, 1 = black, rows byte-aligned
				bit, err := p.bit(x, y, w)
				if err != nil {
					return nil, err
				}
				if bit == 0 {
					r, g, b = 255, 255, 255
				}
			case 5: // binary graymap
				v, err := p.sample(maxval)
				if err != nil {
					return nil, err
				}
				r, g, b = scale(v), scale(v), scale(v)
			case 6: // binary pixmap
				rr, e1 := p.sample(maxval)
				gg, e2 := p.sample(maxval)
				bb, e3 := p.sample(maxval)
				if e1 != nil || e2 != nil || e3 != nil {
					return nil, fmt.Errorf("codec: truncated Netpbm pixmap")
				}
				r, g, b = scale(rr), scale(gg), scale(bb)
			}
			img.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}
	return img, nil
}

// pnmParser walks a Netpbm byte stream.
type pnmParser struct {
	data []byte
	pos  int
}

// headerInt reads the next whitespace-separated integer of the header, skipping
// spaces and '#'-comments.
func (p *pnmParser) headerInt() (int, error) {
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		switch {
		case c == '#':
			for p.pos < len(p.data) && p.data[p.pos] != '\n' {
				p.pos++
			}
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			p.pos++
		default:
			return p.asciiInt()
		}
	}
	return 0, fmt.Errorf("codec: truncated Netpbm header")
}

// asciiInt reads an unsigned decimal integer, skipping any leading whitespace and
// comments.
func (p *pnmParser) asciiInt() (int, error) {
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		if c == '#' {
			for p.pos < len(p.data) && p.data[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			p.pos++
			continue
		}
		break
	}
	start := p.pos
	for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
		p.pos++
	}
	if p.pos == start {
		return 0, fmt.Errorf("codec: expected a Netpbm integer")
	}
	n := 0
	for _, d := range p.data[start:p.pos] {
		n = n*10 + int(d-'0')
	}
	return n, nil
}

// sample reads one binary sample (1 byte for maxval<256, else 2 bytes big-endian).
func (p *pnmParser) sample(maxval int) (int, error) {
	if maxval < 256 {
		if p.pos >= len(p.data) {
			return 0, fmt.Errorf("codec: truncated Netpbm raster")
		}
		v := int(p.data[p.pos])
		p.pos++
		return v, nil
	}
	if p.pos+1 >= len(p.data) {
		return 0, fmt.Errorf("codec: truncated Netpbm raster")
	}
	v := int(p.data[p.pos])<<8 | int(p.data[p.pos+1])
	p.pos += 2
	return v, nil
}

// bit reads the (x,y) pixel of a binary bitmap; rows are padded to whole bytes.
func (p *pnmParser) bit(x, y, w int) (int, error) {
	rowBytes := (w + 7) / 8
	idx := p.pos + y*rowBytes + x/8
	if idx >= len(p.data) {
		return 0, fmt.Errorf("codec: truncated Netpbm bitmap")
	}
	return int(p.data[idx]>>(7-uint(x%8))) & 1, nil
}
