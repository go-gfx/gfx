// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package svg

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

// Every resource a document controls has a ceiling, and a document that passes
// one is REFUSED with a [*LimitError] — never truncated, never drawn "as far as
// it got". A silently clipped logo is a wrong logo, and nothing tells the caller.
//
// ⛔ The defaults are generous for real artwork (an icon is a few hundred
// elements, a large map or chart a few tens of thousands) and small enough that
// a hostile file costs a bounded amount of memory and time. A service that takes
// documents from strangers should still set them to what its own pages need.
const (
	// DefaultMaxBytes is the largest document text accepted.
	DefaultMaxBytes = 8 << 20
	// DefaultMaxElements is the most XML elements one document may hold.
	DefaultMaxElements = 100_000
	// DefaultMaxDepth is the deepest element nesting accepted. The renderer
	// recurses once per level and copies its inherited state at each, so depth is
	// both stack and, by the group lists, quadratic memory.
	DefaultMaxDepth = 256
	// DefaultMaxSegments is the most path commands (moves, lines, curves, closes
	// — an arc is several) summed over every shape of one document.
	DefaultMaxSegments = 1_000_000

	// defaultMinWork is the floor of the default work budget; see [Options.MaxWork].
	defaultMinWork = 1 << 24
	// defaultWorkSurfaces is how many full-surface paints the default budget allows.
	defaultWorkSurfaces = 64

	// maxStops is the most <stop> elements one gradient may have.
	maxStops = 256
	// maxDashes is the most entries a stroke-dasharray may have.
	maxDashes = 64
	// maxCoord is the largest device-space coordinate, stroke width or length a
	// shape may use, in pixels. Curves are flattened by bisection, and the number
	// of pieces grows with the size of the curve: a control point at 1e300 asks
	// for millions of vertices from ONE command. Nothing drawn onto a surface of
	// at most DefaultMaxPixels needs a point ten million pixels away.
	maxCoord = 1e7
	// maxDashRuns is the most dashes one stroke may be cut into.
	maxDashRuns = 1 << 20
)

// ErrLimit is what a [*LimitError] matches under [errors.Is]: the document is
// well formed but asks for more than the configured ceilings allow.
var ErrLimit = errors.New("svg: document exceeds a limit")

// LimitError reports which ceiling a document passed.
type LimitError struct {
	What string // the resource: "bytes", "elements", "depth", "pixels", ...
	Got  int64  // the amount asked for when it was noticed; 0 when only "more than Max" is known
	Max  int64  // the ceiling
}

// Error implements error.
func (e *LimitError) Error() string {
	if e.Got > 0 {
		return fmt.Sprintf("svg: %s: %d is past the %d allowed", e.What, e.Got, e.Max)
	}
	return fmt.Sprintf("svg: %s: more than the %d allowed", e.What, e.Max)
}

// Is reports whether target is [ErrLimit].
func (e *LimitError) Is(target error) bool { return target == ErrLimit }

// limits is the resolved set of ceilings for one run.
type limits struct {
	bytes, elements, depth, segments, work int64
}

func (o Options) limits(w, h int) limits {
	l := limits{
		bytes:    int64(o.MaxBytes),
		elements: int64(o.MaxElements),
		depth:    int64(o.MaxDepth),
		segments: int64(o.MaxSegments),
		work:     int64(o.MaxWork),
	}
	if l.bytes <= 0 {
		l.bytes = DefaultMaxBytes
	}
	if l.elements <= 0 {
		l.elements = DefaultMaxElements
	}
	if l.depth <= 0 {
		l.depth = DefaultMaxDepth
	}
	if l.segments <= 0 {
		l.segments = DefaultMaxSegments
	}
	if l.work <= 0 {
		l.work = max(defaultWorkSurfaces*int64(w)*int64(h), defaultMinWork)
	}
	return l
}

// prescan reads the document's element structure without building anything,
// so the size, count and depth ceilings are enforced BEFORE the tree is held.
//
// ⛔ encoding/xml's own Unmarshal limits depth to 10000, which is far past what
// this renderer's recursion should be asked to carry, and has no limit on
// element count at all. The decoder never expands entities beyond the five
// predefined ones (and it does not fetch DTDs), so there is no entity bomb here.
func prescan(doc string, l limits) error {
	if int64(len(doc)) > l.bytes {
		return &LimitError{What: "bytes", Got: int64(len(doc)), Max: l.bytes}
	}
	d := xml.NewDecoder(strings.NewReader(doc))
	var depth, n int64
	for {
		tok, err := d.RawToken()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errNoSVG
		}
		switch tok.(type) {
		case xml.StartElement:
			n++
			depth++
			if n > l.elements {
				return &LimitError{What: "elements", Max: l.elements}
			}
			if depth > l.depth {
				return &LimitError{What: "depth", Max: l.depth}
			}
		case xml.EndElement:
			depth--
		}
	}
}

// fail records the first ceiling passed; the walk stops at the next element.
func (r *renderer) fail(what string, got, max int64) {
	if r.err == nil {
		r.err = &LimitError{What: what, Got: got, Max: max}
	}
}

// charge adds n pixel operations to the work done and fails past the budget.
func (r *renderer) charge(n int64) {
	r.work += n
	if r.work > r.lim.work {
		r.fail("work", 0, r.lim.work)
	}
}

// finite reports whether every value is a number within ±maxCoord.
func finite(v ...float64) bool {
	for _, x := range v {
		if !(math.Abs(x) <= maxCoord) { // false for NaN as well
			return false
		}
	}
	return true
}
