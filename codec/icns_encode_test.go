// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// TestAnICNSRoundTripsThroughTheDemux: the write half and the read half of the
// container have to agree, and the read half was here first.
func TestAnICNSRoundTripsThroughTheDemux(t *testing.T) {
	var buf bytes.Buffer
	if err := EncodeICNS(&buf, solid(16, 255, 0, 0), solid(128, 0, 255, 0), solid(512, 0, 0, 255)); err != nil {
		t.Fatalf("EncodeICNS: %v", err)
	}
	if f := Sniff(buf.Bytes()); f != ICNS {
		t.Errorf("Sniff = %v, want ICNS", f)
	}
	reps, err := icnsPNGReps(buf.Bytes())
	if err != nil {
		t.Fatalf("icnsPNGReps: %v", err)
	}
	if len(reps) != 3 {
		t.Errorf("%d representations, want 3", len(reps))
	}
	for _, want := range []int{16, 128, 512} {
		img, err := DecodeBest(buf.Bytes(), want)
		if err != nil {
			t.Fatalf("DecodeBest(%d): %v", want, err)
		}
		if img.W != want {
			t.Errorf("DecodeBest(%d) gave %d pixels", want, img.W)
		}
	}
}

// TestICNSDeclaresTheChunkTypeAppleExpects: a payload under the wrong four
// characters is a file that parses and then shows nothing, because the reader
// looks up the representation by type.
func TestICNSDeclaresTheChunkTypeAppleExpects(t *testing.T) {
	var buf bytes.Buffer
	if err := EncodeICNS(&buf, solid(256, 1, 2, 3)); err != nil {
		t.Fatalf("EncodeICNS: %v", err)
	}
	b := buf.Bytes()
	if got := string(b[:4]); got != "icns" {
		t.Errorf("magic = %q, want icns", got)
	}
	if got := int(binary.BigEndian.Uint32(b[4:8])); got != len(b) {
		t.Errorf("header declares %d bytes, file is %d", got, len(b))
	}
	if got := string(b[8:12]); got != "ic08" {
		t.Errorf("a 256-pixel image went in as %q, want ic08", got)
	}
	// The chunk length counts its own header, which is the part of the format
	// most easily written one way and read the other.
	if got := int(binary.BigEndian.Uint32(b[12:16])); got != len(b)-8 {
		t.Errorf("chunk declares %d bytes, %d remain after the file header", got, len(b)-8)
	}
}

// TestICNSWritesSmallestFirst keeps the file's order predictable, so two runs
// over the same icons produce the same bytes.
func TestICNSWritesSmallestFirst(t *testing.T) {
	var buf bytes.Buffer
	if err := EncodeICNS(&buf, solid(512, 0, 0, 255), solid(16, 255, 0, 0)); err != nil {
		t.Fatalf("EncodeICNS: %v", err)
	}
	if got := string(buf.Bytes()[8:12]); got != "icp4" {
		t.Errorf("first chunk is %q, want the 16-pixel icp4", got)
	}
}

// TestICNSRefusesWhatTheContainerCannotName: the format has no chunk for a
// 48-pixel icon, and storing one under a type that says otherwise makes a file
// that opens and shows the wrong size.
func TestICNSRefusesWhatTheContainerCannotName(t *testing.T) {
	var buf bytes.Buffer
	err := EncodeICNS(&buf, solid(48, 1, 2, 3))
	if err == nil {
		t.Fatal("a 48-pixel icon was accepted")
	}
	if !strings.Contains(err.Error(), "16") {
		t.Errorf("error %q does not say which sizes would work", err)
	}
	if err := EncodeICNS(&buf); err == nil {
		t.Error("an empty container was accepted")
	}
	if err := EncodeICNS(&buf, nil); err == nil {
		t.Error("a nil image was accepted")
	}
	if err := EncodeICNS(&buf, solid(32, 1, 2, 3), solid(32, 3, 2, 1)); err == nil {
		t.Error("the same size twice was accepted")
	}
	wide := solid(32, 1, 2, 3)
	wide.W, wide.H = 64, 16
	if err := EncodeICNS(&buf, wide); err == nil {
		t.Error("a non-square icon was accepted")
	}
}
