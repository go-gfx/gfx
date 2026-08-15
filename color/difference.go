// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import "math"

// This file implements the standard perceptual colour-difference metrics
// (ΔE, "delta E"): the size of the perceived difference between two colours.
// DeltaE76 is the plain Euclidean distance in CIE L*a*b*; DeltaE94 and the
// CIEDE2000 DeltaE2000 add lightness/chroma/hue weighting that tracks human
// vision far better; DeltaEOK is the Euclidean distance in the perceptually
// uniform OKLab space. A ΔE of about 1 is the just-noticeable difference.

// DeltaE76 is the CIE 1976 colour difference: the Euclidean distance between two
// L*a*b* colours.
func DeltaE76(a, b Lab) float64 {
	dl := a.L - b.L
	da := a.A - b.A
	db := a.B - b.B
	return math.Sqrt(dl*dl + da*da + db*db)
}

// DeltaE94 is the CIE 1994 colour difference with the graphic-arts weighting
// (kL = kC = kH = 1, K1 = 0.045, K2 = 0.015). The metric is asymmetric: ref is
// the reference (standard) colour and sample the trial colour.
func DeltaE94(ref, sample Lab) float64 {
	const k1, k2 = 0.045, 0.015
	c1 := math.Hypot(ref.A, ref.B)
	c2 := math.Hypot(sample.A, sample.B)
	dl := ref.L - sample.L
	dc := c1 - c2
	da := ref.A - sample.A
	db := ref.B - sample.B
	// dH^2 = da^2 + db^2 - dC^2, clamped against tiny negative rounding.
	dh2 := math.Max(0, da*da+db*db-dc*dc)
	sc := 1 + k1*c1
	sh := 1 + k2*c1
	return math.Sqrt(dl*dl + (dc/sc)*(dc/sc) + dh2/(sh*sh))
}

// atan2Deg returns atan2(y, x) in degrees, normalised to [0, 360).
func atan2Deg(y, x float64) float64 {
	d := math.Atan2(y, x) * 180 / math.Pi
	if d < 0 {
		d += 360
	}
	return d
}

func deg2rad(d float64) float64 { return d * math.Pi / 180 }

// DeltaE2000 is the CIEDE2000 colour difference between two L*a*b* colours, with
// the standard weighting factors kL = kC = kH = 1. It follows the reference
// formulation of Sharma, Wu and Dalal (2005).
func DeltaE2000(c1, c2 Lab) float64 {
	cbar := (math.Hypot(c1.A, c1.B) + math.Hypot(c2.A, c2.B)) / 2
	cbar7 := math.Pow(cbar, 7)
	g := 0.5 * (1 - math.Sqrt(cbar7/(cbar7+pow25_7)))

	a1p := (1 + g) * c1.A
	a2p := (1 + g) * c2.A
	c1p := math.Hypot(a1p, c1.B)
	c2p := math.Hypot(a2p, c2.B)

	h1p := 0.0
	if c1p != 0 {
		h1p = atan2Deg(c1.B, a1p)
	}
	h2p := 0.0
	if c2p != 0 {
		h2p = atan2Deg(c2.B, a2p)
	}

	dLp := c2.L - c1.L
	dCp := c2p - c1p

	var dhp float64
	switch {
	case c1p*c2p == 0:
		dhp = 0
	case math.Abs(h2p-h1p) <= 180:
		dhp = h2p - h1p
	case h2p-h1p > 180:
		dhp = h2p - h1p - 360
	default:
		dhp = h2p - h1p + 360
	}
	dHp := 2 * math.Sqrt(c1p*c2p) * math.Sin(deg2rad(dhp/2))

	Lbarp := (c1.L + c2.L) / 2
	Cbarp := (c1p + c2p) / 2

	var hbarp float64
	switch {
	case c1p*c2p == 0:
		hbarp = h1p + h2p
	case math.Abs(h1p-h2p) <= 180:
		hbarp = (h1p + h2p) / 2
	case h1p+h2p < 360:
		hbarp = (h1p + h2p + 360) / 2
	default:
		hbarp = (h1p + h2p - 360) / 2
	}

	t := 1 - 0.17*math.Cos(deg2rad(hbarp-30)) +
		0.24*math.Cos(deg2rad(2*hbarp)) +
		0.32*math.Cos(deg2rad(3*hbarp+6)) -
		0.20*math.Cos(deg2rad(4*hbarp-63))

	dTheta := 30 * math.Exp(-((hbarp-275)/25)*((hbarp-275)/25))
	Cbarp7 := math.Pow(Cbarp, 7)
	rc := 2 * math.Sqrt(Cbarp7/(Cbarp7+pow25_7))

	sl := 1 + (0.015*(Lbarp-50)*(Lbarp-50))/math.Sqrt(20+(Lbarp-50)*(Lbarp-50))
	sc := 1 + 0.045*Cbarp
	sh := 1 + 0.015*Cbarp*t
	rt := -math.Sin(deg2rad(2*dTheta)) * rc

	tl := dLp / sl
	tc := dCp / sc
	th := dHp / sh
	return math.Sqrt(tl*tl + tc*tc + th*th + rt*tc*th)
}

// pow25_7 is 25^7, the CIEDE2000 chroma-rolloff constant.
var pow25_7 = math.Pow(25, 7)

// DeltaEOK is the OKLab colour difference: the Euclidean distance between two
// OKLab colours. Because OKLab is perceptually near-uniform this is a simple yet
// well-behaved metric.
func DeltaEOK(a, b OKLab) float64 {
	dl := a.L - b.L
	da := a.A - b.A
	db := a.B - b.B
	return math.Sqrt(dl*dl + da*da + db*db)
}
