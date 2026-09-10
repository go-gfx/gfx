// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"io"
	"strings"
	"testing"

	"github.com/go-gfx/gfx/raster"
)

// solid returns a square filled with one colour, with a transparent pixel in
// the corner so a round trip has some alpha to lose.
func solid(side int, r, g, b uint8) *raster.Image {
	img := raster.New(side, side)
	for i := 0; i < side*side; i++ {
		img.Pix[4*i], img.Pix[4*i+1], img.Pix[4*i+2], img.Pix[4*i+3] = r, g, b, 255
	}
	img.Pix[3] = 0 // top-left corner transparent
	return img
}

// TestAnICORoundTripsThroughTheReferenceReader is the test that matters: the
// container this package writes is read back by the independent library it
// wraps for decoding. Checking our own writer with our own reader would prove
// only that the two agree with each other.
func TestAnICORoundTripsThroughTheReferenceReader(t *testing.T) {
	var buf bytes.Buffer
	if err := EncodeICO(&buf, solid(16, 255, 0, 0), solid(32, 0, 255, 0), solid(256, 0, 0, 255)); err != nil {
		t.Fatalf("EncodeICO: %v", err)
	}
	if f := Sniff(buf.Bytes()); f != ICO {
		t.Errorf("Sniff = %v, want ICO", f)
	}
	img, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	// Decode takes the largest representation, which is the 256 one.
	if img.W != 256 || img.H != 256 {
		t.Errorf("largest representation is %dx%d, want 256x256", img.W, img.H)
	}
}

// TestICOKeepsEveryRepresentation: the point of the container is that a chooser
// has sizes to choose between, so each one has to survive as itself.
func TestICOKeepsEveryRepresentation(t *testing.T) {
	var buf bytes.Buffer
	if err := EncodeICO(&buf, solid(16, 255, 0, 0), solid(48, 0, 255, 0)); err != nil {
		t.Fatalf("EncodeICO: %v", err)
	}
	for _, want := range []int{16, 48} {
		img, err := DecodeBest(buf.Bytes(), want)
		if err != nil {
			t.Fatalf("DecodeBest(%d): %v", want, err)
		}
		if img.W != want {
			t.Errorf("DecodeBest(%d) gave %dx%d", want, img.W, img.H)
		}
	}
}

// TestICOCarriesAlpha: an icon is drawn over whatever is behind it, so losing
// the transparency would put a white or black square on the taskbar.
func TestICOCarriesAlpha(t *testing.T) {
	var buf bytes.Buffer
	if err := EncodeICO(&buf, solid(32, 255, 0, 0)); err != nil {
		t.Fatalf("EncodeICO: %v", err)
	}
	img, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if a := img.Pix[3]; a != 0 {
		t.Errorf("the transparent corner came back with alpha %d, want 0", a)
	}
	if a := img.Pix[4*(32*16+16)+3]; a != 255 {
		t.Errorf("the opaque middle came back with alpha %d, want 255", a)
	}
}

// TestICORefusesWhatTheContainerCannotName: a side above 256 has no encoding in
// a directory entry, and writing it would silently store a 256 or a 0.
func TestICORefusesWhatTheContainerCannotName(t *testing.T) {
	var buf bytes.Buffer
	err := EncodeICO(&buf, solid(512, 255, 0, 0))
	if err == nil {
		t.Fatal("a 512-pixel image was accepted")
	}
	if !strings.Contains(err.Error(), "256") {
		t.Errorf("error %q does not say what the ceiling is", err)
	}
	if err := EncodeICO(&buf); err == nil {
		t.Error("an empty container was accepted")
	}
	if err := EncodeICO(&buf, nil); err == nil {
		t.Error("a nil image was accepted")
	}
	if err := EncodeICO(&buf, raster.New(0, 0)); err == nil {
		t.Error("an image with no pixels was accepted")
	}
}

// TestICOSaysWhichRepresentationWouldNotEncode: the directory is written from
// the payloads, so one payload that will not encode stops the whole container
// rather than leaving an entry pointing at nothing — and the error names the
// representation, because a caller passing eight sizes needs to know which.
func TestICOSaysWhichRepresentationWouldNotEncode(t *testing.T) {
	defer func(f func(io.Writer, image.Image) error) { pngEncode = f }(pngEncode)
	pngEncode = func(w io.Writer, m image.Image) error {
		if m.Bounds().Dx() == 32 {
			return errors.New("no")
		}
		return png.Encode(w, m)
	}
	var buf bytes.Buffer
	err := EncodeICO(&buf, solid(16, 1, 2, 3), solid(32, 1, 2, 3))
	if err == nil {
		t.Fatal("a payload that would not encode was accepted")
	}
	if !strings.Contains(err.Error(), "image 1") {
		t.Errorf("error %q does not name the representation that failed", err)
	}
	if buf.Len() != 0 {
		t.Errorf("%d bytes were written for a container that failed", buf.Len())
	}
}

// TestA256PixelSideIsWrittenAsZero pins the one piece of the format that reads
// like a bug: the side is a single byte, so the largest icon there is names
// itself 0.
func TestA256PixelSideIsWrittenAsZero(t *testing.T) {
	var buf bytes.Buffer
	if err := EncodeICO(&buf, solid(256, 1, 2, 3)); err != nil {
		t.Fatalf("EncodeICO: %v", err)
	}
	b := buf.Bytes()
	if b[6] != 0 || b[7] != 0 {
		t.Errorf("directory entry says %dx%d, want 0x0 standing for 256", b[6], b[7])
	}
}
