package color

import (
	"math"
	"testing"
)

// This file proves that the inlinable fast paths in hsl.go (minMax3 instead of
// nested math.Min/math.Max, the dropped modulo-6 wrap in hueOf's max==R branch,
// and math.Mod(hp,2) computed arithmetically in hueToRGB) are BIT-FOR-BIT equal
// to the previous math-package formulations they replaced. The reference
// functions below reproduce those previous formulations verbatim, so the sweep
// compares the new code against the code it replaced.

// refHueOf is the verbatim pre-optimisation hueOf (with the modulo-6 wrap).
func refHueOf(r, g, b, mx, d float64) float64 {
	if d == 0 {
		return 0
	}
	var h float64
	switch mx {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h
}

// refSRGBToHSV is the verbatim pre-optimisation SRGBToHSV (nested math.Max/Min).
func refSRGBToHSV(r, g, b float64) HSV {
	mx := math.Max(r, math.Max(g, b))
	mn := math.Min(r, math.Min(g, b))
	d := mx - mn
	s := 0.0
	if mx != 0 {
		s = d / mx
	}
	return HSV{H: refHueOf(r, g, b, mx, d), S: s, V: mx}
}

// refHueToRGB is the verbatim pre-optimisation hueToRGB (math.Mod + math.Abs).
func refHueToRGB(h, c, m float64) (r, g, b float64) {
	hp := NormalizeHue(h) / 60
	x := c * (1 - math.Abs(math.Mod(hp, 2)-1))
	switch int(hp) {
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
	default:
		r, g, b = c, 0, x
	}
	return r + m, g + m, b + m
}

func refSRGBToHSL(r, g, b float64) HSL {
	mx := math.Max(r, math.Max(g, b))
	mn := math.Min(r, math.Min(g, b))
	d := mx - mn
	l := (mx + mn) / 2
	s := 0.0
	if d != 0 {
		s = d / (1 - math.Abs(2*l-1))
	}
	return HSL{H: refHueOf(r, g, b, mx, d), S: s, L: l}
}

func refSRGBToHWB(r, g, b float64) HWB {
	mx := math.Max(r, math.Max(g, b))
	mn := math.Min(r, math.Min(g, b))
	return HWB{H: refHueOf(r, g, b, mx, mx-mn), W: mn, B: 1 - mx}
}

// TestHSVFastPathMatchesReplacedFormulas sweeps the whole 24-bit RGB cube (the
// exact channel values a byte-backed consumer feeds) and asserts SRGBToHSV,
// SRGBToHSL and SRGBToHWB are bit-identical to the pre-optimisation formulas.
func TestHSVFastPathMatchesReplacedFormulas(t *testing.T) {
	for ri := 0; ri < 256; ri++ {
		r := float64(ri) / 255
		for gi := 0; gi < 256; gi++ {
			g := float64(gi) / 255
			for bi := 0; bi < 256; bi++ {
				b := float64(bi) / 255
				if got, want := SRGBToHSV(r, g, b), refSRGBToHSV(r, g, b); got != want {
					t.Fatalf("SRGBToHSV(%v,%v,%v)=%+v want %+v", r, g, b, got, want)
				}
				if got, want := SRGBToHSL(r, g, b), refSRGBToHSL(r, g, b); got != want {
					t.Fatalf("SRGBToHSL(%v,%v,%v)=%+v want %+v", r, g, b, got, want)
				}
				if got, want := SRGBToHWB(r, g, b), refSRGBToHWB(r, g, b); got != want {
					t.Fatalf("SRGBToHWB(%v,%v,%v)=%+v want %+v", r, g, b, got, want)
				}
			}
		}
	}
}

// TestHueToRGBFastPathMatchesReplacedFormula sweeps every byte-encoded H, S, V
// triple and asserts HSVToSRGB (via hueToRGB) is bit-identical to the
// pre-optimisation formula.
func TestHueToRGBFastPathMatchesReplacedFormula(t *testing.T) {
	for hi := 0; hi < 256; hi++ {
		h := float64(hi) / 255 * 360
		for si := 0; si < 256; si++ {
			s := float64(si) / 255
			for vi := 0; vi < 256; vi++ {
				v := float64(vi) / 255
				c := v * s
				gr, gg, gb := hueToRGB(h, c, v-c)
				wr, wg, wb := refHueToRGB(h, c, v-c)
				if gr != wr || gg != wg || gb != wb {
					t.Fatalf("hueToRGB(%v,%v,%v)=(%v,%v,%v) want (%v,%v,%v)", h, c, v-c, gr, gg, gb, wr, wg, wb)
				}
			}
		}
	}
}
