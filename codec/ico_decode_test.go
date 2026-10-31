// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"testing"

	"github.com/go-gfx/gfx/raster"
	ico "github.com/sergeymakinen/go-ico"
)

// dibSpec describes a headerless DIB representation for icoFile.
type dibSpec struct {
	w, h, bpp int
	topDown   bool
	palette   []color.RGBA          // for bpp <= 8
	pixel     func(x, y int) uint32 // a palette index, or 0xAARRGGBB
	mask      func(x, y int) bool   // AND-mask bit
}

// encode writes the BITMAPINFOHEADER, colour table, XOR rows and AND rows.
func (s dibSpec) encode() []byte {
	le := binary.LittleEndian
	hdr := make([]byte, 40)
	le.PutUint32(hdr[0:], 40)
	le.PutUint32(hdr[4:], uint32(s.w))
	hh := int32(2 * s.h)
	if s.topDown {
		hh = -hh
	}
	le.PutUint32(hdr[8:], uint32(hh))
	le.PutUint16(hdr[12:], 1)
	le.PutUint16(hdr[14:], uint16(s.bpp))
	le.PutUint32(hdr[32:], uint32(len(s.palette)))
	out := hdr
	for _, c := range s.palette {
		out = append(out, c.B, c.G, c.R, 0)
	}
	xorRow := (s.w*s.bpp + 31) / 32 * 4
	andRow := (s.w + 31) / 32 * 4
	row := func(r int) int { // file row r holds image row y
		if s.topDown {
			return r
		}
		return s.h - 1 - r
	}
	for r := 0; r < s.h; r++ {
		b := make([]byte, xorRow)
		y := row(r)
		for x := 0; x < s.w; x++ {
			v := s.pixel(x, y)
			switch s.bpp {
			case 24:
				b[3*x], b[3*x+1], b[3*x+2] = byte(v), byte(v>>8), byte(v>>16)
			case 32:
				b[4*x], b[4*x+1], b[4*x+2], b[4*x+3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
			default:
				bit := x * s.bpp
				b[bit/8] |= byte(v) << (8 - s.bpp - bit%8)
			}
		}
		out = append(out, b...)
	}
	for r := 0; r < s.h; r++ {
		b := make([]byte, andRow)
		y := row(r)
		for x := 0; x < s.w; x++ {
			if s.mask != nil && s.mask(x, y) {
				b[x/8] |= 0x80 >> (x % 8)
			}
		}
		out = append(out, b...)
	}
	return out
}

// icoFile wraps payloads in an ICONDIR.
func icoFile(payloads ...[]byte) []byte {
	le := binary.LittleEndian
	out := []byte{0, 0, 1, 0, 0, 0}
	le.PutUint16(out[4:], uint16(len(payloads)))
	off := 6 + 16*len(payloads)
	for _, p := range payloads {
		e := make([]byte, 16)
		le.PutUint32(e[8:], uint32(len(p)))
		le.PutUint32(e[12:], uint32(off))
		out = append(out, e...)
		off += len(p)
	}
	for _, p := range payloads {
		out = append(out, p...)
	}
	return out
}

var grey16 = func() []color.RGBA {
	p := make([]color.RGBA, 16)
	for i := range p {
		v := uint8(i * 17)
		p[i] = color.RGBA{v, 255 - v, v / 2, 0xFF}
	}
	return p
}()

func checker(x, y int) bool { return (x+y)%3 == 0 }

// Where go-ico was right, the replacement must agree with it pixel for pixel:
// every palette depth, 24 bits, and a 32-bit DIB whose alpha bytes are all
// zero (decoded opaque, AND mask applied). Odd widths exercise row padding.
func TestICODIBMatchesTheFormerReferenceWhereItWasRight(t *testing.T) {
	specs := map[string]dibSpec{
		"1bpp":  {w: 13, h: 7, bpp: 1, palette: grey16[:2], pixel: func(x, y int) uint32 { return uint32(x^y) & 1 }, mask: checker},
		"4bpp":  {w: 9, h: 5, bpp: 4, palette: grey16, pixel: func(x, y int) uint32 { return uint32(x+3*y) % 16 }, mask: checker},
		"8bpp":  {w: 5, h: 6, bpp: 8, palette: grey16, pixel: func(x, y int) uint32 { return uint32(x*y) % 16 }, mask: checker},
		"24bpp": {w: 7, h: 3, bpp: 24, pixel: func(x, y int) uint32 { return uint32(x*40)<<16 | uint32(y*60)<<8 | 7 }, mask: checker},
	}
	for name, s := range specs {
		t.Run(name, func(t *testing.T) {
			data := icoFile(s.encode())
			got, err := Decode(data)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			ref, err := ico.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("go-ico: %v", err)
			}
			want := raster.FromImage(ref)
			if got.W != want.W || got.H != want.H {
				t.Fatalf("%dx%d, go-ico %dx%d", got.W, got.H, want.W, want.H)
			}
			for y := 0; y < got.H; y++ {
				for x := 0; x < got.W; x++ {
					g, w := got.At(x, y), want.At(x, y)
					if g.A == 0 && w.A == 0 {
						continue // colour under full transparency is not a colour
					}
					if g != w {
						t.Fatalf("(%d,%d) = %v, go-ico %v", x, y, g, w)
					}
				}
			}
		})
	}
}

