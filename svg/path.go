// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package svg

import (
	"github.com/go-gfx/gfx/vector"
)

// buildPath parses a "d" string and emits a device-space vector.Path with the
// affine m applied to every coordinate. It returns ok=false when the path uses
// a command this does not know, starts a drawing command with no move, or is
// otherwise malformed, so the caller can skip it without failing the page.
//
// The parser itself is vector.ParseData. It was moved there because the same
// syntax is ODF's svg:d and OpenXPS's abbreviated geometry (ECMA-388 Annex C),
// so a reader of either would otherwise have transcribed the arc maths and the
// smooth-curve reflection a second and a third time. Nothing about SVG was left
// behind: this function is the whole of what was SVG-specific, and that is the
// conversion of the reader's own matrix type.
func buildPath(d string, m matrix) (*vector.Path, bool) {
	return vector.ParseData(d, vector.Affine{m.a, m.b, m.c, m.d, m.e, m.f})
}
