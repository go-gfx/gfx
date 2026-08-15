// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import "math"

// BlendMode names a separable blend mode from the W3C Compositing and Blending
// Level 1 specification. A separable mode blends each colour channel
// independently: given a backdrop channel Cb and a source channel Cs (both in
// [0, 1]) it produces the blended channel B(Cb, Cs). [BlendMode.Blend] evaluates
// the mode; the formulas match the specification exactly so the results equal the
// published reference values.
type BlendMode int

const (
	// Normal returns the source unchanged (B = Cs); the default painting mode.
	Normal BlendMode = iota
	// Multiply multiplies backdrop and source (B = Cb*Cs); the result is never
	// lighter than either input.
	Multiply
	// Screen is the inverse-multiply complement (B = Cb + Cs - Cb*Cs); never
	// darker than either input.
	Screen
	// Overlay multiplies or screens depending on the backdrop; it is HardLight
	// with the operands swapped.
	Overlay
	// Darken keeps the darker channel (B = min(Cb, Cs)).
	Darken
	// Lighten keeps the lighter channel (B = max(Cb, Cs)).
	Lighten
	// ColorDodge brightens the backdrop toward white in proportion to the
	// source.
	ColorDodge
	// ColorBurn darkens the backdrop toward black in proportion to the source.
	ColorBurn
	// HardLight multiplies or screens depending on the source; it is Overlay
	// with the operands swapped.
	HardLight
	// SoftLight is a gentler HardLight, softening rather than clipping at the
	// extremes.
	SoftLight
	// Difference is the absolute channel difference (B = |Cb - Cs|).
	Difference
	// Exclusion is a lower-contrast Difference (B = Cb + Cs - 2*Cb*Cs).
	Exclusion
)

// Blend returns the blended channel value B(cb, cs) for the mode, with backdrop
// cb and source cs each in [0, 1]. An unrecognised mode falls back to Normal.
func (m BlendMode) Blend(cb, cs float64) float64 {
	switch m {
	case Multiply:
		return cb * cs
	case Screen:
		return screen(cb, cs)
	case Overlay:
		return hardLight(cs, cb)
	case Darken:
		return math.Min(cb, cs)
	case Lighten:
		return math.Max(cb, cs)
	case ColorDodge:
		return colorDodge(cb, cs)
	case ColorBurn:
		return colorBurn(cb, cs)
	case HardLight:
		return hardLight(cb, cs)
	case SoftLight:
		return softLight(cb, cs)
	case Difference:
		return math.Abs(cb - cs)
	case Exclusion:
		return cb + cs - 2*cb*cs
	default: // Normal and any unknown mode
		return cs
	}
}

// screen is the Screen blend of a single channel.
func screen(cb, cs float64) float64 { return cb + cs - cb*cs }

// colorDodge is the W3C ColorDodge channel formula.
func colorDodge(cb, cs float64) float64 {
	switch {
	case cb == 0:
		return 0
	case cs == 1:
		return 1
	default:
		return math.Min(1, cb/(1-cs))
	}
}

// colorBurn is the W3C ColorBurn channel formula.
func colorBurn(cb, cs float64) float64 {
	switch {
	case cb == 1:
		return 1
	case cs == 0:
		return 0
	default:
		return 1 - math.Min(1, (1-cb)/cs)
	}
}

// hardLight is the W3C HardLight channel formula (also Overlay with operands
// swapped).
func hardLight(cb, cs float64) float64 {
	if cs <= 0.5 {
		return 2 * cb * cs
	}
	return screen(cb, 2*cs-1)
}

// softLight is the W3C SoftLight channel formula.
func softLight(cb, cs float64) float64 {
	if cs <= 0.5 {
		return cb - (1-2*cs)*cb*(1-cb)
	}
	var d float64
	if cb <= 0.25 {
		d = ((16*cb-12)*cb + 4) * cb
	} else {
		d = math.Sqrt(cb)
	}
	return cb + (2*cs-1)*(d-cb)
}
