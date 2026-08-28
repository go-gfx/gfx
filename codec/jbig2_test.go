// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package codec

import "testing"

// jbig2Segments is a JBIG2 generic region 16 by 8 whose right half is ink. It
// is written out rather than committed as a file, and it is synthetic: no scan
// of anybody's document enters the repository. It was produced by an encoder
// that is not this decoder, and it decodes to the same picture under gobig2,
// under that encoder's own decoder, and under poppler.
var jbig2Segments = []byte{
	0x00, 0x00, 0x00, 0x00, 0x30, 0x00, 0x01, 0x00, 0x00, 0x00, 0x13, 0x00,
	0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x27, 0x00,
	0x01, 0x00, 0x00, 0x00, 0x1e, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00,
	0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x08, 0x03,
	0xff, 0xfd, 0xff, 0x02, 0xfe, 0xfe, 0xfe, 0x8f, 0x66, 0xff, 0xac,
}

// jbig2File puts those segments behind the file header: the eight-byte
// identifier, a flags byte saying the segments are in sequence and the page
// count is known, then that count.
func jbig2File() []byte {
	return append([]byte{0x97, 0x4A, 0x42, 0x32, 0x0D, 0x0A, 0x1A, 0x0A,
		0x01, 0x00, 0x00, 0x00, 0x01}, jbig2Segments...)
}

func TestJBIG2IsSniffedAndDecoded(t *testing.T) {
	data := jbig2File()
	if got := Sniff(data); got != JBIG2 {
		t.Fatalf("sniffed as %s, want JBIG2", got)
	}
	if got := JBIG2.String(); got != "JBIG2" {
		t.Errorf("name %q", got)
	}
	img, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if img.W != 16 || img.H != 8 {
		t.Fatalf("decoded %dx%d, want 16x8", img.W, img.H)
	}
	// Ink on the right, paper on the left, so a decoder handing back a flat or
	// a mirrored picture is caught and not merely one handing back nothing.
	ink := img.Pix[(4*img.W+12)*4]
	paper := img.Pix[(4*img.W+3)*4]
	if ink >= 128 {
		t.Errorf("the inked half came back at %d, which is not ink", ink)
	}
	if paper < 128 {
		t.Errorf("the blank half came back at %d, which is not paper", paper)
	}
}

func TestTheEmbeddedFormCarriesNoSignature(t *testing.T) {
	// What a PDF stores in a /JBIG2Decode stream is the headerless form: the
	// segments alone. It cannot be sniffed and is not meant to be — a PDF
	// consumer learns the format from the image dictionary and decodes it
	// there. Sniffing it as JBIG2 anyway would mean guessing from segment
	// bytes, which is how a registry starts claiming files it cannot read.
	if got := Sniff(jbig2Segments); got == JBIG2 {
		t.Error("the headerless embedded form was sniffed as a JBIG2 file")
	}
}

func TestAJBIG2FileThatIsNotOneIsRefused(t *testing.T) {
	data := append(jbig2File()[:13], 0xde, 0xad, 0xbe, 0xef)
	if got := Sniff(data); got != JBIG2 {
		t.Fatalf("sniffed as %s, want JBIG2", got)
	}
	if _, err := Decode(data); err == nil {
		t.Error("bytes that are not a JBIG2 file decoded anyway")
	}
}
