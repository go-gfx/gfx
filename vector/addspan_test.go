package vector

import (
	"fmt"
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
// probeCase records the span about to be drawn. It is package-level on purpose:
// wrapping the call in a closure was enough to make the 386 panic VANISH, so
// nothing may be added at the call site itself.
var probeW int
var probeOx, probeXa, probeXb, probeWeight float64

func TestAddSpanIsBitIdentical(t *testing.T) {
	defer func() {
		if e := recover(); e != nil {
			xa, xb, ox, w := probeXa, probeXb, probeOx, probeW
			if xa < ox {
				xa = ox
			}
			if hi := ox + float64(w); xb > hi {
				xb = hi
			}
			panic(fmt.Sprintf("PROBE w=%d ox=%#x xa=%#x xb=%#x weight=%v || clamped xa=%#x xb=%#x "+
				"|| xa-ox=%#x xb-ox=%#x || floor=%#x ceil=%#x || first=%d last=%d || %v",
				w, math.Float64bits(probeOx), math.Float64bits(probeXa), math.Float64bits(probeXb), probeWeight,
				math.Float64bits(xa), math.Float64bits(xb),
				math.Float64bits(xa-ox), math.Float64bits(xb-ox),
				math.Float64bits(math.Floor(xa-ox)), math.Float64bits(math.Ceil(xb-ox)),
				int(math.Floor(xa-ox)), int(math.Ceil(xb-ox))-1, e))
		}
	}()
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
				probeW, probeOx, probeXa, probeXb, probeWeight = w, ox, xa, xb, weight
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

// TestAddConstMatchesTheLoopItReplaces. Unrolling changes no arithmetic --
// each element depends on itself alone -- so the answer must be the same bit
// for bit, and at every length, because the tail is where an unrolled loop
// goes wrong.
func TestAddConstMatchesTheLoopItReplaces(t *testing.T) {
	for n := 0; n <= 17; n++ {
		for _, w := range []float64{0, 1, 0.5, -0.25, 1e-300, 1e300} {
			want := make([]float64, n)
			got := make([]float64, n)
			for i := range want {
				// Start from values that are not zero, so a kernel that
				// STORES w rather than adding it is caught.
				want[i] = float64(i) * 0.125
				got[i] = want[i]
			}
			for i := range want {
				want[i] += w
			}
			addConst(got, w)
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("n=%d w=%v: got[%d] = %v, want %v", n, w, i, got[i], want[i])
				}
			}
		}
	}
}

// BenchmarkAddConst is the middle of a span: one line that held 170ms of the
// 440ms one page of go-pdfkit spent drawing, more than any other line in that
// renderer. The run lengths are the ones a page produces.
func BenchmarkAddConst(b *testing.B) {
	for _, n := range []int{8, 32, 128, 545, 596} {
		row := make([]float64, n)
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.SetBytes(int64(n) * 8)
			for i := 0; i < b.N; i++ {
				addConst(row, 0.5)
			}
		})
	}
}
