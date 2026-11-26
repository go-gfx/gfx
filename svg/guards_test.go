// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package svg

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"runtime"
	"strings"
	"testing"
	"time"
)

// totalAlloc runs f and reports how many bytes it allocated.
func totalAlloc(f func()) uint64 {
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	f()
	runtime.ReadMemStats(&b)
	return b.TotalAlloc - a.TotalAlloc
}

// TestAClipPastTheWorkBudgetStopsPainting: once the work budget is spent the
// clip's pixel loop must not run again for the clip's remaining children. Each
// child here covers the whole surface, so 20,000 of them would cost ~8e10 pixel
// visits; refused after the fifth, the render returns at once.
func TestAClipPastTheWorkBudgetStopsPainting(t *testing.T) {
	doc := `<svg xmlns="http://www.w3.org/2000/svg" width="2000" height="2000"><defs><clipPath id="c">` +
		strings.Repeat(`<rect width="2000" height="2000"/>`, 20000) +
		`</clipPath></defs><rect width="2000" height="2000" clip-path="url(#c)"/></svg>`
	done := make(chan error, 1)
	go func() { _, err := Rasterize(doc, Options{Scale: 1, MaxWork: 1 << 24}); done <- err }()
	select {
	case err := <-done:
		if !errors.Is(err, ErrLimit) {
			t.Fatalf("err = %v, want ErrLimit", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("a clip kept painting after the work budget was spent")
	}
}

// TestARefusedGradientAllocatesNoSurface: a refusal found while collecting the
// definitions comes before the surface is allocated, so a document that asks for
// a 100 MB surface and a bad gradient costs the parse and nothing more.
func TestARefusedGradientAllocatesNoSurface(t *testing.T) {
	doc := `<svg xmlns="http://www.w3.org/2000/svg" width="5000" height="5000"><defs><linearGradient id="g">` +
		strings.Repeat(`<stop offset="0" stop-color="#000"/>`, maxStops+1) +
		`</linearGradient></defs><rect width="9" height="9" fill="url(#g)"/></svg>`
	var err error
	n := totalAlloc(func() { _, err = Rasterize(doc, Options{Scale: 1}) })
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("err = %v, want ErrLimit", err)
	}
	if n > 8<<20 {
		t.Errorf("a refused document allocated %d bytes; the surface was built first", n)
	}
}

// TestAnImagePastTheWorkBudgetIsNotDecoded: the decode touches every source
// pixel, so its cost is charged from the header, before image.Decode allocates.
func TestAnImagePastTheWorkBudgetIsNotDecoded(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 5000, 4000))); err != nil {
		t.Fatal(err)
	}
	doc := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">`+
		`<image width="10" height="10" href="data:image/png;base64,%s"/></svg>`, base64Of(buf.Bytes()))
	var err error
	n := totalAlloc(func() { _, err = Rasterize(doc, Options{Scale: 1}) })
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("err = %v, want ErrLimit", err)
	}
	if n > 8<<20 {
		t.Errorf("a refused image allocated %d bytes; it was decoded first", n)
	}
}
