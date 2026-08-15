// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

// This file provides the Y'CbCr luma/chroma encoding used by JPEG and digital
// video, for the two common primary sets: ITU-R BT.601 (SDTV) and BT.709
// (HDTV). The transform is applied to gamma-encoded (primed) R'G'B' and is
// full-range: Y' is in [0, 1] and Cb, Cr are in [0, 1] centred on 0.5 (neutral),
// matching the JFIF/JPEG convention of Go's standard image/color package once
// scaled to 8 bits.

// YCbCr is a full-range Y'CbCr colour: Y is luma in [0, 1]; Cb and Cr are the
// blue- and red-difference chroma in [0, 1], neutral at 0.5.
type YCbCr struct{ Y, Cb, Cr float64 }

// lumaCoeffs returns the BT.601 luma weights (Kr, Kb); G's weight is
// 1 - Kr - Kb.
const (
	kr601 = 0.299
	kb601 = 0.114
	kr709 = 0.2126
	kb709 = 0.0722
)

// rgbToYCbCr applies the full-range Y'CbCr transform with luma weights (kr, kb)
// to gamma sRGB (r, g, b).
func rgbToYCbCr(r, g, b, kr, kb float64) YCbCr {
	y := kr*r + (1-kr-kb)*g + kb*b
	return YCbCr{
		Y:  y,
		Cb: 0.5*(b-y)/(1-kb) + 0.5,
		Cr: 0.5*(r-y)/(1-kr) + 0.5,
	}
}

// yCbCrToRGB inverts [rgbToYCbCr] for luma weights (kr, kb).
func yCbCrToRGB(c YCbCr, kr, kb float64) (r, g, b float64) {
	r = c.Y + (1-kr)/0.5*(c.Cr-0.5)
	b = c.Y + (1-kb)/0.5*(c.Cb-0.5)
	g = (c.Y - kr*r - kb*b) / (1 - kr - kb)
	return
}

// SRGBToYCbCr601 converts a gamma-encoded sRGB colour to full-range Y'CbCr with
// ITU-R BT.601 (SDTV) luma weights.
func SRGBToYCbCr601(r, g, b float64) YCbCr { return rgbToYCbCr(r, g, b, kr601, kb601) }

// YCbCr601ToSRGB converts full-range BT.601 Y'CbCr back to gamma-encoded sRGB,
// the inverse of [SRGBToYCbCr601].
func YCbCr601ToSRGB(c YCbCr) (r, g, b float64) { return yCbCrToRGB(c, kr601, kb601) }

// SRGBToYCbCr709 converts a gamma-encoded sRGB colour to full-range Y'CbCr with
// ITU-R BT.709 (HDTV) luma weights.
func SRGBToYCbCr709(r, g, b float64) YCbCr { return rgbToYCbCr(r, g, b, kr709, kb709) }

// YCbCr709ToSRGB converts full-range BT.709 Y'CbCr back to gamma-encoded sRGB,
// the inverse of [SRGBToYCbCr709].
func YCbCr709ToSRGB(c YCbCr) (r, g, b float64) { return yCbCrToRGB(c, kr709, kb709) }
