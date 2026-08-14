// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	ico "github.com/sergeymakinen/go-ico"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

// gradient builds a synthetic w*h straight-alpha NRGBA test image. Its content
// is a deterministic colour ramp (neutral, generated in-test — never personal
// data), so lossless round trips can be checked pixel for pixel.
func gradient(w, h int) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m.Set(x, y, color.NRGBA{uint8(x * 17), uint8(y * 23), 0x80, 0xff})
		}
	}
	return m
}

// encodePNG returns the PNG encoding of a synthetic w*h gradient.
func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, gradient(w, h)); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

// webpBase64 is a synthetic 6x4 lossless WebP gradient. golang.org/x/image has
// no WebP encoder, so these bytes were produced once from the same kind of
// generated gradient and embedded verbatim (neutral test content). The test
// below proves the reference decoder we ship decodes them.
const webpBase64 = "UklGRooBAABXRUJQVlA4TH4BAAAvBcAAAE1kRP/DRYBMMwAAAAAAAAAcAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACA4AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABBAJAMAAAAAAgAAAAAAAAAAAAAAAAAAAAAAAMAAAAAAAAAAEEAkAwAAAAAAAAAAAAAAAAAAAAIAAAAAAAAAwAAAAAAAAACICRDq/gAA"

// webpBytes decodes the embedded synthetic WebP fixture.
func webpBytes(t *testing.T) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(webpBase64)
	if err != nil {
		t.Fatalf("webp base64: %v", err)
	}
	return b
}

