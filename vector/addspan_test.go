package vector

import (
	"math"
	"math/rand"
	"testing"
)

// addSpanGeneral is what addSpan was before the middle of a span stopped being
// worked out one pixel at a time. It is kept as the thing the faster version
// has to agree with, bit for bit: coverage feeds straight into pixel values,
// so a difference of one unit in the last place is a different picture.
func addSpanGeneral(row []float64, xa, xb, ox float64, w int, weight float64) {
	if xa < ox {
		xa = ox
	}
	if hi := ox + float64(w); xb > hi {
		xb = hi
	}
	if xb <= xa {
		return
	}
	ixa := int(math.Floor(xa - ox))
	ixb := int(math.Ceil(xb - ox))
	for ix := ixa; ix < ixb; ix++ {
		left := xa
		if l := ox + float64(ix); l > left {
			left = l
		}
		right := xb
		if r := ox + float64(ix+1); r < right {
			right = r
		}
		if c := right - left; c > 0 {
			row[ix] += c * weight
		}
	}
}

// TestAddSpanIsBitIdentical compares the two over a wide sample of spans: ones
// that fall inside a single pixel, ones that straddle two, ones that cover a
// whole row, ones that begin and end exactly on a pixel boundary, and ones
// that hang off both ends and have to be clipped.
func TestAddSpanIsBitIdentical(t *testing.T) {
	r := rand.New(rand.NewSource(20260826))
	widths := []int{1, 2, 3, 7, 64, 257, 1024}
	origins := []float64{0, 1, -1, 17, -313, 4096, 1 << 20}
	checked := 0
	for _, w := range widths {
		for _, ox := range origins {
			fast := make([]float64, w)
			slow := make([]float64, w)
			for i := 0; i < 4000; i++ {
				var xa, xb float64
				switch i % 4 {
				case 0: // anywhere, including off both ends
					xa = ox + (r.Float64()*float64(w+4) - 2)
					xb = xa + r.Float64()*float64(w+4)
				case 1: // inside one pixel
					p := float64(r.Intn(w))
					xa = ox + p + r.Float64()
					xb = xa + r.Float64()*0.4
				case 2: // exactly on pixel boundaries
					a := r.Intn(w + 1)
					b := r.Intn(w + 1)
					if b < a {
						a, b = b, a
					}
					xa, xb = ox+float64(a), ox+float64(b)
				default: // the whole row and more
					xa = ox - r.Float64()*3
					xb = ox + float64(w) + r.Float64()*3
				}
				weight := []float64{1, 0.5, 0.25, 1.0 / 3, 1.0 / 7, 1e-9, 1e9}[r.Intn(7)]
				addSpan(fast, xa, xb, ox, w, weight)
				addSpanGeneral(slow, xa, xb, ox, w, weight)
				for ix := range slow {
					if math.Float64bits(fast[ix]) != math.Float64bits(slow[ix]) {
						t.Fatalf("w=%d ox=%v span [%v,%v) weight=%v: pixel %d is %v (%#x) "+
							"where the general form gives %v (%#x)",
							w, ox, xa, xb, weight, ix,
							fast[ix], math.Float64bits(fast[ix]),
							slow[ix], math.Float64bits(slow[ix]))
					}
				}
				checked++
			}
		}
	}
	if checked < 100000 {
		t.Fatalf("only %d spans compared", checked)
	}
	t.Logf("%d spans compared, every pixel bit for bit", checked)
}

// TestAddSpanCoversWhatItShould states the arithmetic plainly, so that a
// reader can see what the fast path is claiming without reading both versions.
func TestAddSpanCoversWhatItShould(t *testing.T) {
	cases := []struct {
		name       string
		xa, xb, ox float64
		w          int
		want       []float64
	}{
		{"inside one pixel", 2.25, 2.75, 0, 4, []float64{0, 0, 0.5, 0}},
		{"across two", 1.5, 2.5, 0, 4, []float64{0, 0.5, 0.5, 0}},
		{"three whole pixels", 1, 4, 0, 4, []float64{0, 1, 1, 1}},
		{"partial both ends", 0.75, 3.25, 0, 4, []float64{0.25, 1, 1, 0.25}},
		{"clipped left", -5, 2, 0, 4, []float64{1, 1, 0, 0}},
		{"clipped right", 2, 99, 0, 4, []float64{0, 0, 1, 1}},
		{"empty", 2, 2, 0, 4, []float64{0, 0, 0, 0}},
		{"reversed", 3, 1, 0, 4, []float64{0, 0, 0, 0}},
		{"shifted origin", 102.5, 104, 100, 4, []float64{0, 0, 0.5, 1}},
	}
	for _, c := range cases {
		row := make([]float64, c.w)
		addSpan(row, c.xa, c.xb, c.ox, c.w, 1)
		for i := range row {
			if math.Abs(row[i]-c.want[i]) > 1e-12 {
				t.Errorf("%s: got %v, want %v", c.name, row, c.want)
				break
			}
		}
	}
}
