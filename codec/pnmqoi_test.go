// Copyright (c) the go-gfx authors.
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"image/color"
	"runtime"
	"testing"
)

var (
	white = color.RGBA{255, 255, 255, 255}
	black = color.RGBA{0, 0, 0, 255}
	red   = color.RGBA{255, 0, 0, 255}
	green = color.RGBA{0, 255, 0, 255}
	blue  = color.RGBA{0, 0, 255, 255}
)

func TestPNM(t *testing.T) {
	cases := []struct {
		name string
		data string
		w, h int
		want map[[2]int]color.RGBA
	}{
		{"P3 ascii pixmap", "P3\n2 1\n255\n255 0 0  0 255 0\n", 2, 1,
			map[[2]int]color.RGBA{{0, 0}: red, {1, 0}: green}},
		{"P2 ascii graymap", "P2 2 1 255  0 255", 2, 1,
			map[[2]int]color.RGBA{{0, 0}: black, {1, 0}: white}},
		{"P1 ascii bitmap", "P1\n2 1\n1 0\n", 2, 1, // 1 = black, 0 = white
			map[[2]int]color.RGBA{{0, 0}: black, {1, 0}: white}},
		{"P3 comment + rescale", "P3\n# a comment\n1 1\n15\n15 0 0\n", 1, 1,
			map[[2]int]color.RGBA{{0, 0}: red}},
		{"P6 binary pixmap", "P6\n1 1\n255\n\x00\x00\xff", 1, 1,
			map[[2]int]color.RGBA{{0, 0}: blue}},
		{"P5 binary graymap", "P5\n2 1\n255\n\x00\xff", 2, 1,
			map[[2]int]color.RGBA{{0, 0}: black, {1, 0}: white}},
		{"P5 16-bit graymap", "P5\n1 1\n65535\n\xff\xff", 1, 1,
			map[[2]int]color.RGBA{{0, 0}: white}},
		{"P4 binary bitmap", "P4\n2 1\n\x80", 2, 1, // MSB set → x0 black, x1 white
			map[[2]int]color.RGBA{{0, 0}: black, {1, 0}: white}},
		{"P2 comment between samples", "P2 1 1 255 # hi\n128", 1, 1,
			map[[2]int]color.RGBA{{0, 0}: {128, 128, 128, 255}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Sniff([]byte(tc.data)); got != PNM {
				t.Fatalf("Sniff = %v, want PNM", got)
			}
			img, err := Decode([]byte(tc.data))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if img.W != tc.w || img.H != tc.h {
				t.Fatalf("size %dx%d, want %dx%d", img.W, img.H, tc.w, tc.h)
			}
			for xy, want := range tc.want {
				if got := img.At(xy[0], xy[1]); got != want {
					t.Errorf("pixel %v = %v, want %v", xy, got, want)
				}
			}
		})
	}
}

func TestPNMErrors(t *testing.T) {
	bad := []struct{ name, data string }{
		{"not netpbm", "P0 1 1"},
		{"too short", "P"},
		{"bad char", "XY"},
		{"truncated width", "P6\n"},
		{"truncated height", "P6\n2 "},
		{"missing maxval", "P5\n1 1\n"},
		{"non-integer dim", "P6\nX Y\n255\n\x00\x00\x00"},
		{"zero dims", "P6\n0 0\n255\n"},
		{"maxval too big", "P5\n1 1\n70000\n\x00"},
		{"P1 truncated", "P1\n2 1\n1"},
		{"P2 truncated", "P2\n2 1\n255\n0"},
		{"P3 truncated pixmap", "P3\n1 1\n255\n15 0"},
		{"P4 truncated bitmap", "P4\n8 2\n\x00"},
		{"P5 truncated raster", "P5\n2 1\n255\n\x00"},
		{"P5 16-bit truncated", "P5\n1 1\n65535\n\x00"},
		{"P6 truncated pixmap", "P6\n1 1\n255\n\x00\x00"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodePNM([]byte(tc.data), DefaultMaxPixels); err == nil {
				t.Errorf("%q: expected an error", tc.data)
			}
		})
	}
}

func TestQOI(t *testing.T) {
	hdr := func(w, h byte) []byte {
		return []byte{'q', 'o', 'i', 'f', 0, 0, 0, w, 0, 0, 0, h, 3, 0}
	}
	end := []byte{0, 0, 0, 0, 0, 0, 0, 1}

	// RGB then RGB.
	d := append(hdr(2, 1), 0xFE, 255, 0, 0, 0xFE, 0, 255, 0)
	d = append(d, end...)
	if got := Sniff(d); got != QOI {
		t.Fatalf("Sniff = %v, want QOI", got)
	}
	img, err := Decode(d)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if img.At(0, 0) != red || img.At(1, 0) != green {
		t.Errorf("RGB chunks = %v,%v", img.At(0, 0), img.At(1, 0))
	}

	// RGBA, then RUN (repeat), then INDEX back to the first colour, then DIFF, then LUMA.
	// px0: RGBA(10,20,30,255); px1: RUN 1 → same; px2: INDEX of px0; px3: DIFF; px4: LUMA.
	d2 := append(hdr(5, 1),
		0xFF, 10, 20, 30, 255, // RGBA
		0xC0,                                   // RUN length 1
		0x00|hash(color.RGBA{10, 20, 30, 255}), // INDEX to px0
		0x40|(3<<4)|(2<<2)|1,                   // DIFF dr=+1 dg=0 db=-1
		0x80|34, 0x88,                          // LUMA dg=+2, dr=dg-8+8, db=dg-8+8
	)
	d2 = append(d2, end...)
	img2, err := Decode(d2)
	if err != nil {
		t.Fatalf("Decode2: %v", err)
	}
	if img2.At(1, 0) != img2.At(0, 0) {
		t.Error("RUN did not repeat the pixel")
	}
	if img2.At(2, 0) != img2.At(0, 0) {
		t.Error("INDEX did not recall the pixel")
	}

	// A RUN of length 3 (0xC2): the current pixel plus two carried over, so a later
	// loop iteration enters with run > 0.
	d3 := append(hdr(4, 1), 0xFE, 9, 9, 9, 0xC2)
	d3 = append(d3, end...)
	img3, err := Decode(d3)
	if err != nil {
		t.Fatalf("Decode3: %v", err)
	}
	for x := 0; x < 4; x++ {
		if img3.At(x, 0) != (color.RGBA{9, 9, 9, 255}) {
			t.Errorf("run pixel %d = %v, want grey", x, img3.At(x, 0))
		}
	}
}

