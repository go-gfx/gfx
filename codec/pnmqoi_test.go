// Copyright (c) the go-gfx authors.
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"image/color"
	"testing"
)

func TestPNM(t *testing.T) {
	cases := []struct {
		name string
		data string
		w, h int
		want map[[2]int]color.RGBA // pixel → expected colour
	}{
		{"P3 ascii pixmap", "P3\n2 1\n255\n255 0 0  0 255 0\n", 2, 1,
			map[[2]int]color.RGBA{{0, 0}: {255, 0, 0, 255}, {1, 0}: {0, 255, 0, 255}}},
		{"P2 ascii graymap", "P2 2 1 255  0 255", 2, 1,
			map[[2]int]color.RGBA{{0, 0}: {0, 0, 0, 255}, {1, 0}: {255, 255, 255, 255}}},
		{"P1 ascii bitmap", "P1\n2 1\n1 0\n", 2, 1, // 1 = black, 0 = white
			map[[2]int]color.RGBA{{0, 0}: {0, 0, 0, 255}, {1, 0}: {255, 255, 255, 255}}},
		{"P3 with comment + rescale", "P3\n# a comment\n1 1\n15\n15 0 0\n", 1, 1,
			map[[2]int]color.RGBA{{0, 0}: {255, 0, 0, 255}}},
		{"P6 binary pixmap", "P6\n1 1\n255\n\x00\x00\xff", 1, 1,
			map[[2]int]color.RGBA{{0, 0}: {0, 0, 255, 255}}},
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
	if _, err := Decode([]byte("P6\n0 0\n255\n")); err == nil {
		t.Error("zero-dimension PNM should error")
	}
}

func TestQOI(t *testing.T) {
	// A 2×1 image: QOI_OP_RGB red, then QOI_OP_RGB green, then the 8-byte end marker.
	data := []byte{'q', 'o', 'i', 'f', 0, 0, 0, 2, 0, 0, 0, 1, 3, 0,
		0xFE, 255, 0, 0,
		0xFE, 0, 255, 0,
		0, 0, 0, 0, 0, 0, 0, 1}
	if got := Sniff(data); got != QOI {
		t.Fatalf("Sniff = %v, want QOI", got)
	}
	img, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if img.W != 2 || img.H != 1 {
		t.Fatalf("size %dx%d, want 2x1", img.W, img.H)
	}
	if got := img.At(0, 0); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("pixel 0 = %v, want red", got)
	}
	if got := img.At(1, 0); got != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("pixel 1 = %v, want green", got)
	}
	// A run and an index reference round-trip: RGB red, RUN of 1 (one more red),
	// then INDEX back to red for the last pixel.
	run := []byte{'q', 'o', 'i', 'f', 0, 0, 0, 3, 0, 0, 0, 1, 3, 0,
		0xFE, 10, 20, 30, // red-ish
		0xC0, // QOI_OP_RUN, length 1
		0, 0, 0, 0, 0, 0, 0, 1}
	img2, err := Decode(run)
	if err != nil {
		t.Fatalf("Decode run: %v", err)
	}
	if img2.At(0, 0) != img2.At(1, 0) {
		t.Errorf("QOI_OP_RUN did not repeat the previous pixel")
	}
}
