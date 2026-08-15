// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import "math"

// This file holds the cylindrical models of gamma-encoded sRGB used by the CSS
// Color specification: HSV (hue/saturation/value, a.k.a. HSB), HSL
// (hue/saturation/lightness) and HWB (hue/whiteness/blackness). They are simple
// geometric re-coordinates of the RGB cube, not perceptual spaces; hue is in
// degrees [0, 360) and the remaining components are in [0, 1]. The formulas
// match the CSS Color Module Level 4 sample conversions.

// HSV is a colour in the hue/saturation/value cylinder of gamma-encoded sRGB. H
// is in degrees [0, 360); S and V are in [0, 1].
type HSV struct{ H, S, V float64 }

// HSL is a colour in the hue/saturation/lightness cylinder of gamma-encoded
// sRGB. H is in degrees [0, 360); S and L are in [0, 1].
type HSL struct{ H, S, L float64 }

// HWB is a colour in the hue/whiteness/blackness model of gamma-encoded sRGB. H
// is in degrees [0, 360); W and B are in [0, 1].
type HWB struct{ H, W, B float64 }

// absf returns the absolute value of x. It is a small, inlinable stand-in for
// math.Abs on the hot cylindrical-colour paths: math.Abs is not inlined, so a
// per-pixel caller pays a call for it, while this folds into the caller. The
// two agree bit-for-bit on every finite input (the only difference is the sign
// of a zero result, which none of these callers observes).
func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// minMax3 returns the minimum and maximum of three values using plain
// comparisons. It replaces the nested math.Min/math.Max calls the cylindrical
// conversions used to make: those functions carry IEEE NaN/signed-zero handling
// that is both slower and not inlinable, whereas for the finite channel values
// these callers pass the plain comparison yields the identical float and folds
// into the caller. Keeping it here means every sRGB->HS* conversion (and every
// fleet consumer of them) gets the faster path at once.
func minMax3(a, b, c float64) (mn, mx float64) {
	mn, mx = a, a
	if b < mn {
		mn = b
	}
	if b > mx {
		mx = b
	}
	if c < mn {
		mn = c
	}
	if c > mx {
		mx = c
	}
	return mn, mx
}

// hueOf returns the CSS hue in degrees [0, 360) for gamma sRGB (r, g, b), given
// the channel maximum mx and range d = mx - min. For an achromatic colour
// (d == 0) the hue is 0.
func hueOf(r, g, b, mx, d float64) float64 {
	if d == 0 {
		return 0
	}
	var h float64
	switch mx {
	case r:
		// (g-b)/d lies in [-1, 1] when r is the maximum, so the modulo-6 wrap the
		// other CSS formulations apply here is a no-op; dropping it keeps the value
		// bit-identical while making the function small enough to inline.
		h = (g - b) / d
	case g:
		h = (b-r)/d + 2
	default: // mx == b
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h
}

// SRGBToHSV converts a gamma-encoded sRGB colour (each channel 0..1) to HSV.
func SRGBToHSV(r, g, b float64) HSV {
	mn, mx := minMax3(r, g, b)
	d := mx - mn
	s := 0.0
	if mx != 0 {
		s = d / mx
	}
	return HSV{H: hueOf(r, g, b, mx, d), S: s, V: mx}
}

// hueToRGB reconstructs gamma sRGB from a hue h (degrees), chroma c and match
// value m (the minimum channel), the shared core of the HSV/HSL/HWB inverses.
func hueToRGB(h, c, m float64) (r, g, b float64) {
	hp := NormalizeHue(h) / 60
	seg := int(hp)
	// math.Mod(hp, 2) computed without the (non-inlinable) call: it equals
	// hp - 2*floor(hp/2), and for the hp in [0, 6) this receives 2*floor(hp/2) is
	// simply seg with its low bit cleared. Exact on these inputs, and it lets the
	// whole function inline into a per-pixel caller.
	x := c * (1 - absf(hp-float64(seg&^1)-1))
	switch seg {
	case 0:
		r, g, b = c, x, 0
	case 1:
		r, g, b = x, c, 0
	case 2:
		r, g, b = 0, c, x
	case 3:
		r, g, b = 0, x, c
	case 4:
		r, g, b = x, 0, c
	default: // 5 and the hp == 6 wrap
		r, g, b = c, 0, x
	}
	return r + m, g + m, b + m
}

// HSVToSRGB converts an HSV colour back to gamma-encoded sRGB, the inverse of
// [SRGBToHSV].
func HSVToSRGB(h HSV) (r, g, b float64) {
	c := h.V * h.S
	return hueToRGB(h.H, c, h.V-c)
}

// SRGBToHSL converts a gamma-encoded sRGB colour (each channel 0..1) to HSL.
func SRGBToHSL(r, g, b float64) HSL {
	mn, mx := minMax3(r, g, b)
	d := mx - mn
	l := (mx + mn) / 2
	s := 0.0
	if d != 0 {
		s = d / (1 - absf(2*l-1))
	}
	return HSL{H: hueOf(r, g, b, mx, d), S: s, L: l}
}

// HSLToSRGB converts an HSL colour back to gamma-encoded sRGB, the inverse of
// [SRGBToHSL].
func HSLToSRGB(h HSL) (r, g, b float64) {
	c := (1 - absf(2*h.L-1)) * h.S
	return hueToRGB(h.H, c, h.L-c/2)
}

// SRGBToHWB converts a gamma-encoded sRGB colour (each channel 0..1) to HWB.
func SRGBToHWB(r, g, b float64) HWB {
	mn, mx := minMax3(r, g, b)
	return HWB{H: hueOf(r, g, b, mx, mx-mn), W: mn, B: 1 - mx}
}

// NormalizeHue folds a hue angle in degrees into the canonical range [0, 360),
// handling negative angles and multiples of 360. It is applied inside the
// hue-based inverses, but is exported for callers building hues arithmetically.
func NormalizeHue(h float64) float64 {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	return h
}

// HWBToSRGB converts an HWB colour back to gamma-encoded sRGB, the inverse of
// [SRGBToHWB]. When whiteness and blackness together fill or overflow the range
// the result is the achromatic grey W/(W+B).
func HWBToSRGB(h HWB) (r, g, b float64) {
	if h.W+h.B >= 1 {
		gray := h.W / (h.W + h.B)
		return gray, gray, gray
	}
	// Take the pure hue at full saturation and value, then scale it into the
	// [W, 1-B] window.
	r, g, b = hueToRGB(h.H, 1, 0)
	span := 1 - h.W - h.B
	return r*span + h.W, g*span + h.W, b*span + h.W
}
