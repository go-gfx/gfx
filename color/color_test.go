package color

import "testing"

// Premultiply then UnpremultiplyInPlace recovers the original colour within
// rounding for a normal (colour <= 255, any alpha) pixel, and drives the
// alpha == 0 branch (colour forced to black).
func TestPremultiplyRoundTrip(t *testing.T) {
	src := []uint8{
		200, 100, 50, 255, // opaque: exact
		200, 100, 50, 128, // half alpha: within rounding
		123, 231, 45, 0, // transparent: colour must come back black
	}
	pm := Premultiply(src)
	// Alpha is carried through unchanged.
	for i := 3; i < len(src); i += 4 {
		if pm[i] != src[i] {
			t.Fatalf("alpha at %d changed: %d -> %d", i, src[i], pm[i])
		}
	}
	// Premultiplying by alpha 0 zeroes the colour.
	if pm[8] != 0 || pm[9] != 0 || pm[10] != 0 {
		t.Fatalf("premultiplied transparent pixel = %v, want zero colour", pm[8:11])
	}
	out := append([]uint8(nil), pm...)
	UnpremultiplyInPlace(out)
	// Opaque pixel: exact.
	for c := 0; c < 3; c++ {
		if out[c] != src[c] {
			t.Fatalf("opaque channel %d = %d, want %d", c, out[c], src[c])
		}
	}
	// Half-alpha pixel: within one level.
	for c := 4; c < 7; c++ {
		if d := int(out[c]) - int(src[c]); d > 1 || d < -1 {
			t.Fatalf("half-alpha channel %d = %d, want ~%d", c, out[c], src[c])
		}
	}
	// Transparent pixel stays black.
	if out[8] != 0 || out[9] != 0 || out[10] != 0 {
		t.Fatalf("transparent pixel = %v, want black", out[8:11])
	}
}

// UnpremultiplyInPlace clamps to 255 when a channel exceeds its alpha (which a
// signed resampling kernel can produce): 200 over alpha 100 divides to 510.
func TestUnpremultiplyClampsOverflow(t *testing.T) {
	pix := []uint8{200, 0, 100, 100}
	UnpremultiplyInPlace(pix)
	if pix[0] != 255 {
		t.Fatalf("overflowing channel = %d, want clamped 255", pix[0])
	}
	if pix[3] != 100 {
		t.Fatalf("alpha changed to %d, want 100", pix[3])
	}
}