func TestFormatString(t *testing.T) {
	cases := []struct {
		f    Format
		want string
	}{
		{PNG, "PNG"}, {JPEG, "JPEG"}, {GIF, "GIF"}, {WEBP, "WEBP"},
		{TIFF, "TIFF"}, {BMP, "BMP"}, {ICO, "ICO"}, {ICNS, "ICNS"},
		{Unknown, "unknown"}, {Format(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.f.String(); got != c.want {
			t.Errorf("Format(%d).String() = %q, want %q", c.f, got, c.want)
		}
	}
}

func TestSniff(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want Format
	}{
		{"png", []byte("\x89PNG\r\n\x1a\n....."), PNG},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, JPEG},
		{"gif87", []byte("GIF87a...."), GIF},
		{"gif89", []byte("GIF89a...."), GIF},
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8L"), WEBP},
		{"tiff-le", []byte("II*\x00...."), TIFF},
		{"tiff-be", []byte("MM\x00*...."), TIFF},
		{"bmp", []byte("BM........"), BMP},
		{"ico", []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00}, ICO},
		{"icns", []byte("icns\x00\x00\x00\x08"), ICNS},
		{"empty", nil, Unknown},
		{"short", []byte{0x89}, Unknown},
		{"noise", []byte("not an image at all"), Unknown},
	}
	for _, c := range cases {
		if got := Sniff(c.data); got != c.want {
			t.Errorf("Sniff(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestDecodePNGRoundTrip proves the whole pipeline: sniff -> reference decode ->
// raster.Image, pixel-exact against the source for a lossless format.
func TestDecodePNGRoundTrip(t *testing.T) {
	src := gradient(5, 3)
	img, err := Decode(encodePNG(t, 5, 3))
	if err != nil {
		t.Fatalf("Decode png: %v", err)
	}
	if img.W != 5 || img.H != 3 {
		t.Fatalf("size = %dx%d, want 5x3", img.W, img.H)
	}
	for y := 0; y < 3; y++ {
		for x := 0; x < 5; x++ {
			got := img.At(x, y)
			want := src.NRGBAAt(x, y)
			if got.R != want.R || got.G != want.G || got.B != want.B || got.A != want.A {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

// TestDecodeSingleFormats round-trips every single-image reference format,
// asserting the decoded dimensions (lossy formats can shift colour, so only the
// shape is asserted for those).
func TestDecodeSingleFormats(t *testing.T) {
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, gradient(8, 6), nil); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	var g bytes.Buffer
	if err := gif.Encode(&g, gradient(8, 6), nil); err != nil {
		t.Fatalf("gif encode: %v", err)
	}
	var tif bytes.Buffer
	if err := tiff.Encode(&tif, gradient(8, 6), nil); err != nil {
		t.Fatalf("tiff encode: %v", err)
	}
	var bm bytes.Buffer
	if err := bmp.Encode(&bm, gradient(8, 6)); err != nil {
		t.Fatalf("bmp encode: %v", err)
	}
	cases := []struct {
		name       string
		data       []byte
		wantW, wtH int
	}{
		{"jpeg", jpg.Bytes(), 8, 6},
		{"gif", g.Bytes(), 8, 6},
		{"tiff", tif.Bytes(), 8, 6},
		{"bmp", bm.Bytes(), 8, 6},
		{"webp", webpBytes(t), 6, 4},
	}
	for _, c := range cases {
		img, err := Decode(c.data)
		if err != nil {
			t.Fatalf("Decode %s: %v", c.name, err)
		}
		if img.W != c.wantW || img.H != c.wtH {
			t.Errorf("%s size = %dx%d, want %dx%d", c.name, img.W, img.H, c.wantW, c.wtH)
		}
	}
}

func TestDecodeUnknownFormat(t *testing.T) {
	if _, err := Decode([]byte("this is plainly not an image")); err != ErrUnknownFormat {
		t.Fatalf("err = %v, want ErrUnknownFormat", err)
	}
}

// TestDecodeCorruptSingle exercises decodeSingle's error branch: valid magic,
// invalid body, so the reference decoder reports an error the registry returns.
func TestDecodeCorruptSingle(t *testing.T) {
	// PNG signature followed by garbage instead of an IHDR chunk.
	corrupt := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 16)...)
	if _, err := Decode(corrupt); err == nil {
		t.Fatal("expected error decoding corrupt png, got nil")
	}
}

// makeICO encodes a synthetic multi-size .ico from generated gradients.
func makeICO(t *testing.T, sizes ...int) []byte {
	t.Helper()
	imgs := make([]image.Image, len(sizes))
	for i, s := range sizes {
		imgs[i] = gradient(s, s)
	}
	var buf bytes.Buffer
	if err := ico.EncodeAll(&buf, imgs); err != nil {
		t.Fatalf("ico encode: %v", err)
	}
	return buf.Bytes()
}

func TestDecodeICO(t *testing.T) {
	data := makeICO(t, 16, 32, 48)

	// Decode -> largest representation.
	img, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode ico: %v", err)
	}
	if img.W != 48 {
		t.Errorf("Decode ico largest = %d, want 48", img.W)
	}

	// DecodeBest picks the smallest representation >= target.
	best, err := DecodeBest(data, 20)
	if err != nil {
		t.Fatalf("DecodeBest ico: %v", err)
	}
	if best.W != 32 {
		t.Errorf("DecodeBest(20) = %d, want 32", best.W)
	}

	// A target above every representation falls back to the largest.
	big, err := DecodeBest(data, 1000)
	if err != nil {
		t.Fatalf("DecodeBest ico big: %v", err)
	}
	if big.W != 48 {
		t.Errorf("DecodeBest(1000) = %d, want 48", big.W)
	}
}

func TestDecodeCorruptICO(t *testing.T) {
	// ICO magic but a truncated / invalid directory.
	corrupt := []byte{0x00, 0x00, 0x01, 0x00, 0xFF, 0xFF}
	if _, err := Decode(corrupt); err == nil {
		t.Fatal("expected error decoding corrupt ico, got nil")
	}
}

func TestDimLong(t *testing.T) {
	if got := (dim{4, 9}).long(); got != 9 { // h > w
		t.Errorf("dim{4,9}.long() = %d, want 9", got)
	}
	if got := (dim{7, 3}).long(); got != 7 { // w >= h
		t.Errorf("dim{7,3}.long() = %d, want 7", got)
	}
	if got := (dim{5, 5}).long(); got != 5 { // equal
		t.Errorf("dim{5,5}.long() = %d, want 5", got)
	}
}

func TestPickIndex(t *testing.T) {
	// Ascending sizes so the largest-search update branch runs each step.
	asc := []dim{{16, 16}, {32, 32}, {64, 64}}
	if got := pickIndex(asc, 0); got != 2 { // largest
		t.Errorf("pickIndex(asc, 0) = %d, want 2", got)
	}
	if got := pickIndex(asc, 20); got != 1 { // smallest >= 20
		t.Errorf("pickIndex(asc, 20) = %d, want 1", got)
	}
	if got := pickIndex(asc, 1000); got != 2 { // none adequate -> largest
		t.Errorf("pickIndex(asc, 1000) = %d, want 2", got)
	}
	// Descending order: exact-target match, largest is index 0.
	desc := []dim{{64, 64}, {32, 32}, {16, 16}}
	if got := pickIndex(desc, 32); got != 1 {
		t.Errorf("pickIndex(desc, 32) = %d, want 1", got)
	}
	// Single representation.
	if got := pickIndex([]dim{{8, 8}}, 4); got != 0 {
		t.Errorf("pickIndex(single, 4) = %d, want 0", got)
	}
}
