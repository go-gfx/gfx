// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import "math"

// This file holds the plain power-law gamma transfer functions (as opposed to
// the piecewise sRGB curve in space.go) and small gamut helpers. A "gamma" here
// is the display exponent γ: encoding raises linear light to 1/γ, decoding
// raises the stored value to γ.

// EncodeGamma encodes a linear-light channel value c (>= 0) with a pure
// power-law of exponent gamma, returning c**(1/gamma).
func EncodeGamma(c, gamma float64) float64 { return math.Pow(c, 1/gamma) }

// DecodeGamma decodes a gamma-encoded channel value c (>= 0) back to linear
// light with a pure power-law of exponent gamma, returning c**gamma, the
// inverse of [EncodeGamma].
func DecodeGamma(c, gamma float64) float64 { return math.Pow(c, gamma) }

// Clamp01 clamps a channel value to the unit range [0, 1].
func Clamp01(c float64) float64 {
	if c < 0 {
		return 0
	}
	if c > 1 {
		return 1
	}
	return c
}

// ClampRGB clamps each channel of an RGB triple to [0, 1] independently. This is
// the crudest gamut mapping: it can shift hue, but never leaves the cube.
func ClampRGB(r, g, b float64) (float64, float64, float64) {
	return Clamp01(r), Clamp01(g), Clamp01(b)
}

// InGamut reports whether every channel of an RGB triple lies within [0, 1] to
// within eps, i.e. whether the colour is displayable without gamut mapping. Pass
// eps = 0 for an exact test or a small value (e.g. 1e-6) to absorb rounding.
func InGamut(r, g, b, eps float64) bool {
	return inRange(r, eps) && inRange(g, eps) && inRange(b, eps)
}

// inRange reports whether v lies in [0-eps, 1+eps].
func inRange(v, eps float64) bool { return v >= -eps && v <= 1+eps }
