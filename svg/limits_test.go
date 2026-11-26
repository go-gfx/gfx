// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package svg

import (
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// Bomb tests assert a REFUSAL. Each runs under a soft memory limit and a
// deadline, so a regression fails the test instead of eating the host: the
// limit makes the collector fight to stay under it, and the deadline turns a
// hang into a failure with a name.
const (
	bombMemLimit = 1 << 30
	bombDeadline = 30 * time.Second
	// bombAllocCeiling is how much a refused document may allocate in total.
	// A refusal that first built the whole tree has not protected anything.
	bombAllocCeiling = 600 << 20
)

// refused renders doc both ways and requires an ErrLimit naming what, within
// the deadline, allocating less than bombAllocCeiling.
func refused(t *testing.T, doc string, opt Options, what string) {
	t.Helper()
	refusedBy(t, doc, opt, what, "Rasterize", "Walk")
}

// refusedBy is refused for the named entry points only: the work budget has no
// surface to spend on in a Walk.
func refusedBy(t *testing.T, doc string, opt Options, what string, modes ...string) {
	t.Helper()
	old := debug.SetMemoryLimit(bombMemLimit)
	defer debug.SetMemoryLimit(old)
	for _, mode := range modes {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		done := make(chan error, 1)
		go func() {
			var err error
			if mode == "Rasterize" {
				_, err = Rasterize(doc, opt)
			} else {
				_, err = Walk(doc, opt, SinkFunc(func(Shape) {}))
			}
			done <- err
		}()
		select {
		case err := <-done:
			runtime.ReadMemStats(&after)
			if !errors.Is(err, ErrLimit) {
				t.Fatalf("%s: err = %v, want ErrLimit (%s)", mode, err, what)
			}
			var le *LimitError
			if !errors.As(err, &le) || !strings.Contains(le.What, what) {
				t.Fatalf("%s: err = %v, want a LimitError about %q", mode, err, what)
			}
			if got := after.TotalAlloc - before.TotalAlloc; got > bombAllocCeiling {
				t.Errorf("%s: refusal allocated %d MiB, ceiling %d MiB", mode, got>>20, bombAllocCeiling>>20)
			}
		case <-time.After(bombDeadline):
			t.Fatalf("%s: no answer within %v (%s)", mode, bombDeadline, what)
		}
	}
}

// accepted requires doc to render without error.
func accepted(t *testing.T, doc string, opt Options) {
	t.Helper()
	if _, err := Rasterize(doc, opt); err != nil {
		t.Fatalf("Rasterize: %v", err)
	}
	if _, err := Walk(doc, opt, SinkFunc(func(Shape) {})); err != nil {
		t.Fatalf("Walk: %v", err)
	}
}

func wrap(body string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 100 100">` + body + `</svg>`
}

func TestBombDeepNesting(t *testing.T) {
	const n = 100_000
	refused(t, wrap(strings.Repeat("<g>", n)+strings.Repeat("</g>", n)), Options{Scale: 1}, "depth")
}

func TestBombManyElements(t *testing.T) {
	refused(t, wrap(strings.Repeat("<g/>", 1_000_000)), Options{Scale: 1}, "elements")
}

func TestBombHugePathData(t *testing.T) {
	d := "M0 0" + strings.Repeat(" L1 1", 2_100_000) // ~10.5 MB
	if len(d) < 10_000_000 {
		t.Fatalf("test bug: %d bytes", len(d))
	}
	refused(t, wrap(`<path d="`+d+`"/>`), Options{Scale: 1}, "bytes")
}

func TestBombManyPathCommandsUnderTheByteCeiling(t *testing.T) {
	// 6 MiB of commands passes MaxBytes and must still be refused, for its count.
	d := "M0 0" + strings.Repeat("l1 1", 1_600_000)
	refused(t, wrap(`<path d="`+d+`"/>`), Options{Scale: 1}, "path commands")
}

func TestBombHugeDasharray(t *testing.T) {
	d := strings.Repeat("1,", 100_000) + "1"
	refused(t, wrap(`<path d="M0 0L100 100" stroke="black" stroke-dasharray="`+d+`"/>`), Options{Scale: 1}, "dasharray")
}

func TestBombTinyDashOnALongPath(t *testing.T) {
	refused(t, wrap(`<path d="M0 0L100 100" stroke="black" stroke-dasharray="1e-9"/>`), Options{Scale: 1}, "dash runs")
	// And through a transform that stretches a harmless pattern.
	refused(t, wrap(`<path d="M0 0L1000000 0" stroke="black" stroke-dasharray="0.001"/>`), Options{Scale: 1}, "dash runs")
}

func TestBombGradientStops(t *testing.T) {
	stops := strings.Repeat(`<stop offset="0" stop-color="#fff"/>`, 20_000)
	refused(t, wrap(`<defs><linearGradient id="g">`+stops+`</linearGradient></defs><rect width="9" height="9" fill="url(#g)"/>`),
		Options{Scale: 1}, "gradient stops")
}

func TestBombCoordinates(t *testing.T) {
	for name, body := range map[string]string{
		"path":      `<path d="M0 0 C 1e300 0 0 1e300 5 5" fill="black"/>`,
		"transform": `<g transform="scale(1e300)"><rect width="9" height="9"/></g>`,
		"nan":       `<g transform="matrix(NaN 0 0 1 0 0)"><rect width="9" height="9"/></g>`,
		"inf":       `<rect width="Inf" height="9"/>`,
		"strokew":   `<path d="M0 0L5 5" stroke="black" stroke-width="1e300"/>`,
	} {
		t.Run(name, func(t *testing.T) {
			refused(t, wrap(body), Options{Scale: 1}, "")
		})
	}
}

func TestBombSurfaceFromOverflowingFloats(t *testing.T) {
	// On arm64 a float past MaxInt64 converts to MaxInt64 and two of them
	// multiply to 1; the ceiling must see the real product.
	for _, doc := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1e30 1e30"/>`,
		`<svg xmlns="http://www.w3.org/2000/svg" width="1e19" height="1e19"/>`,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 9.3e18 9.3e18"/>`,
	} {
		refused(t, doc, Options{Scale: 1}, "pixels")
	}
	if _, err := Rasterize(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"/>`, Options{Scale: 1e300}); !errors.Is(err, ErrLimit) {
		t.Errorf("huge Scale: err = %v, want ErrLimit", err)
	}
	if _, err := Rasterize(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"/>`, Options{Scale: nan()}); err == nil {
		t.Error("NaN Scale was accepted")
	}
}