func TestQOIErrors(t *testing.T) {
	hdr := func(w, h byte) []byte {
		return []byte{'q', 'o', 'i', 'f', 0, 0, 0, w, 0, 0, 0, h, 3, 0}
	}
	bad := []struct {
		name string
		data []byte
	}{
		{"not qoif", []byte("nope............")},
		{"zero dims", hdr(0, 0)},
		{"truncated stream", hdr(1, 1)},
		// Within the 62-pixels-per-byte bound, yet short: an RGB op is four
		// bytes for one pixel, and the second pixel has no chunk.
		{"stream ends after one of two pixels", append(hdr(2, 1), 0xFE, 1, 2, 3)},
		{"truncated RGB", append(hdr(1, 1), 0xFE, 1, 2)},
		{"truncated RGBA", append(hdr(1, 1), 0xFF, 1, 2, 3)},
		{"truncated LUMA", append(hdr(1, 1), 0x80)},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeQOI(tc.data, DefaultMaxPixels); err == nil {
				t.Errorf("expected an error")
			}
		})
	}
}

// A header must not make the decoder allocate a raster the file cannot hold:
// before the fix, each of these few-byte inputs asked raster.New for gigabytes
// (QOI up to 2^20 x 2^20, 4 TiB of RGBA) before reading a single pixel.
func TestAHeaderLargerThanItsFileIsRefusedBeforeAllocating(t *testing.T) {
	qoi := func(w, h uint32, chunks int) []byte {
		d := []byte{'q', 'o', 'i', 'f', byte(w >> 24), byte(w >> 16), byte(w >> 8), byte(w),
			byte(h >> 24), byte(h >> 16), byte(h >> 8), byte(h), 3, 0}
		return append(d, make([]byte, chunks)...)
	}
	bad := []struct {
		name string
		data []byte
	}{
		{"QOI 2^20 x 2^20, no chunks", qoi(1<<20, 1<<20, 0)},
		{"QOI one pixel past 62 per byte", qoi(62*8+1, 1, 8)},
		{"P5 100000 x 100000", []byte("P5\n100000 100000\n255\n\x00\x00")},
		{"P6 16-bit one byte short", []byte("P6\n1 1\n65535\n\x00\x00\x00\x00\x00")},
		{"P4 two rows, one byte", []byte("P4\n8 2\n\x00")},
		{"P2 more samples than bytes", []byte("P2\n1000 1000\n255\n0 0 0")},
		{"P3 more samples than bytes", []byte("P3\n100000 100000\n255\n0")},
		{"P1 more bits than bytes", []byte("P1\n100000 100000\n1")},
		// 2^32 * 2^32 wraps a 64-bit product to 0: the check must divide.
		{"P6 product wraps to zero", []byte("P6\n4294967296 4294967296\n255\n\x00\x00\x00")},
		// 20 nines overflow the integer parser itself.
		{"P5 width overflows int", []byte("P5\n99999999999999999999 1\n255\n\x00")},
		// MaxInt64: the bitmap row size w+7 wraps negative.
		{"P4 row size wraps", []byte("P4\n9223372036854775807 1\n\x00")},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			_, err := Decode(tc.data)
			runtime.ReadMemStats(&after)
			if err == nil {
				t.Fatalf("expected an error")
			}
			if grew := after.TotalAlloc - before.TotalAlloc; grew > 1<<20 {
				t.Fatalf("allocated %d bytes before refusing a %d-byte input", grew, len(tc.data))
			}
		})
	}
	// The bound is exact, not a guess below it: three RUN-62 chunks (0xFD) are
	// 186 pixels in 3 bytes, the densest a QOI stream can be, and must decode.
	run := append(qoi(62*3, 1, 0), 0xFD, 0xFD, 0xFD)
	if _, err := Decode(run); err != nil {
		t.Fatalf("a QOI image at 62 pixels per chunk byte must still decode: %v", err)
	}
}

// hash mirrors decodeQOI's index hash so a test can craft an INDEX chunk.
func hash(c color.RGBA) byte {
	return byte((int(c.R)*3 + int(c.G)*5 + int(c.B)*7 + int(c.A)*11) % 64)
}

func TestFormatStringNew(t *testing.T) {
	if PNM.String() != "PNM" || QOI.String() != "QOI" {
		t.Errorf("String: PNM=%q QOI=%q", PNM.String(), QOI.String())
	}
}
