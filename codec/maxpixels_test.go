// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"runtime"
	"testing"

	jpeg2000 "github.com/go-images/jpeg2000"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

// The ceiling is exact for every format: an image of exactly MaxPixels
// decodes, one pixel fewer allowed refuses it with ErrTooLarge.
func TestMaxPixelsIsExactForEveryFormat(t *testing.T) {
	m := gradient(10, 10) // 100 pixels
	enc := func(f func(*bytes.Buffer) error) []byte {
		var b bytes.Buffer
		if err := f(&b); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	files := map[string][]byte{
		"png":  enc(func(b *bytes.Buffer) error { return png.Encode(b, m) }),
		"jpeg": enc(func(b *bytes.Buffer) error { return jpeg.Encode(b, m, nil) }),
		"gif":  enc(func(b *bytes.Buffer) error { return gif.Encode(b, m, nil) }),
		"bmp":  enc(func(b *bytes.Buffer) error { return bmp.Encode(b, m) }),
		"tiff": enc(func(b *bytes.Buffer) error { return tiff.Encode(b, m, nil) }),
		"jp2":  enc(func(b *bytes.Buffer) error { return jpeg2000.Encode(b, m, nil) }),
		"ico":  icoFile(dibSpec{w: 10, h: 10, bpp: 24, pixel: func(x, y int) uint32 { return 0 }}.encode()),
		"icns": icnsOf(t, enc(func(b *bytes.Buffer) error { return png.Encode(b, m) })),
		"pnm":  append([]byte("P5\n10 10\n255\n"), make([]byte, 100)...),
		"qoi":  append([]byte{'q', 'o', 'i', 'f', 0, 0, 0, 10, 0, 0, 0, 10, 4, 0, 0xFD}, 0xE5), // RUN 62 + RUN 38
	}
	for name, data := range files {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeWith(data, Options{MaxPixels: 100}); err != nil {
				t.Fatalf("at the ceiling: %v", err)
			}
			if _, err := DecodeWith(data, Options{MaxPixels: 99}); !errors.Is(err, ErrTooLarge) {
				t.Fatalf("one over: err = %v, want ErrTooLarge", err)
			}
		})
	}
}

// icnsOf wraps one PNG as an .icns ic07 chunk.
func icnsOf(t *testing.T, p []byte) []byte {
	t.Helper()
	out := make([]byte, 16, 16+len(p))
	copy(out, "icns")
	binary.BigEndian.PutUint32(out[4:], uint32(16+len(p)))
	copy(out[8:], "ic07")
	binary.BigEndian.PutUint32(out[12:], uint32(8+len(p)))
	return append(out, p...)
}

// A 1x1 PNG whose IHDR (CRC recomputed) claims 100000x100000: 40 GB once
// decoded. Decode, with the default ceiling, refuses it from the header.
func TestTheDefaultCeilingRefusesAPNGHeaderBomb(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	p := b.Bytes()
	binary.BigEndian.PutUint32(p[16:], 100000)
	binary.BigEndian.PutUint32(p[20:], 100000)
	binary.BigEndian.PutUint32(p[29:], crc32.ChecksumIEEE(p[12:29]))
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := Decode(p)
	runtime.ReadMemStats(&after)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 1<<20 {
		t.Fatalf("allocated %d bytes before refusing", grew)
	}
}

// A pixel count does not bound JPEG 2000: 64 components of 64x64 are 4096
// pixels, inside a ceiling of 4096, and 262144 samples. codec hands jpeg2000
// a sample ceiling of four a pixel, so the decoder refuses it itself.
func TestJPEG2000ComponentsAreBoundedThroughCodec(t *testing.T) {
	data := j2kComponents(64, 64, 64)
	_, err := DecodeWith(data, Options{MaxPixels: 4096})
	if !errors.Is(err, jpeg2000.ErrTooLarge) {
		t.Fatalf("err = %v, want jpeg2000.ErrTooLarge", err)
	}
	if _, err := DecodeWith(j2kComponents(64, 64, 4), Options{MaxPixels: 4096}); err != nil {
		t.Fatalf("four components at the ceiling: %v", err)
	}
}

// j2kComponents is a codestream of w x h with c 8-bit components and no
// packet data (the fixture go-images/jpeg2000's component_budget_test uses).
func j2kComponents(w, h, c int) []byte {
	be := binary.BigEndian
	var b bytes.Buffer
	put16 := func(v int) { var x [2]byte; be.PutUint16(x[:], uint16(v)); b.Write(x[:]) }
	put32 := func(v int) { var x [4]byte; be.PutUint32(x[:], uint32(v)); b.Write(x[:]) }
	put16(0xFF4F)
	put16(0xFF51)
	put16(38 + 3*c)
	put16(0)
	put32(w)
	put32(h)
	put32(0)
	put32(0)
	put32(w)
	put32(h)
	put32(0)
	put32(0)
	put16(c)
	for i := 0; i < c; i++ {
		b.Write([]byte{7, 1, 1})
	}
	put16(0xFF52)
	put16(12)
	b.Write([]byte{0, 0, 0, 1, 0, 0, 4, 4, 0, 1})
	put16(0xFF5C)
	put16(4)
	b.Write([]byte{0x40, 0x48})
	put16(0xFF90)
	put16(10)
	put16(0)
	put32(14)
	b.Write([]byte{0, 1})
	put16(0xFF93)
	put16(0xFFD9)
	return b.Bytes()
}
