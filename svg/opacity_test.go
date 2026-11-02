package svg

import (
	"fmt"
	"strings"
	"testing"
)

func opDoc(body string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" width="20" height="20">` + body + `</svg>`
}

func alphaAt(t *testing.T, body string, x, y int) uint8 {
	t.Helper()
	res, err := Rasterize(opDoc(body), Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	return res.Image.At(x, y).A
}

// gfx#77: the three spellings of 18 % all give round(0.18*255) = 46.
func TestOpacityForms(t *testing.T) {
	for _, form := range []string{
		`<g opacity="0.18"><rect width="10" height="10" fill="#06183a"/></g>`,
		`<rect width="10" height="10" fill="#06183a" opacity="0.18"/>`,
		`<rect width="10" height="10" fill="#06183a" fill-opacity="0.18"/>`,
	} {
		if a := alphaAt(t, form, 5, 5); a != 46 {
			t.Errorf("alpha %d, want 46: %s", a, form)
		}
	}
}

// SVG 1.1 §14.5: group compositing. Overlapping children must not show
// through each other: the overlap has the same alpha as the rest (128), not
// the 191 a per-child multiplication gives.
func TestOpacityGroupOverlap(t *testing.T) {
	body := `<g opacity="0.5"><rect width="12" height="10" fill="#000"/><rect x="6" width="12" height="10" fill="#000"/></g>`
	for _, x := range []int{2, 9, 15} {
		if a := alphaAt(t, body, x, 5); a != 128 {
			t.Errorf("x=%d alpha %d, want 128", x, a)
		}
	}
	// The wrong model, for contrast.
	per := `<rect width="12" height="10" fill="#000" fill-opacity="0.5"/><rect x="6" width="12" height="10" fill="#000" fill-opacity="0.5"/>`
	if a := alphaAt(t, per, 9, 5); a < 191 || a > 192 {
		t.Errorf("control: per-child overlap %d, want 191 (192 after 8-bit rounding)", a)
	}
}

// Nested groups multiply: 0.5 * 0.5 -> 64 (63.75).
func TestOpacityNested(t *testing.T) {
	body := `<g opacity="0.5"><g opacity="0.5"><rect width="10" height="10" fill="#000"/></g></g>`
	if a := alphaAt(t, body, 5, 5); a != 64 {
		t.Errorf("alpha %d, want 64", a)
	}
}

// opacity is not inherited as a second multiplication: a child of an
// opacity group with no opacity of its own is drawn once.
func TestOpacityNotInherited(t *testing.T) {
	body := `<g opacity="0.5"><g><rect width="10" height="10" fill="#000"/></g></g>`
	if a := alphaAt(t, body, 5, 5); a != 128 {
		t.Errorf("alpha %d, want 128", a)
	}
}

func TestOpacityRootAndZeroAndOne(t *testing.T) {
	res, err := Rasterize(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" opacity="0.5"><rect width="10" height="10"/></svg>`, Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	if a := res.Image.At(5, 5).A; a != 128 {
		t.Errorf("root opacity alpha %d, want 128", a)
	}
	if a := alphaAt(t, `<rect width="10" height="10" opacity="0"/>`, 5, 5); a != 0 {
		t.Errorf("opacity 0 alpha %d", a)
	}
	if a := alphaAt(t, `<rect width="10" height="10" opacity="1"/>`, 5, 5); a != 255 {
		t.Errorf("opacity 1 alpha %d", a)
	}
}

// A layer's buffer is recycled: a second group must not inherit the first's
// pixels.
func TestOpacityBufferRecycled(t *testing.T) {
	body := `<g opacity="0.5"><rect width="5" height="5"/></g><g opacity="0.5"><rect x="10" y="10" width="5" height="5"/></g>`
	if a := alphaAt(t, body, 2, 2); a != 128 {
		t.Errorf("first %d", a)
	}
	if a := alphaAt(t, body, 12, 12); a != 128 {
		t.Errorf("second %d", a)
	}
	if a := alphaAt(t, body, 2, 12); a != 0 {
		t.Errorf("stale pixel %d", a)
	}
}

// Nesting past maxLayers must not allocate without bound and must not panic.
func TestOpacityDepthBounded(t *testing.T) {
	n := 500
	body := strings.Repeat(`<g opacity="0.9">`, n) + `<rect width="10" height="10"/>` + strings.Repeat(`</g>`, n)
	if a := alphaAt(t, body, 5, 5); a == 0 || a == 255 {
		t.Errorf("alpha %d", a)
	}
}

func FuzzRasterizeOpacity(f *testing.F) {
	f.Add(`<g opacity="0.5"><rect width="5" height="5"/></g>`)
	f.Add(`<rect opacity="50%" width="5" height="5"/>`)
	f.Add(`<g opacity="nan"><g opacity="-1"><circle r="3"/></g></g>`)
	f.Fuzz(func(t *testing.T, body string) {
		_, _ = Rasterize(opDoc(body), Options{Scale: 1, MaxPixels: 1 << 16})
	})
}

func benchDoc(b *testing.B, doc string) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Rasterize(doc, Options{Scale: 2}); err != nil {
			b.Fatal(err)
		}
	}
}

func shapes(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, `<rect x="%d" y="%d" width="40" height="40" fill="#369"/>`, i*3, i*2)
	}
	return sb.String()
}

func BenchmarkNoOpacity(b *testing.B) {
	benchDoc(b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 400">`+shapes(50)+`</svg>`)
}

func BenchmarkOpacityGroup(b *testing.B) {
	benchDoc(b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 400"><g opacity="0.5">`+shapes(50)+`</g></svg>`)
}

func BenchmarkOpacityNested(b *testing.B) {
	benchDoc(b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 400"><g opacity="0.9"><g opacity="0.9"><g opacity="0.9">`+shapes(50)+`</g></g></g></svg>`)
}
