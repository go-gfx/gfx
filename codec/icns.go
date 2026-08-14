// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image/png"

	"github.com/go-gfx/gfx/raster"
)

// This file demuxes the Apple .icns icon container. It is the one format the
// registry has no clean pure-Go DECODE reference to wrap, so — like go-ico
// around the .ico container — it unpacks the container here and hands the actual
// pixel decoding to the standard library's image/png. NO image decoder is
// reimplemented.
//
// VERDICT (2026-08-14, control-run): no pure-Go icns library suits a robust
// Decode registry.
//   - github.com/jackmordaunt/icns/v3 is encode-first; its Decode/DecodeAll walk
//     the container with UNCHECKED slice indexing and PANIC on malformed input
//     (proven: a truncated container and a chunk with dataSize<8 both panic with
//     "slice bounds out of range"), which is unacceptable for a Decode entry
//     point fed arbitrary bytes. It also drags the deprecated, archived
//     github.com/nfnt/resize into the graph (needed only for its encoder) and
//     mutates the global image format registry in init.
//   - yrh.dev/icns exposes only a PNG/JPEG subset behind a bespoke *ICNS type
//     rather than image.Image, needing an adapter and still not covering the
//     container robustly.
// So this bounds-checked demux (returning errors, never panicking) lives here.
// It is a container walk plus the standard image/png decoder — the same
// "unpack + reference decoder" shape as the rest of the registry, not a
// reinvented image codec. It subsumes the equivalent hand-rolled unpacker in
// go-widgets/desktop.
//
// Modern .icns files store each retina/size variant as an embedded PNG (chunk
// types ic07/ic08/ic09/ic10/ic11/ic12/ic13/ic14); some legacy variants are raw
// ARGB or JPEG-2000, which this demux skips because the standard library cannot
// decode them and the registry needs a decodable representation.

var (
	errICNSNotContainer = errors.New("icns: not an icns container")
	errICNSSize         = errors.New("icns: declared size out of range")
	errICNSCorrupt      = errors.New("icns: corrupt chunk")
	errICNSNoPNG        = errors.New("icns: no PNG representation")
)

// pngMagic is the 8-byte PNG file signature.
var pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// icnsPNGReps returns the encoded-PNG payload of every PNG representation in an
// .icns container, in file order. Non-PNG chunks (raw ARGB, JPEG-2000, masks,
// the table of contents) are skipped. It errors when the input is not an icns
// container, is structurally corrupt, or carries no PNG variant. Pixel decoding
// is left to image/png; this only unpacks the container.
func icnsPNGReps(data []byte) ([][]byte, error) {
	if len(data) < 8 || string(data[:4]) != "icns" {
		return nil, errICNSNotContainer
	}
	total := int(binary.BigEndian.Uint32(data[4:8]))
	if total < 8 || total > len(data) {
		return nil, errICNSSize
	}
	var reps [][]byte
	for off := 8; off+8 <= total; {
		length := int(binary.BigEndian.Uint32(data[off+4 : off+8]))
		if length < 8 || off+length > total {
			return nil, errICNSCorrupt
		}
		payload := data[off+8 : off+length]
		if bytes.HasPrefix(payload, pngMagic) {
			reps = append(reps, payload)
		}
		off += length
	}
	if len(reps) == 0 {
		return nil, errICNSNoPNG
	}
	return reps, nil
}

// decodeICNS demuxes an .icns container, selects the PNG representation best
// matching targetSize (via pickIndex over each representation's PNG-declared
// pixel size), and decodes it with the standard library.
func decodeICNS(data []byte, targetSize int) (*raster.Image, error) {
	reps, err := icnsPNGReps(data)
	if err != nil {
		return nil, err
	}
	sizes := make([]dim, len(reps))
	for i, rep := range reps {
		cfg, err := png.DecodeConfig(bytes.NewReader(rep))
		if err != nil {
			return nil, err
		}
		sizes[i] = dim{cfg.Width, cfg.Height}
	}
	chosen := reps[pickIndex(sizes, targetSize)]
	img, err := png.Decode(bytes.NewReader(chosen))
	if err != nil {
		return nil, err
	}
	return raster.FromImage(img), nil
}