func nan() float64 { z := 0.0; return z / z }

func TestBombFullSurfaceRectsExhaustTheWorkBudget(t *testing.T) {
	// 100x100 = 10k pixels; the floor budget is 1<<24, so 5000 full covers
	// (50M) pass the element ceiling and must still be stopped.
	refusedBy(t, wrap(strings.Repeat(`<rect width="100" height="100"/>`, 5000)), Options{Scale: 1}, "work", "Rasterize")
}

func TestBombClipPerTransform(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<defs><clipPath id="c"><rect width="100" height="100"/></clipPath></defs>`)
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&b, `<g transform="translate(%d.5 0)" clip-path="url(#c)"><rect width="1" height="1"/></g>`, i%7)
		fmt.Fprintf(&b, `<g transform="translate(0 %d.25)" clip-path="url(#c)"><rect width="1" height="1"/></g>`, i)
	}
	opt := Options{Scale: 1, MaxWork: 1 << 40} // the work budget must not be the thing that saves us
	old := debug.SetMemoryLimit(bombMemLimit)
	defer debug.SetMemoryLimit(old)
	_, err := Rasterize(wrap(b.String()), opt)
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("err = %v, want ErrLimit", err)
	}
	var le *LimitError
	if !errors.As(err, &le) || !strings.Contains(le.What, "clip mask bytes") {
		t.Fatalf("err = %v, want clip mask bytes", err)
	}
}

func TestBombOpacityLayersAreChargedToo(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 3000; i++ {
		b.WriteString(`<g opacity="0.5"><rect width="1" height="1"/></g>`)
	}
	_, err := Rasterize(wrap(b.String()), Options{Scale: 1, MaxWork: 1 << 18})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("err = %v, want ErrLimit", err)
	}
}

func TestBombManyImages(t *testing.T) {
	// A 3000x3000 claim is under MaxPixels yet 9M pixels per decode: a
	// document may not decode it a thousand times.
	img := base64Of(claimingPNG(t, 3000, 3000))
	body := strings.Repeat(`<image width="9" height="9" href="data:image/png;base64,`+img+`"/>`, 50)
	_, err := Rasterize(wrap(body), Options{Scale: 1, MaxWork: 1 << 20})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("err = %v, want ErrLimit", err)
	}
}

func base64Of(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// A reference cycle must terminate: nothing here resolves <use>, a gradient's
// href or a clip's own clip-path, so a cycle is inert. The test pins that.
func TestReferenceCyclesTerminate(t *testing.T) {
	for name, body := range map[string]string{
		"use":      `<defs><g id="a"><use href="#b"/></g><g id="b"><use href="#a"/></g></defs><use href="#a"/>`,
		"selfuse":  `<g id="a"><use href="#a"/></g>`,
		"gradient": `<defs><linearGradient id="a" href="#b"/><linearGradient id="b" href="#a"/></defs><rect width="9" height="9" fill="url(#a)"/>`,
		"clip":     `<defs><clipPath id="c" clip-path="url(#c)"><rect width="9" height="9" clip-path="url(#c)"/></clipPath></defs><rect width="9" height="9" clip-path="url(#c)"/>`,
		"clipnest": `<defs><clipPath id="a"><g clip-path="url(#b)"><rect width="9" height="9"/></g></clipPath><clipPath id="b"><g clip-path="url(#a)"><rect width="9" height="9"/></g></clipPath></defs><rect width="9" height="9" clip-path="url(#a)"/>`,
		"mask":     `<defs><mask id="m"><rect width="9" height="9" mask="url(#m)"/></mask></defs><rect width="9" height="9" mask="url(#m)"/>`,
	} {
		t.Run(name, func(t *testing.T) {
			done := make(chan error, 1)
			go func() { _, err := Rasterize(wrap(body), Options{Scale: 1}); done <- err }()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("did not terminate")
			}
		})
	}
}

// Each ceiling admits exactly its value and refuses one past it, and an
// option moves it.
func TestLimitsAreExactAndAdjustable(t *testing.T) {
	g := func(n int) string { return strings.Repeat("<g/>", n) }
	nest := func(n int) string { return strings.Repeat("<g>", n) + strings.Repeat("</g>", n) }
	// the root <svg> counts as an element and as depth 1.
	accepted(t, wrap(g(4)), Options{Scale: 1, MaxElements: 5})
	refused(t, wrap(g(5)), Options{Scale: 1, MaxElements: 5}, "elements")
	accepted(t, wrap(nest(4)), Options{Scale: 1, MaxDepth: 5})
	refused(t, wrap(nest(5)), Options{Scale: 1, MaxDepth: 5}, "depth")

	p := func(n int) string { return `<path d="M0 0` + strings.Repeat("L1 1", n-1) + `" fill="black"/>` }
	accepted(t, wrap(p(10)), Options{Scale: 1, MaxSegments: 10})
	refused(t, wrap(p(11)), Options{Scale: 1, MaxSegments: 10}, "path commands")
	// the count is over the whole document, not per shape.
	refused(t, wrap(p(6)+p(6)), Options{Scale: 1, MaxSegments: 10}, "path commands")

	doc := wrap(`<rect width="1" height="1"/>`)
	accepted(t, doc, Options{Scale: 1, MaxBytes: len(doc)})
	refused(t, doc, Options{Scale: 1, MaxBytes: len(doc) - 1}, "bytes")

	stops := func(n int) string {
		return `<defs><linearGradient id="g">` + strings.Repeat(`<stop offset="0" stop-color="#fff"/>`, n) +
			`</linearGradient></defs><rect width="9" height="9" fill="url(#g)"/>`
	}
	accepted(t, wrap(stops(maxStops)), Options{Scale: 1})
	refused(t, wrap(stops(maxStops+1)), Options{Scale: 1}, "gradient stops")

	dash := func(n int) string {
		return `<path d="M0 0L9 9" stroke="black" stroke-dasharray="` + strings.Repeat("1 ", n) + `"/>`
	}
	accepted(t, wrap(dash(maxDashes)), Options{Scale: 1})
	refused(t, wrap(dash(maxDashes+1)), Options{Scale: 1}, "dasharray")
}

func TestLimitErrorText(t *testing.T) {
	if got := (&LimitError{What: "depth", Max: 3}).Error(); got != "svg: depth: more than the 3 allowed" {
		t.Errorf("got %q", got)
	}
	if got := (&LimitError{What: "bytes", Got: 9, Max: 3}).Error(); got != "svg: bytes: 9 is past the 3 allowed" {
		t.Errorf("got %q", got)
	}
	if errors.Is(&LimitError{}, errNoSVG) {
		t.Error("LimitError matched errNoSVG")
	}
}

// A document that is not a bomb is untouched by all of this.
func TestDefaultsAdmitRealisticDocuments(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 20_000; i++ {
		fmt.Fprintf(&b, `<path d="M%d 0 l3 3 l-3 3z" fill="#%06x" stroke="black" stroke-dasharray="1 1"/>`, i%90, i)
	}
	accepted(t, wrap(b.String()), Options{Scale: 1})
}
