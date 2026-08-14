// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// icnsChunk is one 4-byte-type chunk of a synthetic .icns container.
type icnsChunk struct {
	typ     string
	payload []byte
}

// makeICNS assembles a well-formed .icns container: the "icns" magic, the total
// length, then each chunk as an 8-byte header (type + length) plus its payload.
func makeICNS(chunks ...icnsChunk) []byte {
	var body []byte
	for _, c := range chunks {
		hdr := make([]byte, 8)
		copy(hdr, c.typ)
		binary.BigEndian.PutUint32(hdr[4:], uint32(8+len(c.payload)))
		body = append(body, hdr...)
		body = append(body, c.payload...)
	}
	out := make([]byte, 8+len(body))
	copy(out, "icns")
	binary.BigEndian.PutUint32(out[4:], uint32(len(out)))
	copy(out[8:], body)
	return out
}

func TestDecodeICNS(t *testing.T) {
	small := encodePNG(t, 32, 32)
	large := encodePNG(t, 256, 256)
	// A realistic mix: a raw ARGB chunk (skipped) between two PNG variants.
	data := makeICNS(
		icnsChunk{"ic07", small},
		icnsChunk{"ic08", []byte("ARGB-not-a-png-payload")},
		icnsChunk{"ic10", large},
	)

	// Decode -> the largest PNG representation.
	img, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode icns: %v", err)
	}
	if img.W != 256 {
		t.Errorf("Decode icns = %d, want 256", img.W)
	}

	// DecodeBest picks the smallest representation >= the target.
	best, err := DecodeBest(data, 40)
	if err != nil {
		t.Fatalf("DecodeBest icns: %v", err)
	}
	if best.W != 256 { // 32 is < 40, so 256 is the smallest adequate
		t.Errorf("DecodeBest(40) = %d, want 256", best.W)
	}
	small32, err := DecodeBest(data, 16)
	if err != nil {
		t.Fatalf("DecodeBest icns small: %v", err)
	}
	if small32.W != 32 { // 32 >= 16 and is smaller than 256
		t.Errorf("DecodeBest(16) = %d, want 32", small32.W)
	}
}

func TestICNSErrors(t *testing.T) {
	valid := encodePNG(t, 16, 16)

	t.Run("not-a-container", func(t *testing.T) {
		if _, err := icnsPNGReps([]byte("icnX\x00\x00\x00\x08")); err != errICNSNotContainer {
			t.Fatalf("err = %v, want errICNSNotContainer", err)
		}
		if _, err := icnsPNGReps([]byte("ic")); err != errICNSNotContainer {
			t.Fatalf("short err = %v, want errICNSNotContainer", err)
		}
	})

	t.Run("declared-size-out-of-range", func(t *testing.T) {
		// total field smaller than the 8-byte header.
		tooSmall := append([]byte("icns"), 0x00, 0x00, 0x00, 0x04)
		if _, err := icnsPNGReps(tooSmall); err != errICNSSize {
			t.Fatalf("small total err = %v, want errICNSSize", err)
		}
		// total field larger than the actual buffer.
		tooBig := append([]byte("icns"), 0xFF, 0xFF, 0xFF, 0xFF)
		if _, err := icnsPNGReps(tooBig); err != errICNSSize {
			t.Fatalf("big total err = %v, want errICNSSize", err)
		}
	})

	t.Run("corrupt-chunk", func(t *testing.T) {
		// A chunk whose declared length (2) is below the 8-byte minimum.
		data := make([]byte, 12)
		copy(data, "icns")
		binary.BigEndian.PutUint32(data[4:], 12)
		copy(data[8:], "ic07")
		// length field at [8+4:12] left as a value < 8 would need 16 bytes;
		// build it explicitly instead.
		bad := makeICNS(icnsChunk{"ic07", valid})
		binary.BigEndian.PutUint32(bad[12:], 2) // set the first chunk length to 2 (<8)
		if _, err := icnsPNGReps(bad); err != errICNSCorrupt {
			t.Fatalf("err = %v, want errICNSCorrupt", err)
		}
		// A chunk claiming to run past the declared total.
		over := makeICNS(icnsChunk{"ic07", valid})
		binary.BigEndian.PutUint32(over[12:], uint32(len(over)+100))
		if _, err := icnsPNGReps(over); err != errICNSCorrupt {
			t.Fatalf("overrun err = %v, want errICNSCorrupt", err)
		}
	})

	t.Run("no-png-representation", func(t *testing.T) {
		data := makeICNS(icnsChunk{"ic08", []byte("raw-argb-bytes-only")})
		if _, err := icnsPNGReps(data); err != errICNSNoPNG {
			t.Fatalf("err = %v, want errICNSNoPNG", err)
		}
		if _, err := Decode(data); err != errICNSNoPNG {
			t.Fatalf("Decode err = %v, want errICNSNoPNG", err)
		}
	})

	t.Run("png-header-config-error", func(t *testing.T) {
		// A chunk carrying the PNG signature but no valid IHDR: it is collected
		// as a PNG representation, then png.DecodeConfig fails.
		junk := append(append([]byte(nil), pngMagic...), bytes.Repeat([]byte{0}, 8)...)
		data := makeICNS(icnsChunk{"ic07", junk})
		if _, err := Decode(data); err == nil {
			t.Fatal("expected DecodeConfig error, got nil")
		}
	})

	t.Run("png-decode-error-after-valid-header", func(t *testing.T) {
		// A PNG valid enough for DecodeConfig (signature + IHDR) but truncated
		// before the pixel data, so DecodeConfig succeeds yet Decode fails.
		full := encodePNG(t, 8, 8)
		// Keep the 8-byte signature plus the 25-byte IHDR chunk (len+type+13+crc).
		truncated := full[:8+25]
		data := makeICNS(icnsChunk{"ic07", truncated})
		if _, err := Decode(data); err == nil {
			t.Fatal("expected png.Decode error on truncated body, got nil")
		}
	})
}
