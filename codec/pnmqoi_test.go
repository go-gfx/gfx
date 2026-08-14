// Copyright (c) the go-gfx authors.
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"image/color"
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
			if _, err := decodePNM([]byte(tc.data)); err == nil {
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
		{"truncated RGB", append(hdr(1, 1), 0xFE, 1, 2)},
		{"truncated RGBA", append(hdr(1, 1), 0xFF, 1, 2, 3)},
		{"truncated LUMA", append(hdr(1, 1), 0x80)},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeQOI(tc.data); err == nil {
				t.Errorf("expected an error")
			}
		})
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
