package raster

import (
	"image"
	"image/color"
	"testing"
)

func TestNewAndAccessors(t *testing.T) {
	p := New(3, 2)
	if p.W != 3 || p.H != 2 {
		t.Fatalf("size = %dx%d, want 3x2", p.W, p.H)
	}
	if len(p.Pix) != 3*2*4 {
		t.Fatalf("Pix len = %d, want 24", len(p.Pix))
	}
	if b := p.Bounds(); b != image.Rect(0, 0, 3, 2) {
		t.Fatalf("Bounds = %v", b)
	}
	if o := p.PixOffset(2, 1); o != (1*3+2)*4 {
		t.Fatalf("PixOffset(2,1) = %d, want %d", o, (1*3+2)*4)
	}
	c := color.RGBA{10, 20, 30, 40}
	p.Set(2, 1, c)
	if got := p.At(2, 1); got != c {
		t.Fatalf("At after Set = %v, want %v", got, c)
	}
}

// FromImage on an *image.NRGBA takes the direct-copy fast path; the bytes are
// already straight, so a partially transparent pixel keeps its colour exactly.
func TestFromImageNRGBAFastPath(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	want := []color.NRGBA{{200, 100, 50, 128}, {0, 0, 0, 0}, {255, 255, 255, 255}, {1, 2, 3, 4}}
	for i, c := range want {
		src.SetNRGBA(i%2, i/2, c)
	}
	p := FromImage(src)
	for i, c := range want {
		if got := p.At(i%2, i/2); got != (color.RGBA{c.R, c.G, c.B, c.A}) {
			t.Fatalf("pixel %d = %v, want %v", i, got, c)
		}
	}
}

// FromImage on a non-NRGBA source (here a premultiplied *image.RGBA) converts
// through the straight colour model, so the result is un-premultiplied cleanly.
func TestFromImageGeneralPath(t *testing.T) {
	// A premultiplied RGBA pixel: half-alpha grey. Straight colour is (128,128,128,128).
	src := image.NewRGBA(image.Rect(0, 0, 1, 1))
	src.SetRGBA(0, 0, color.RGBA{64, 64, 64, 128}) // premultiplied bytes
	p := FromImage(src)
	got := p.At(0, 0)
	// 64 premultiplied over alpha 128 -> straight ~128 (allow rounding).
	if got.A != 128 || abs(int(got.R)-128) > 1 {
		t.Fatalf("un-premultiplied = %v, want ~(128,128,128,128)", got)
	}
}

func TestToNRGBARoundTrip(t *testing.T) {
	p := New(2, 2)
	for i := range p.Pix {
		p.Pix[i] = uint8(i * 7)
	}
	n := p.ToNRGBA()
	back := FromImage(n)
	for i := range p.Pix {
		if p.Pix[i] != back.Pix[i] {
			t.Fatalf("byte %d changed across ToNRGBA round trip: %d -> %d", i, p.Pix[i], back.Pix[i])
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