// Where go-ico was wrong, twice over for 32 bits: it keeps the fourth byte as
// alpha unconditionally AND applies the AND mask. Chromium decodes a 32-bit DIB
// whose alpha bytes are all zero as opaque and masks it ...
func TestICOA32BitDIBWithoutAlphaIsOpaqueThenMasked(t *testing.T) {
	s := dibSpec{w: 6, h: 4, bpp: 32, pixel: func(x, y int) uint32 { return uint32(x*30)<<16 | uint32(y*50)<<8 | 9 }, mask: checker}
	data := icoFile(s.encode())
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := ico.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 6; x++ {
			want := color.RGBA{uint8(x * 30), uint8(y * 50), 9, 0xFF}
			if checker(x, y) {
				want = color.RGBA{}
			}
			if got.At(x, y) != want {
				t.Fatalf("(%d,%d) = %v, want %v", x, y, got.At(x, y), want)
			}
			if _, _, _, a := ref.At(x, y).RGBA(); a != 0 {
				t.Fatalf("go-ico was measured to return this icon fully transparent; (%d,%d) has alpha %d", x, y, a)
			}
		}
	}
}

// ... and with real alpha it uses the alpha and skips the mask.
func TestICOKeepsTheAlphaOfA32BitDIB(t *testing.T) {
	s := dibSpec{w: 4, h: 2, bpp: 32,
		pixel: func(x, y int) uint32 { return uint32(x*60)<<24 | 0x00102030 },
		mask:  func(x, y int) bool { return true }, // would erase everything if applied
	}
	got, err := Decode(icoFile(s.encode()))
	if err != nil {
		t.Fatal(err)
	}
	for x := 0; x < 4; x++ {
		if want := (color.RGBA{0x10, 0x20, 0x30, uint8(x * 60)}); got.At(x, 1) != want {
			t.Fatalf("pixel %d = %v, want %v", x, got.At(x, 1), want)
		}
	}
	ref, err := ico.Decode(bytes.NewReader(icoFile(s.encode())))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := ref.At(3, 1).RGBA(); a != 0 {
		t.Fatalf("go-ico was measured to erase this pixel with the mask; alpha %d", a)
	}
}

func TestICOTopDownDIBKeepsItsOrientation(t *testing.T) {
	s := dibSpec{w: 2, h: 3, bpp: 24, topDown: true,
		pixel: func(x, y int) uint32 { return uint32(y*100) << 16 },
		mask:  func(x, y int) bool { return x == 1 && y == 0 },
	}
	got, err := Decode(icoFile(s.encode()))
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 3; y++ {
		if got.At(0, y).R != uint8(y*100) {
			t.Fatalf("row %d red = %d, want %d", y, got.At(0, y).R, y*100)
		}
	}
	if got.At(1, 0).A != 0 || got.At(1, 2).A != 0xFF {
		t.Fatalf("the mask must follow the same orientation: %v %v", got.At(1, 0), got.At(1, 2))
	}
}

