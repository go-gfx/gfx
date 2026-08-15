package color

import (
	"math"
	"testing"
)

// The four W3C non-separable blend modes match an independent reference
// implementation of the Compositing and Blending Level 1 procedures (computed
// out of tree). The five cases together drive both ClipColor branches (n < 0 and
// x > 1) and the SetSat achromatic-source branch (a grey operand).
func TestNonSeparableBlends(t *testing.T) {
	rgbClose := func(g, w RGB, tol float64) bool {
		return approx(g.R, w.R, tol) && approx(g.G, w.G, tol) && approx(g.B, w.B, tol)
	}
	cases := []struct {
		cb, cs             RGB
		hue, sat, col, lum RGB
	}{
		{
			RGB{0.2, 0.4, 0.6}, RGB{0.8, 0.2, 0.3},
			RGB{0.6346666667, 0.2346666667, 0.3013333333},
			RGB{0.1190000000, 0.4190000000, 0.7190000000},
			RGB{0.7710000000, 0.1710000000, 0.2710000000},
			RGB{0.2290000000, 0.4290000000, 0.6290000000},
		},
		{
			RGB{0.9, 0.1, 0.7}, RGB{0.2, 0.9, 0.4},
			RGB{0.0000000000, 0.6533333333, 0.1866666667},
			RGB{0.8382500000, 0.1382500000, 0.6632500000},
			RGB{0.0000000000, 0.6533333333, 0.1866666667},
			RGB{1.0000000000, 0.4089068826, 0.8522267206},
		},
		{
			RGB{0.1, 0.05, 0.9}, RGB{0.95, 0.9, 0.05},
			RGB{0.1848995463, 0.1746273493, 0.0000000000},
			RGB{0.0965588235, 0.0436176471, 0.9436176471},
			RGB{0.1848995463, 0.1746273493, 0.0000000000},
			RGB{0.8074173972, 0.7953809845, 1.0000000000},
		},
		{
			RGB{0.5, 0.5, 0.5}, RGB{0.9, 0.1, 0.2},
			RGB{0.5000000000, 0.5000000000, 0.5000000000},
			RGB{0.5000000000, 0.5000000000, 0.5000000000},
			RGB{1.0000000000, 0.2714025501, 0.3624772313},
			RGB{0.3510000000, 0.3510000000, 0.3510000000},
		},
		{
			RGB{0.3, 0.7, 0.2}, RGB{0.5, 0.5, 0.5},
			RGB{0.5250000000, 0.5250000000, 0.5250000000},
			RGB{0.5250000000, 0.5250000000, 0.5250000000},
			RGB{0.5250000000, 0.5250000000, 0.5250000000},
			RGB{0.2750000000, 0.6750000000, 0.1750000000},
		},
	}
	const tol = 1e-9
	for i, c := range cases {
		if got := BlendHue(c.cb, c.cs); !rgbClose(got, c.hue, tol) {
			t.Errorf("case %d BlendHue = %+v, want %+v", i, got, c.hue)
		}
		if got := BlendSaturation(c.cb, c.cs); !rgbClose(got, c.sat, tol) {
			t.Errorf("case %d BlendSaturation = %+v, want %+v", i, got, c.sat)
		}
		if got := BlendColor(c.cb, c.cs); !rgbClose(got, c.col, tol) {
			t.Errorf("case %d BlendColor = %+v, want %+v", i, got, c.col)
		}
		if got := BlendLuminosity(c.cb, c.cs); !rgbClose(got, c.lum, tol) {
			t.Errorf("case %d BlendLuminosity = %+v, want %+v", i, got, c.lum)
		}
	}
}

// The luminosity invariants of the four modes hold exactly: Hue, Saturation and
// Color take the backdrop's luminosity; Luminosity takes the source's.
func TestNonSeparableLuminosityInvariants(t *testing.T) {
	cb := RGB{0.2, 0.55, 0.75}
	cs := RGB{0.7, 0.3, 0.1}
	if l := lum(BlendHue(cb, cs)); math.Abs(l-lum(cb)) > 1e-9 {
		t.Errorf("BlendHue luminosity = %v, want %v", l, lum(cb))
	}
	if l := lum(BlendSaturation(cb, cs)); math.Abs(l-lum(cb)) > 1e-9 {
		t.Errorf("BlendSaturation luminosity = %v, want %v", l, lum(cb))
	}
	if l := lum(BlendColor(cb, cs)); math.Abs(l-lum(cb)) > 1e-9 {
		t.Errorf("BlendColor luminosity = %v, want %v", l, lum(cb))
	}
	if l := lum(BlendLuminosity(cb, cs)); math.Abs(l-lum(cs)) > 1e-9 {
		t.Errorf("BlendLuminosity luminosity = %v, want %v", l, lum(cs))
	}
}
