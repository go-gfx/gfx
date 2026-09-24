// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package codec

import (
	"bytes"

	"github.com/go-gfx/gfx/raster"
	"github.com/tannevaled/gobig2"
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
// per-decode, so the day it is swapped should be a change in one place.
//
// The decoder is a fork, which is not the usual arrangement here and is meant
// to end. It carries two changes, and both are the same shape: a resource cap
// calibrated on a population that does not include real scanned books, and no
// way for a library to raise it, because the limits are process-global
// variables.
//
//   - the PER-SYMBOL cap, 4 MP, which refuses 7 of 403 streams taken from
//     public scans. Some encoders emit a page-sized region as one symbol.
//     Offered upstream as dkrisman/gobig2#2.
//   - the AGGREGATE cap, 16 MP, which refuses 3 of 866. Those encoders emit
//     thousands of near-duplicate symbols off a noisy scan, so the aggregate
//     is a MULTIPLE of the page rather than a fraction of it -- up to 64 MP
//     for a 6 MP page. Refused, the /Mask that shapes a scanned page's ink
//     layer is dropped and the page is drawn from its background alone: 59%
//     of pixels away from poppler on one of them.
//
// poppler reads all of them at its own defaults. Both caps stay configurable,
// and the seed that motivated the aggregate one -- 198 MP across 538 symbols
// -- is still refused.
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
