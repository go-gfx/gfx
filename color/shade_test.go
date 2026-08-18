// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package color

import (
	"image/color"
	"testing"
)

func TestShade(t *testing.T) {
	base := color.RGBA{R: 200, G: 120, B: 80, A: 255}
	cases := []struct {
		name   string
		in     color.RGBA
		factor float64
		want   color.RGBA
	}{
		{"identity keeps the colour", base, 1.0, color.RGBA{200, 120, 80, 255}},
		{"half darkens each channel, rounds to nearest", base, 0.5, color.RGBA{100, 60, 40, 255}},
		{"three-quarters (left face)", base, 0.75, color.RGBA{150, 90, 60, 255}},
		{"0.55 (right face)", base, 0.55, color.RGBA{110, 66, 44, 255}},
		{"above one clamps to 255 per channel", base, 2.0, color.RGBA{255, 240, 160, 255}},
		{"zero factor yields black, alpha kept", base, 0, color.RGBA{0, 0, 0, 255}},
		{"negative factor also yields black", base, -1, color.RGBA{0, 0, 0, 255}},
		{"alpha is preserved untouched", color.RGBA{80, 80, 80, 128}, 0.5, color.RGBA{40, 40, 40, 128}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Shade(c.in, c.factor)
			if got != c.want {
				t.Fatalf("Shade(%v, %v) = %v, want %v", c.in, c.factor, got, c.want)
			}
		})
	}
}
