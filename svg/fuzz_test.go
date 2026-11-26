// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package svg

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// FuzzParse feeds arbitrary text to every entry point. The contract under test
// is the one an untrusted-input caller relies on: no panic, no hang, and an
// error that is either "not an SVG" or a *LimitError — never anything else.
//
// The options keep each execution small, so the fuzzer spends its time on
// parser states rather than on allocating surfaces.
func FuzzParse(f *testing.F) {
	if page, err := os.ReadFile("testdata/page.svg"); err == nil {
		f.Add(string(page), uint8(0))
	}
	for _, s := range fuzzSeeds() {
		f.Add(s, uint8(0))
	}
	f.Fuzz(func(t *testing.T, doc string, mode uint8) {
		opt := Options{
			Scale:       []float64{0.5, 1, 2, 7}[mode&3],
			MaxPixels:   1 << 18,
			MaxBytes:    1 << 20,
			MaxElements: 5000,
			MaxDepth:    64,
			MaxSegments: 20_000,
			MaxWork:     1 << 24,
		}
		start := time.Now()
		check := func(what string, err error) {
			var le *LimitError
			switch {
			case err == nil, errors.Is(err, errNoSVG):
			case errors.As(err, &le) && errors.Is(err, ErrLimit):
			default:
				t.Fatalf("%s: unexpected error %T: %v", what, err, err)
			}
		}
		res, err := Rasterize(doc, opt)
		check("Rasterize", err)
		if err == nil && (res == nil || res.Image == nil || len(res.Image.Pix) != res.Image.W*res.Image.H*4) {
			t.Fatalf("Rasterize returned a malformed result")
		}
		_, err = RasterizeMask(doc, opt)
		check("RasterizeMask", err)
		_, err = Walk(doc, opt, SinkFunc(func(Shape) {}))
		check("Walk", err)
		if d := time.Since(start); d > 5*time.Second {
			t.Fatalf("three renders took %v", d)
		}
	})
}

// fuzzSeeds are the documents the bomb tests use, scaled down, plus one
// document per feature the renderer parses.
func fuzzSeeds() []string {
	w := func(b string) string { return wrap(b) }
	return []string{
		w(strings.Repeat("<g>", 300) + strings.Repeat("</g>", 300)),
		w(strings.Repeat("<g/>", 6000)),
		w(`<path d="M0 0` + strings.Repeat("L1 1", 30000) + `"/>`),
		w(`<path d="M0 0L100 100" stroke="black" stroke-dasharray="` + strings.Repeat("1,", 100) + `1"/>`),
		w(`<path d="M0 0L100 100" stroke="black" stroke-dasharray="1e-9"/>`),
		w(`<path d="M0 0 C 1e300 0 0 1e300 5 5" fill="black"/>`),
		w(`<g transform="scale(1e300)"><rect width="9" height="9"/></g>`),
		w(`<g transform="matrix(NaN 0 0 Inf 0 0)"><rect width="9" height="9"/></g>`),
		w(`<defs><linearGradient id="g">` + strings.Repeat(`<stop offset="0" stop-color="#fff"/>`, 300) + `</linearGradient></defs><rect width="9" height="9" fill="url(#g)"/>`),
		w(`<defs><linearGradient id="a" href="#b"/><linearGradient id="b" href="#a"/></defs><rect width="9" height="9" fill="url(#a)"/>`),
		w(`<defs><clipPath id="c" clip-path="url(#c)"><rect width="9" height="9" clip-path="url(#c)"/></clipPath></defs><rect width="9" height="9" clip-path="url(#c)"/>`),
		w(`<defs><g id="a"><use href="#a"/></g></defs><use href="#a"/>`),
		w(`<g opacity="0.5"><g opacity="0.5"><rect width="5" height="5" stroke="red" stroke-width="3" stroke-linejoin="round"/></g></g>`),
		w(`<path d="M10 10 A 1e9 1e-9 33 1 1 90 90 a 0 0 0 0 0 1 1 Z" fill="#369" stroke="#000" stroke-dasharray="3 1 2"/>`),
		w(`<path d="M0 0 Q 50 -1e8 100 0 T 200 0 S 1 1 2 2 H 1e9 V -1e9 z" stroke="black" stroke-width="4"/>`),
		w(`<circle r="1e9"/><ellipse rx="1e-9" ry="1e9"/><rect rx="1e9" width="50" height="50"/><polygon points="0,0 1e9,1e9 -1e9,5"/>`),
		w(`<svg x="1e9" width="1e9" viewBox="0 0 1e-9 1"><rect width="1e9" height="1e9"/></svg>`),
		w(`<image width="1e9" height="9" href="data:image/png;base64,AAAA"/>`),
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1e30 1e30"/>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="1e19" height="1e19"/>`,
		`<!DOCTYPE svg [<!ENTITY a "aaaaaaaaaa"><!ENTITY b "&a;&a;&a;&a;&a;&a;&a;&a;&a;&a;">]><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 9 9"><g id="&b;"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 9 9"><g></svg>`,
		``,
	}
}
