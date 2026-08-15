// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import "math"

// This file implements Björn Ottosson's OKLab perceptual colour space and its
// cylindrical form OKLCH (https://bottosson.github.io/posts/oklab/). OKLab is a
// D65 space with markedly better perceptual uniformity than CIE L*a*b* for hue
// and lightness; the CSS Color Module Level 4 adopts it. The matrices below are
// Ottosson's published constants, going from linear sRGB through a cube-root
// cone space to OKLab. L is in [0, 1] for in-gamut colour; a and b are the
// green/red and blue/yellow chroma axes.

// OKLab is a colour in Ottosson's OKLab space: L is lightness (0..1), A is the
// green(-)/red(+) axis and B the blue(-)/yellow(+) axis.
type OKLab struct{ L, A, B float64 }

// OKLCH is the cylindrical form of [OKLab]: L is lightness (0..1), C is chroma
// and H is the hue angle in degrees [0, 360).
type OKLCH struct{ L, C, H float64 }

// LinearRGBToOKLab converts linear-light sRGB primaries (each 0..1) to OKLab.
func LinearRGBToOKLab(r, g, b float64) OKLab {
	l := 0.4122214708*r + 0.5363325363*g + 0.0514459929*b
	m := 0.2119034982*r + 0.6806995451*g + 0.1073969566*b
	s := 0.0883024619*r + 0.2817188376*g + 0.6299787005*b

	l_ := math.Cbrt(l)
	m_ := math.Cbrt(m)
	s_ := math.Cbrt(s)

	return OKLab{
		L: 0.2104542553*l_ + 0.7936177850*m_ - 0.0040720468*s_,
		A: 1.9779984951*l_ - 2.4285922050*m_ + 0.4505937099*s_,
		B: 0.0259040371*l_ + 0.7827717662*m_ - 0.8086757660*s_,
	}
}

// OKLabToLinearRGB converts an OKLab colour to linear-light sRGB primaries (each
// 0..1 for in-gamut colour), the inverse of [LinearRGBToOKLab].
func OKLabToLinearRGB(c OKLab) (r, g, b float64) {
	l_ := c.L + 0.3963377774*c.A + 0.2158037573*c.B
	m_ := c.L - 0.1055613458*c.A - 0.0638541728*c.B
	s_ := c.L - 0.0894841775*c.A - 1.2914855480*c.B

	l := l_ * l_ * l_
	m := m_ * m_ * m_
	s := s_ * s_ * s_

	r = +4.0767416621*l - 3.3077115913*m + 0.2309699292*s
	g = -1.2684380046*l + 2.6097574011*m - 0.3413193965*s
	b = -0.0041960863*l - 0.7034186147*m + 1.7076147010*s
	return
}

// SRGBToOKLab converts a gamma-encoded sRGB colour (each channel 0..1) to OKLab.
func SRGBToOKLab(r, g, b float64) OKLab {
	return LinearRGBToOKLab(SRGBToLinear(r), SRGBToLinear(g), SRGBToLinear(b))
}

// OKLabToSRGB converts an OKLab colour to gamma-encoded sRGB (each channel 0..1,
// not clamped to gamut), the inverse of [SRGBToOKLab].
func OKLabToSRGB(c OKLab) (r, g, b float64) {
	lr, lg, lb := OKLabToLinearRGB(c)
	return LinearToSRGB(lr), LinearToSRGB(lg), LinearToSRGB(lb)
}

// OKLabToOKLCH converts OKLab to its cylindrical form OKLCH.
func OKLabToOKLCH(c OKLab) OKLCH {
	l := toCylindrical(c.L, c.A, c.B)
	return OKLCH{L: l.L, C: l.C, H: l.H}
}

// OKLCHToOKLab converts cylindrical OKLCH back to OKLab, the inverse of
// [OKLabToOKLCH].
func OKLCHToOKLab(c OKLCH) OKLab {
	a, b := fromCylindrical(LCH{L: c.L, C: c.C, H: c.H})
	return OKLab{L: c.L, A: a, B: b}
}

// SRGBToOKLCH converts a gamma-encoded sRGB colour to OKLCH.
func SRGBToOKLCH(r, g, b float64) OKLCH { return OKLabToOKLCH(SRGBToOKLab(r, g, b)) }

// OKLCHToSRGB converts an OKLCH colour back to gamma-encoded sRGB (not clamped
// to gamut), the inverse of [SRGBToOKLCH].
func OKLCHToSRGB(c OKLCH) (r, g, b float64) { return OKLabToSRGB(OKLCHToOKLab(c)) }
