// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import "math"

// This file holds the colour-matrix constructions of the W3C Filter Effects
// Module Level 1 (https://www.w3.org/TR/filter-effects-1/): the component-transfer
// and colour-matrix operations behind the CSS `filter` functions brightness(),
// saturate(), grayscale(), sepia(), hue-rotate() and invert(), and the SVG
// feColorMatrix `saturate`/`hueRotate` primitives. They are pure colour-space
// transforms (a saturation matrix mixes each channel towards luminance; a hue
// rotation is a rotation about the luminance axis), independent of any pixel
// buffer — a consumer builds the matrix here and applies it to its own image.

// ColorMatrix is a 3x4 colour transform applied to a gamma-encoded sRGB colour:
// three rows of [cr cg cb offset]. An output channel is
// cr*R + cg*G + cb*B + offset, with R, G, B and the offset in [0, 1]; alpha is
// not touched by these operations, so it is not part of the matrix. A consumer
// working in another numeric range (e.g. 0..255 bytes) scales the offset by its
// full-scale value.
type ColorMatrix [3][4]float64

// Apply transforms one gamma-encoded sRGB colour (channels in [0, 1]) by m,
// returning the three output channels unclamped (a saturation or hue rotation
// can push a channel slightly outside [0, 1]; the caller clamps to its range).
func (m ColorMatrix) Apply(r, g, b float64) (float64, float64, float64) {
	return m[0][0]*r + m[0][1]*g + m[0][2]*b + m[0][3],
		m[1][0]*r + m[1][1]*g + m[1][2]*b + m[1][3],
		m[2][0]*r + m[2][1]*g + m[2][2]*b + m[2][3]
}

// BrightnessMatrix is CSS brightness(a): a per-channel linear multiply by a
// (a >= 0, may exceed 1). It is a scale, not the additive brightness of an
// image-processing "adjust brightness".
func BrightnessMatrix(a float64) ColorMatrix {
	return ColorMatrix{
		{a, 0, 0, 0},
		{0, a, 0, 0},
		{0, 0, a, 0},
	}
}

// InvertMatrix is CSS invert(a): each channel interpolates towards its negative
// by a, out = (1-2a)*in + a. a = 1 is a full photographic negative.
func InvertMatrix(a float64) ColorMatrix {
	d := 1 - 2*a
	return ColorMatrix{
		{d, 0, 0, a},
		{0, d, 0, a},
		{0, 0, d, a},
	}
}

// Rec601Luma is the Rec. 601 luminance coefficients (0.213, 0.715, 0.072) that
// the Filter Effects spec fixes for saturate() and hue-rotate().
var Rec601Luma = [3]float64{0.213, 0.715, 0.072}

// Rec709Luma is the Rec. 709 luminance coefficients (0.2126, 0.7152, 0.0722)
// that the Filter Effects spec fixes for grayscale().
var Rec709Luma = [3]float64{0.2126, 0.7152, 0.0722}

// saturationMatrix is the SVG/CSS saturation matrix for saturation factor s with
// luminance coefficients (lr, lg, lb). s = 1 is identity, s = 0 collapses to
// luminance grey, s > 1 over-saturates.
func saturationMatrix(s, lr, lg, lb float64) ColorMatrix {
	return ColorMatrix{
		{lr + s*(1-lr), lg - s*lg, lb - s*lb, 0},
		{lr - s*lr, lg + s*(1-lg), lb - s*lb, 0},
		{lr - s*lr, lg - s*lg, lb + s*(1-lb), 0},
	}
}

// SaturateMatrix is CSS saturate(s) / SVG feColorMatrix type="saturate": the
// saturation matrix at factor s with Rec. 601 luma. s may exceed 1.
func SaturateMatrix(s float64) ColorMatrix {
	return saturationMatrix(s, Rec601Luma[0], Rec601Luma[1], Rec601Luma[2])
}

// GrayscaleMatrix is CSS grayscale(a): the saturation matrix at s = 1-a with
// Rec. 709 luma, so a interpolates from the original (a = 0) towards full
// luminance grey (a = 1).
func GrayscaleMatrix(a float64) ColorMatrix {
	return saturationMatrix(1-a, Rec709Luma[0], Rec709Luma[1], Rec709Luma[2])
}

// SepiaMatrix is CSS sepia(a): a = 1 is full sepia tone, a = 0 is identity.
func SepiaMatrix(a float64) ColorMatrix {
	t := 1 - a
	return ColorMatrix{
		{0.393 + 0.607*t, 0.769 - 0.769*t, 0.189 - 0.189*t, 0},
		{0.349 - 0.349*t, 0.686 + 0.314*t, 0.168 - 0.168*t, 0},
		{0.272 - 0.272*t, 0.534 - 0.534*t, 0.131 + 0.869*t, 0},
	}
}

// HueRotateMatrix is CSS hue-rotate(rad) / SVG feColorMatrix type="hueRotate":
// a rotation of the colour about the luminance axis by rad radians, built from
// the fixed constant, cosine and sine matrices of the Filter Effects spec.
func HueRotateMatrix(rad float64) ColorMatrix {
	c := math.Cos(rad)
	s := math.Sin(rad)
	return ColorMatrix{
		{0.213 + c*0.787 - s*0.213, 0.715 - c*0.715 - s*0.715, 0.072 - c*0.072 + s*0.928, 0},
		{0.213 - c*0.213 + s*0.143, 0.715 + c*0.285 + s*0.140, 0.072 - c*0.072 - s*0.283, 0},
		{0.213 - c*0.213 - s*0.787, 0.715 - c*0.715 + s*0.715, 0.072 + c*0.928 + s*0.072, 0},
	}
}
