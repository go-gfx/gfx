// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sort"

	"github.com/go-gfx/gfx/raster"
)

// This file MUXES the Apple .icns container, the write half of the demux in
// icns.go. Like the .ico writer beside it, it writes no pixels: each
// representation is a PNG from the standard library, wrapped in an eight-byte
// chunk header.
//
// Until now the only way to get an .icns out of this fleet was to shell out to
// macOS iconutil, which meant the one format Apple platforms want could not be
// built on the Linux machine that builds everything else.

// icnsChunkFor maps a square pixel size to the chunk type Apple gives it.
//
// These are the "1x" types. A 32-pixel image is ambiguous in the format -- it
// is icp5 at 1x and ic11 as the retina half of a 16 -- and a writer handed one
// image cannot tell which was meant, so it writes the 1x type, which macOS
// reads either way.
var icnsChunkFor = map[int]string{
	16:   "icp4",
	32:   "icp5",
	64:   "icp6",
	128:  "ic07",
	256:  "ic08",
	512:  "ic09",
	1024: "ic10",
}

// icnsSizes lists the sizes the container can name, for error messages that
// say what would have worked.
func icnsSizes() []int {
	out := make([]int, 0, len(icnsChunkFor))
	for s := range icnsChunkFor {
		out = append(out, s)
	}
	sort.Ints(out)
	return out
}

// EncodeICNS writes an .icns container holding every image given.
//
// Like [EncodeICO] this is its own entry point rather than a format for
// [Encode], because an .icns is a set of representations and a function taking
// one image has nowhere to put the rest.
//
// Every image must be square and one of the sizes the format names: 16, 32,
// 64, 128, 256, 512 or 1024. The container has no chunk for anything else, so
// a 48-pixel icon is refused rather than stored under a type that claims it is
// something it is not. Representations are written smallest first, and a size
// given twice is refused: two chunks of one type is a container macOS reads
// unpredictably.
func EncodeICNS(w io.Writer, imgs ...*raster.Image) error {
	if len(imgs) == 0 {
		return fmt.Errorf("codec: icns: nothing to encode")
	}
	type rep struct {
		size    int
		kind    string
		payload []byte
	}
	reps := make([]rep, 0, len(imgs))
	seen := map[int]bool{}
	for i, img := range imgs {
		if img == nil {
			return fmt.Errorf("codec: icns: image %d is nil", i)
		}
		if img.W != img.H {
			return fmt.Errorf("codec: icns: image %d is %dx%d; an icon is square", i, img.W, img.H)
		}
		kind, ok := icnsChunkFor[img.W]
		if !ok {
			return fmt.Errorf("codec: icns: image %d is %d pixels, which the container cannot name; it has chunks for %v",
				i, img.W, icnsSizes())
		}
		if seen[img.W] {
			return fmt.Errorf("codec: icns: image %d repeats the %d-pixel size", i, img.W)
		}
		seen[img.W] = true
		var buf bytes.Buffer
		if err := pngEncode(&buf, nrgba(img)); err != nil {
			return fmt.Errorf("codec: icns: image %d: %w", i, err)
		}
		reps = append(reps, rep{size: img.W, kind: kind, payload: buf.Bytes()})
	}
	sort.Slice(reps, func(a, b int) bool { return reps[a].size < reps[b].size })

	body := &bytes.Buffer{}
	for _, r := range reps {
		body.WriteString(r.kind)
		// The length a chunk declares includes its own eight-byte header.
		binary.Write(body, binary.BigEndian, uint32(len(r.payload)+8))
		body.Write(r.payload)
	}

	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(body.Len()+8))
	out.Write(body.Bytes())
	_, err := w.Write(out.Bytes())
	return err
}