// An index beyond a short colour table is opaque black, not a panic.
func TestICOPaletteIndexPastTheTable(t *testing.T) {
	s := dibSpec{w: 2, h: 1, bpp: 4, palette: grey16[:2], pixel: func(x, y int) uint32 { return 15 }}
	got, err := Decode(icoFile(s.encode()))
	if err != nil {
		t.Fatal(err)
	}
	if got.At(0, 0) != (color.RGBA{0, 0, 0, 0xFF}) {
		t.Fatalf("got %v", got.At(0, 0))
	}
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// The bomb go-ico could not refuse: 65535 directory entries all pointing at
// one 512x512 PNG. go-ico's DecodeAll decoded and kept 65535 copies: by
// arithmetic, 65535 x 1 MiB = 68.7 GB (not run, for obvious reasons). Only
// the chosen entry is decoded now.
func TestICOEntriesSharingOnePNGAreDecodedOnce(t *testing.T) {
	p := pngBytes(t, 512, 512)
	le := binary.LittleEndian
	const n = 65535
	data := make([]byte, 6+16*n, 6+16*n+len(p))
	le.PutUint16(data[2:], 1)
	le.PutUint16(data[4:], n)
	for i := 0; i < n; i++ {
		e := data[6+16*i:]
		le.PutUint32(e[8:], uint32(len(p)))
		le.PutUint32(e[12:], uint32(6+16*n))
	}
	data = append(data, p...)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	img, err := Decode(data)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if img.W != 512 {
		t.Fatalf("width %d", img.W)
	}
	// One decoded copy is 1 MiB (NRGBA) plus 1 MiB (raster), the directory
	// 65535 small entries: 16 MiB is generous, against ~68 GB before.
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 16<<20 {
		t.Fatalf("allocated %d bytes for one 512x512 representation", grew)
	}
}

func TestICORefusesWhatItsBytesDoNotHold(t *testing.T) {
	good := dibSpec{w: 8, h: 4, bpp: 8, palette: grey16, pixel: func(x, y int) uint32 { return 1 }}.encode()
	with := func(off int, v uint32) []byte {
		b := append([]byte{}, good...)
		binary.LittleEndian.PutUint32(b[off:], v)
		return icoFile(b)
	}
	with16 := func(off int, v uint16) []byte {
		b := append([]byte{}, good...)
		binary.LittleEndian.PutUint16(b[off:], v)
		return icoFile(b)
	}
	outside := icoFile(good)
	binary.LittleEndian.PutUint32(outside[6+12:], uint32(len(outside))) // offset past the end
	zero := icoFile(good)
	binary.LittleEndian.PutUint32(zero[6+8:], 0) // dwBytesInRes 0
	badPNG := icoFile(append(append([]byte{}, pngMagic...), 0, 0, 0, 0))
	ihdr := func(off int, v uint32) []byte {
		p := pngBytes(t, 4, 4)
		binary.BigEndian.PutUint32(p[off:], v)
		return icoFile(p)
	}
	corruptPNG := pngBytes(t, 4, 4)
	corruptPNG = corruptPNG[:len(corruptPNG)-20] // IHDR intact, image data cut

	bad := map[string][]byte{
		"not a container":           []byte("\x00\x00\x02\x00\x01\x00"),
		"too short":                 {0, 0, 1},
		"no entries":                {0, 0, 1, 0, 0, 0},
		"directory past the end":    {0, 0, 1, 0, 2, 0, 1},
		"entry past the end":        outside,
		"empty entry":               zero,
		"PNG header unreadable":     badPNG,
		"PNG data cut":              icoFile(corruptPNG),
		"IHDR length not 13":        ihdr(8, 12),
		"IHDR width zero":           ihdr(16, 0),
		"IHDR height past 2^31-1":   ihdr(20, 1<<31),
		"DIB shorter than a header": icoFile(good[:39]),
		"AND rows missing":          icoFile(good[:len(good)-1]),
		"infoLen below 40":          with(0, 12),
		"infoLen past the entry":    with(0, 1<<20),
		"width zero":                with(4, 0),
		"width negative":            with(4, 0xFFFFFFFF),
		"height odd":                with(8, 9),
		"height zero":               with(8, 0),
		"two planes":                with16(12, 2),
		"16 bits":                   with16(14, 16),
		"compressed":                with(16, 1),
		"more colours than depth":   with(32, 257),
		"colour table cut":          icoFile(good[:40+100]),
		// 2^31-1 wide and one row: the row alone is far past the entry.
		"rows past the entry": with(4, 0x7FFFFFFF),
	}
	for name, data := range bad {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeICO(data, 0); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	t.Run("raster past an int", func(t *testing.T) {
		defer func(v uint64) { icoMaxPixels = v }(icoMaxPixels)
		icoMaxPixels = 31 // the 8x4 DIB is 32 pixels
		if _, err := decodeICO(icoFile(good), 0); err == nil {
			t.Fatal("expected an error")
		}
	})
}

// biClrUsed 0 means a full table for the depth (1 << bpp entries).
func TestICOColourCountZeroMeansAFullTable(t *testing.T) {
	s := dibSpec{w: 2, h: 1, bpp: 1, palette: grey16[:2], pixel: func(x, y int) uint32 { return uint32(x) }}
	b := s.encode()
	binary.LittleEndian.PutUint32(b[32:], 0)
	got, err := decodeICO(icoFile(b), 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.At(1, 0) != grey16[1] {
		t.Fatalf("got %v, want %v", got.At(1, 0), grey16[1])
	}
}

// The replacement must be no slower than go-ico. Each pair decodes the same
// file; run with -benchmem, interleaved rounds, on an idle machine.
func benchICO(b *testing.B, data []byte, ours bool) {
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		if ours {
			if _, err := decodeICO(data, 0); err != nil {
				b.Fatal(err)
			}
		} else {
			m, err := ico.Decode(bytes.NewReader(data))
			if err != nil {
				b.Fatal(err)
			}
			raster.FromImage(m)
		}
	}
}

func icoDIB256() []byte {
	return icoFile(dibSpec{w: 256, h: 256, bpp: 32, pixel: func(x, y int) uint32 { return uint32(x)<<24 | uint32(y)<<8 | 0x40 }}.encode())
}

func BenchmarkICODIB32Ours(b *testing.B)  { benchICO(b, icoDIB256(), true) }
func BenchmarkICODIB32GoICO(b *testing.B) { benchICO(b, icoDIB256(), false) }

func BenchmarkICOPNGOurs(b *testing.B) {
	benchICO(b, icoFile(pngBytesB(b, 256, 256)), true)
}
func BenchmarkICOPNGGoICO(b *testing.B) {
	benchICO(b, icoFile(pngBytesB(b, 256, 256)), false)
}

func pngBytesB(b *testing.B, w, h int) []byte {
	var buf bytes.Buffer
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range m.Pix {
		m.Pix[i] = uint8(i * 7)
	}
	if err := png.Encode(&buf, m); err != nil {
		b.Fatal(err)
	}
	return buf.Bytes()
}

func FuzzDecodeICO(f *testing.F) {
	f.Add(icoFile(dibSpec{w: 3, h: 2, bpp: 1, palette: grey16[:2], pixel: func(x, y int) uint32 { return uint32(x) & 1 }, mask: checker}.encode()))
	f.Add(icoFile(dibSpec{w: 2, h: 2, bpp: 32, pixel: func(x, y int) uint32 { return 0x80FF0000 }}.encode()))
	f.Add(icoFile(dibSpec{w: 3, h: 1, bpp: 24, topDown: true, pixel: func(x, y int) uint32 { return 0xFF }}.encode()))
	f.Fuzz(func(t *testing.T, data []byte) {
		img, err := decodeICO(data, 0)
		if err == nil && (img.W <= 0 || img.H <= 0) {
			t.Fatalf("no error and an empty %dx%d image", img.W, img.H)
		}
	})
}
