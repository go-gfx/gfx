// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"bytes"
	"errors"
	"testing"

	"github.com/go-gfx/gfx/raster"
)

// twoTone is a picture whose left half is opaque red and whose right half is
// fully transparent: enough to say whether alpha survived, and what replaced it
// when it did not.
//
// It is 32 pixels across rather than 2 because JPEG subsamples chroma. On a
// two-pixel picture the red is averaged with its neighbour and comes back
// 163,36,51, which says something about the format and nothing about this
// code. The samples are taken well inside each half.
func twoTone() *raster.Image {
	const w, h = 32, 32
	img := raster.New(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w/2; x++ {
			i := (y*w + x) * 4
			img.Pix[i], img.Pix[i+3] = 255, 255
		}
	}
	return img
}

// at reads one pixel of a decoded picture.
func at(img *raster.Image, x, y int) (r, g, b, a uint8) {
	i := (y*img.W + x) * 4
	return img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]
}

func TestWhatIsWrittenComesBackTheSame(t *testing.T) {
	// Round-tripping through this package's own reader is the check that
	// matters: an encoder that writes something no reader here accepts has
	// written nothing useful.
	for _, f := range []Format{PNG, JPEG, GIF, TIFF, BMP} {
		t.Run(f.String(), func(t *testing.T) {
			var buf bytes.Buffer
			if err := Encode(&buf, twoTone(), f); err != nil {
				t.Fatalf("encoding: %v", err)
			}
			if got := Sniff(buf.Bytes()); got != f {
				t.Fatalf("what was written sniffs as %s", got)
			}
			back, err := Decode(buf.Bytes())
			if err != nil {
				t.Fatalf("reading it back: %v", err)
			}
			if back.W != 32 || back.H != 32 {
				t.Fatalf("came back %dx%d", back.W, back.H)
			}
			// The red half is red in every one of them. JPEG is lossy, so this
			// asks that it is far more red than anything else rather than
			// exactly 255.
			r, g, b, _ := at(back, 4, 16)
			if r < 200 || g > 80 || b > 80 {
				t.Errorf("the red half came back %d,%d,%d", r, g, b)
			}
		})
	}
}

func TestAlphaSurvivesOnlyWhereItCan(t *testing.T) {
	// PNG and TIFF carry alpha. The others do not, and what they are given is
	// the image on WHITE — chosen here rather than left to the encoder,
	// because an encoder that simply drops the channel puts the colour that
	// was under the transparency on the page, and for a page drawn on
	// transparent ground that is black.
	for _, tc := range []struct {
		f    Format
		want uint8
	}{
		{PNG, 0}, {TIFF, 0}, {JPEG, 255}, {GIF, 255}, {BMP, 255},
	} {
		t.Run(tc.f.String(), func(t *testing.T) {
			var buf bytes.Buffer
			if err := Encode(&buf, twoTone(), tc.f); err != nil {
				t.Fatal(err)
			}
			back, err := Decode(buf.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			r, g, b, a := at(back, 28, 16)
			if a != tc.want {
				t.Errorf("the transparent half came back with alpha %d, want %d", a, tc.want)
			}
			if tc.want == 255 && (r < 200 || g < 200 || b < 200) {
				// And it is white underneath, not black.
				t.Errorf("what replaced the transparency is %d,%d,%d", r, g, b)
			}
		})
	}
}

func TestAFormatThisReadsAndCannotWrite(t *testing.T) {
	// Reading and writing are not symmetric, and the gap is not an oversight:
	// this package writes no encoder of its own, and there is no pure-Go
	// reference encoder for these. Saying so beats writing something in
	// another format under the asked-for name.
	for _, f := range []Format{WEBP, ICO, ICNS, PNM, QOI, JP2, JBIG2, Format(99)} {
		t.Run(f.String(), func(t *testing.T) {
			if CanEncode(f) {
				t.Fatal("it claims to write this")
			}
			var buf bytes.Buffer
			err := Encode(&buf, twoTone(), f)
			if !errors.Is(err, ErrCannotEncode) {
				t.Fatalf("refused with %v", err)
			}
			if buf.Len() != 0 {
				t.Errorf("%d bytes were written anyway", buf.Len())
			}
		})
	}
	for _, f := range []Format{PNG, JPEG, GIF, TIFF, BMP} {
		if !CanEncode(f) {
			t.Errorf("it will not admit to writing %s", f)
		}
	}
}

func TestNothingToEncode(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, nil, PNG); err == nil {
		t.Error("no picture at all was encoded without complaint")
	}
}

// failingWriter is a place to write that will not have it.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("no") }

func TestAPlaceThatWillNotBeWrittenTo(t *testing.T) {
	for _, f := range []Format{PNG, JPEG, GIF, TIFF, BMP} {
		if err := Encode(failingWriter{}, twoTone(), f); err == nil {
			t.Errorf("%s reported success writing nowhere", f)
		}
	}
}

// noisy is a picture that does not compress to nothing. Flat colour flatters
// every encoder equally and would say nothing about which of them compresses.
func noisy(w, h int) *raster.Image {
	img := raster.New(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			v := uint8(255)
			if (x/3+y/5)%17 == 0 || (x*y)%2003 < 40 {
				v = uint8((x * y) % 256)
			}
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = v, v, v, 255
		}
	}
	return img
}

func TestATIFFIsCompressed(t *testing.T) {
	// The encoder's default is no compression at all, and nil reads like "no
	// options" rather than like a choice. On a page-sized picture that is the
	// difference between eight megabytes and four hundred kilobytes.
	img := noisy(612, 792)
	var got bytes.Buffer
	if err := Encode(&got, img, TIFF); err != nil {
		t.Fatal(err)
	}
	raw := 4 * img.W * img.H // what the pixels weigh uncompressed
	if got.Len() > raw/4 {
		t.Errorf("a TIFF of %d pixels came to %d bytes, which is not compressed at all",
			img.W*img.H, got.Len())
	}
	// And it still reads back, which is the half that matters: a smaller file
	// nothing opens is worse than a large one.
	back, err := Decode(got.Bytes())
	if err != nil {
		t.Fatalf("the compressed TIFF cannot be read back: %v", err)
	}
	if back.W != img.W || back.H != img.H {
		t.Fatalf("it came back %dx%d", back.W, back.H)
	}
	// Losslessly: TIFF is not a lossy format, so every pixel is the one that
	// went in.
	for i := range img.Pix {
		if back.Pix[i] != img.Pix[i] {
			t.Fatalf("pixel byte %d came back %d, want %d", i, back.Pix[i], img.Pix[i])
		}
	}
}

func TestWhichFormatsAreLargeAndStayLarge(t *testing.T) {
	// BMP has no compression to ask for, and this says so out loud rather than
	// leaving somebody to discover it on a document of two hundred pages.
	img := noisy(200, 200)
	sizes := map[Format]int{}
	for _, f := range []Format{PNG, TIFF, BMP} {
		var buf bytes.Buffer
		if err := Encode(&buf, img, f); err != nil {
			t.Fatal(err)
		}
		sizes[f] = buf.Len()
	}
	raw := 4 * img.W * img.H
	if sizes[BMP] < raw/2 {
		t.Errorf("BMP came to %d bytes for %d of pixels: it has learned to compress, "+
			"and what this package says about it is now wrong", sizes[BMP], raw)
	}
	if sizes[TIFF] >= sizes[BMP] {
		t.Errorf("TIFF (%d) is no smaller than BMP (%d)", sizes[TIFF], sizes[BMP])
	}
}
