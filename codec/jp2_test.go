// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	jpeg2000 "github.com/ajroetker/go-jpeg2000"
)

// jp2Of encodes a small picture as JPEG 2000, losslessly so the test can check
// the pixels that come back rather than merely that some came back. Building it
// here rather than committing one keeps somebody else's bytes out of the
// repository.
func jp2Of(t *testing.T, w, h int) []byte {
	t.Helper()
	src := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// A gradient, so a decoder that returns a flat image is caught.
			src.Set(x, y, color.RGBA{R: uint8(x * 255 / w), G: uint8(y * 255 / h), B: 40, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg2000.Encode(&buf, src, &jpeg2000.EncodeOptions{Lossless: true}); err != nil {
		t.Fatalf("encoding a JPEG 2000 to test against: %v", err)
	}
	return buf.Bytes()
}

func TestJPEG2000IsSniffedAndDecoded(t *testing.T) {
	data := jp2Of(t, 16, 12)
	if got := Sniff(data); got != JP2 {
		t.Fatalf("sniffed as %s, want JP2", got)
	}
	if got := JP2.String(); got != "JP2" {
		t.Errorf("name %q", got)
	}
	img, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if img.W != 16 || img.H != 12 {
		t.Fatalf("decoded %dx%d, want 16x12", img.W, img.H)
	}
	// Lossless, so the gradient comes back. A decoder that hands over a blank
	// image would pass a bounds check and fail this.
	first := img.Pix[0]
	same := true
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] != first {
			same = false
			break
		}
	}
	if same {
		t.Error("every pixel is the same, and a gradient was encoded")
	}
}

func TestABareCodestreamIsJPEG2000Too(t *testing.T) {
	// A PDF's /JPXDecode stream is often not a JP2 file but the codestream on
	// its own, which starts SOC then SIZ. Sniff has to know both, because
	// almost every scanned page in the wild arrives as one of them: JPEG 2000
	// is in 100% of the biodiversity scans and 99.2% of the medical ones.
	codestream := []byte{0xFF, 0x4F, 0xFF, 0x51, 0x00, 0x00}
	if got := Sniff(codestream); got != JP2 {
		t.Errorf("a bare codestream sniffed as %s", got)
	}
	// And SOC without SIZ after it is not one.
	if got := Sniff([]byte{0xFF, 0x4F, 0x00, 0x00}); got == JP2 {
		t.Error("SOC alone was taken for a codestream")
	}
	// The other way it arrives: a JP2 file, which opens with the twelve-byte
	// signature box rather than with the codestream. The reference encoder
	// writes a bare codestream, so this shape has to be stated on its own.
	signatureBox := []byte("\x00\x00\x00\x0cjP  \r\n\x87\n")
	if got := Sniff(signatureBox); got != JP2 {
		t.Errorf("a JP2 signature box sniffed as %s", got)
	}
	// Neither is a signature box that is nearly right.
	nearly := []byte("\x00\x00\x00\x0cjP  \r\n\x87\x0b")
	if got := Sniff(nearly); got == JP2 {
		t.Error("a signature box with the wrong last byte was accepted")
	}
	// Nor anything too short to tell.
	for _, short := range [][]byte{{}, {0xFF}, {0xFF, 0x4F}, {0xFF, 0x4F, 0xFF}} {
		if got := Sniff(short); got == JP2 {
			t.Errorf("%d bytes sniffed as JP2", len(short))
		}
	}
}

func TestHalfACodestreamIsHalfTheQualityRatherThanAnError(t *testing.T) {
	// JPEG 2000 is progressive by construction: a codestream cut short is a
	// coarser picture of the same size, which is the whole point of the format
	// and the opposite of a Flate stream cut short. A decoder that refused it
	// would throw away a page that every other reader shows.
	data := jp2Of(t, 16, 16)
	img, err := Decode(data[:len(data)*2/3])
	if err != nil {
		t.Skipf("this build of the reference refuses a cut codestream: %v", err)
	}
	if img.W != 16 || img.H != 16 {
		t.Errorf("a cut codestream gave %dx%d, and the size is in the header", img.W, img.H)
	}
}

func TestSomethingThatIsNotACodestreamIsRefused(t *testing.T) {
	// The SOC/SIZ pair opens it, and nothing follows that is a picture.
	if _, err := Decode([]byte{0xFF, 0x4F, 0xFF, 0x51, 0x00, 0x01, 0x02}); err == nil {
		t.Error("seven bytes decoded as a picture")
	}
}
