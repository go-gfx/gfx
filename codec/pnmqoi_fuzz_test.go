// Copyright (c) the go-gfx authors.
// SPDX-License-Identifier: BSD-3-Clause

package codec

import "testing"

// PNM and QOI are the two formats this package parses itself rather than
// handing to a reference decoder, so they are the two it must fuzz. Each must
// return an image or an error on any input: never panic, and never allocate a
// raster larger than the input can describe (the bound checked in decodePNM
// and decodeQOI keeps a fuzz run from being killed for memory).

func FuzzDecodePNM(f *testing.F) {
	for _, s := range []string{
		"P1\n2 1\n1 0",
		"P2\n2 1\n255\n0 128",
		"P3\n1 1\n255\n255 0 0",
		"P4\n8 1\n\xA5",
		"P5\n2 1\n255\n\x00\xFF",
		"P5\n1 1\n65535\n\xFF\xFF",
		"P6\n1 1\n255\n\x01\x02\x03",
		"P6\n4294967296 4294967296\n255\n\x00\x00\x00",
		"P4\n9223372036854775807 1\n\x00",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		img, err := decodePNM(data)
		if err == nil && (img.W <= 0 || img.H <= 0) {
			t.Fatalf("no error and an empty %dx%d image", img.W, img.H)
		}
	})
}

func FuzzDecodeQOI(f *testing.F) {
	hdr := []byte{'q', 'o', 'i', 'f', 0, 0, 0, 2, 0, 0, 0, 1, 3, 0}
	end := []byte{0, 0, 0, 0, 0, 0, 0, 1}
	f.Add(append(append(append([]byte{}, hdr...), 0xFE, 1, 2, 3, 0xC0), end...))
	f.Add(append(append(append([]byte{}, hdr...), 0xFF, 1, 2, 3, 4, 0x40|0x15), end...))
	f.Add([]byte{'q', 'o', 'i', 'f', 0, 0x10, 0, 0, 0, 0x10, 0, 0, 3, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		img, err := decodeQOI(data)
		if err == nil && (img.W <= 0 || img.H <= 0) {
			t.Fatalf("no error and an empty %dx%d image", img.W, img.H)
		}
	})
}
