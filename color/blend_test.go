package color

import (
	"math"
	"testing"
)

// Every separable blend mode matches its W3C reference formula, and every
// internal branch (the piecewise dodge/burn/hard-light/soft-light cases) is
// exercised.
func TestBlendModes(t *testing.T) {
	root2 := math.Sqrt(0.5)
	cases := []struct {
		name   string
		mode   BlendMode
		cb, cs float64
		want   float64
	}{
		{"normal", Normal, 0.3, 0.7, 0.7},
		{"multiply", Multiply, 0.5, 0.5, 0.25},
		{"screen", Screen, 0.5, 0.5, 0.75},
		{"overlay-dark", Overlay, 0.25, 0.5, 0.25},
		{"overlay-light", Overlay, 0.75, 0.5, 0.75},
		{"darken", Darken, 0.3, 0.7, 0.3},
		{"lighten", Lighten, 0.3, 0.7, 0.7},
		{"dodge-zero-backdrop", ColorDodge, 0, 0.5, 0},
		{"dodge-white-source", ColorDodge, 0.5, 1, 1},
		{"dodge-mid", ColorDodge, 0.2, 0.5, 0.4},
		{"burn-white-backdrop", ColorBurn, 1, 0.5, 1},
		{"burn-zero-source", ColorBurn, 0.5, 0, 0},
		{"burn-mid", ColorBurn, 0.8, 0.5, 0.6},
		{"hardlight-dark", HardLight, 0.5, 0.25, 0.25},
		{"hardlight-light", HardLight, 0.5, 0.75, 0.75},
		{"softlight-low", SoftLight, 0.5, 0.5, 0.5},
		{"softlight-high-sqrt", SoftLight, 0.5, 0.75, 0.5 + 0.5*(root2-0.5)},
		{"softlight-high-poly", SoftLight, 0.2, 0.75, 0.324},
		{"difference", Difference, 0.3, 0.7, 0.4},
		{"exclusion", Exclusion, 0.5, 0.5, 0.5},
	}
	for _, c := range cases {
		if got := c.mode.Blend(c.cb, c.cs); math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("%s: Blend(%v,%v) = %v, want %v", c.name, c.cb, c.cs, got, c.want)
		}
	}
}

// An unrecognised mode falls back to Normal (returns the source).
func TestBlendUnknownMode(t *testing.T) {
	if got := BlendMode(999).Blend(0.3, 0.7); got != 0.7 {
		t.Fatalf("unknown mode = %v, want source 0.7", got)
	}
}
