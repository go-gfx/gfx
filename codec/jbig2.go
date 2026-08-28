// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"bytes"

	"github.com/dkrisman/gobig2"
	"github.com/go-gfx/gfx/raster"
)

// DecodeEmbeddedJBIG2 decodes the headerless form of JBIG2 — segments and
// nothing else, with the shared ones handed in separately — which is what a
// container embeds when its own metadata already says what the bytes are.
//
// It sits outside [Decode]'s contract deliberately. Every other format here is
// found by sniffing, and this one cannot be: the embedded form carries no
// signature, and the globals cannot be guessed from the stream at all. Passing
// it through [Sniff] would mean pretending to recognise something.
//
// It is here rather than in each container's own reader so that one package
// names the JBIG2 decoder. That matters more than usual for this format: the
// reference decoder's resource limits are process-global rather than
// per-decode, and it publishes no tagged version, so the day it is swapped
// should be a change in one place.
//
// globals may be nil, which is the common case: an encoder that puts a page's
// symbol dictionary in the page's own stream needs no shared segments.
func DecodeEmbeddedJBIG2(data, globals []byte) (*raster.Image, error) {
	d, err := gobig2.NewDecoderEmbedded(bytes.NewReader(data), globals)
	if err != nil {
		return nil, err
	}
	img, err := d.Decode()
	if err != nil {
		return nil, err
	}
	return raster.FromImage(img), nil
}
