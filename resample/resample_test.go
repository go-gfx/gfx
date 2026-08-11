package resample

import (
	"image/color"
	"math"
	"math/rand"
	"testing"

	"github.com/go-gfx/gfx/raster"
)

func randomImage(w, h int, seed int64) *raster.Image {
	rng := rand.New(rand.NewSource(seed))
	im := raster.New(w, h)
	for i := range im.Pix {
		im.Pix[i] = uint8(rng.Intn(256))
	}
	return im
}

func opaqueImage(w, h int, seed int64) *raster.Image {
	im := randomImage(w, h, seed)
	for i := 3; i < len(im.Pix); i += 4 {
		im.Pix[i] = 255
	}
	return im
}

// --- independent reference for the filtered modes ---------------------------

// refFilter is the DEFINITION of a filtered separable resize, written out as a
// full two-dimensional weighted sum with the coefficients recomputed here from
// the Pillow scheme. It shares no code with the implementation (its own coeffs,
// no separation into passes, no SIMD), so agreement is a real check.
func refFilter(src *raster.Image, dw, dh int, support float64, f func(float64) float64) *raster.Image {
	xw := refCoeffs1D(src.W, dw, support, f)
	yw := refCoeffs1D(src.H, dh, support, f)
	out := raster.New(dw, dh)
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var acc [4]float64
			for _, yt := range yw[y] {
				for _, xt := range xw[x] {
					si := (yt.idx*src.W + xt.idx) * 4
					w := yt.w * xt.w
					for c := 0; c < 4; c++ {
						acc[c] += float64(src.Pix[si+c]) * w
					}
				}
			}
			di := (y*dw + x) * 4
			for c := 0; c < 4; c++ {
				v := acc[c]
				switch {
				case v <= 0:
					out.Pix[di+c] = 0
				case v >= 255:
					out.Pix[di+c] = 255
				default:
					out.Pix[di+c] = uint8(v + 0.5)
				}
			}
		}
	}
	return out
}

type tap struct {
	idx int
	w   float64
}

func refCoeffs1D(inSize, outSize int, support float64, f func(float64) float64) [][]tap {
	scale := float64(inSize) / float64(outSize)
	fs := scale
	if fs < 1 {
		fs = 1
	}
	sup := support * fs
	ss := 1 / fs
	all := make([][]tap, outSize)
	for i := 0; i < outSize; i++ {
		center := (float64(i) + 0.5) * scale
		xmin := int(center - sup + 0.5)
		if xmin < 0 {
			xmin = 0
		}
		xmax := int(center + sup + 0.5)
		if xmax > inSize {
			xmax = inSize
		}
		var taps []tap
		var sum float64
		for j := xmin; j < xmax; j++ {
			w := f((float64(j) - center + 0.5) * ss)
			taps = append(taps, tap{j, w})
			sum += w
		}
		for k := range taps {
			taps[k].w /= sum
		}
		all[i] = taps
	}
	return all
}

func refCubic(x float64) float64 {
	const a = -0.5
	if x < 0 {
		x = -x
	}
	switch {
	case x < 1:
		return ((a+2)*x-(a+3))*x*x + 1
	case x < 2:
		return (((x-5)*x+8)*x - 4) * a
	default:
		return 0
	}
}

func refLanczos(x float64) float64 {
	if x <= -3 || x >= 3 {
		return 0
	}
	s := func(t float64) float64 {
		if t == 0 {
			return 1
		}
		t *= math.Pi
		return math.Sin(t) / t
	}
	return s(x) * s(x/3)
}

func maxByteDiff(t *testing.T, got, want *raster.Image, tol int) {
	t.Helper()
	if len(got.Pix) != len(want.Pix) {
		t.Fatalf("length mismatch: got %d want %d", len(got.Pix), len(want.Pix))
	}
	for i := range want.Pix {
		if d := int(got.Pix[i]) - int(want.Pix[i]); d > tol || d < -tol {
			p := i / 4
			t.Fatalf("pixel %d,%d channel %d = %d, definition says %d (tol %d)",
				p%got.W, p/got.W, i%4, got.Pix[i], want.Pix[i], tol)
		}
	}
}

// The separable filtered implementation must agree with the two-dimensional
// definition. Both round once at the end in float64, so they agree within one
// level (float regrouping across the two passes), and more than that is a bug.
func TestFilteredMatchesTheDefinition(t *testing.T) {
	cases := []struct {
		name   string
		sw, sh int
		dw, dh int
	}{
		{"bicubic down", 40, 32, 17, 13},
		{"bicubic up", 12, 9, 40, 27},
		{"bicubic down x up y", 40, 9, 15, 30},
		{"bicubic unchanged", 20, 20, 20, 20},
	}
	for _, tc := range cases {
		src := randomImage(tc.sw, tc.sh, int64(tc.sw*7+tc.sh))
		for _, m := range []struct {
			name    string
			mode    Mode
			support float64
			f       func(float64) float64
		}{
			{"Bicubic", Bicubic, 2, refCubic},
			{"Lanczos", Lanczos, 3, refLanczos},
		} {
			t.Run(tc.name+"/"+m.name, func(t *testing.T) {
				got, err := Resize(src, tc.dw, tc.dh, m.mode)
				if err != nil {
					t.Fatal(err)
				}
				want := refFilter(src, tc.dw, tc.dh, m.support, m.f)
				maxByteDiff(t, got, want, 1)
			})
		}
	}
}

// --- area / box, ported definition checks -----------------------------------

// refArea is the naive two-dimensional area average, independent of the
// separable implementation.
func refArea(src *raster.Image, dw, dh int) *raster.Image {
	sx := float64(src.W) / float64(dw)
	sy := float64(src.H) / float64(dh)
	out := raster.New(dw, dh)
	span := func(i int, scale float64, n int) (int, int) {
		lo, hi := float64(i)*scale, float64(i)*scale+scale
		i0, i1 := int(math.Floor(lo)), int(math.Ceil(hi))
		if i0 < 0 {
			i0 = 0
		}
		if i1 > n {
			i1 = n
		}
		if i1 <= i0 {
			i1 = i0 + 1
		}
		return i0, i1
	}
	overlap := func(j, i int, scale float64) float64 {
		lo, hi := float64(i)*scale, float64(i)*scale+scale
		w := math.Min(hi, float64(j+1)) - math.Max(lo, float64(j))
		if w < 0 {
			return 0
		}
		return w
	}
	for y := 0; y < dh; y++ {
		y0, y1 := span(y, sy, src.H)
		for x := 0; x < dw; x++ {
			x0, x1 := span(x, sx, src.W)
			var acc [4]float64
			var sum float64
			for j := y0; j < y1; j++ {
				wy := overlap(j, y, sy)
				for i := x0; i < x1; i++ {
					w := wy * overlap(i, x, sx)
					si := (j*src.W + i) * 4
					for c := 0; c < 4; c++ {
						acc[c] += float64(src.Pix[si+c]) * w
					}
					sum += w
				}
			}
			di := (y*dw + x) * 4
			for c := 0; c < 4; c++ {
				out.Pix[di+c] = uint8(math.Round(math.Max(0, math.Min(255, acc[c]/sum))))
			}
		}
	}
	return out
}

func TestBoxMatchesTheDefinition(t *testing.T) {
	for _, tc := range []struct{ sw, sh, dw, dh int }{
		{16, 12, 8, 6}, {17, 13, 5, 4}, {9, 7, 1, 1}, {4, 3, 12, 9}, {16, 3, 4, 9},
	} {
		src := randomImage(tc.sw, tc.sh, int64(tc.sw*31+tc.sh))
		got, err := Resize(src, tc.dw, tc.dh, Box)
		if err != nil {
			t.Fatal(err)
		}
		maxByteDiff(t, got, refArea(src, tc.dw, tc.dh), 1)
	}
}

func TestBoxEnlargingEqualsNearest(t *testing.T) {
	src := randomImage(5, 4, 7)
	area, err := Resize(src, 15, 12, Box)
	if err != nil {
		t.Fatal(err)
	}
	near, err := Resize(src, 15, 12, Nearest)
	if err != nil {
		t.Fatal(err)
	}
	for i := range near.Pix {
		if area.Pix[i] != near.Pix[i] {
			t.Fatalf("byte %d: area %d, nearest %d", i, area.Pix[i], near.Pix[i])
		}
	}
}

// --- bilinear -----------------------------------------------------------------

// TestBilinearHitsBorders enlarges and reduces so the sample positions fall
// before the first and past the last source pixel, exercising the clamp-to-edge
// index clamping in both directions.
func TestBilinearHitsBorders(t *testing.T) {
	src := randomImage(6, 5, 99)
	for _, d := range []int{3, 20} {
		got, err := Resize(src, d, d, Bilinear)
		if err != nil {
			t.Fatal(err)
		}
		if got.W != d || got.H != d {
			t.Fatalf("size = %dx%d, want %dx%d", got.W, got.H, d, d)
		}
	}
	// A 1x1 constant image bilinear-enlarged stays that constant everywhere.
	one := raster.New(1, 1)
	one.Set(0, 0, color.RGBA{40, 80, 120, 200})
	up, err := Resize(one, 4, 4, Bilinear)
	if err != nil {
		t.Fatal(err)
	}
	for p := 0; p < 16; p++ {
		if got := up.At(p%4, p/4); got != (color.RGBA{40, 80, 120, 200}) {
			t.Fatalf("pixel %d = %v, want the constant", p, got)
		}
	}
}

// --- premultiplied alpha ------------------------------------------------------

// whiteDiskOnTransparent builds a white opaque disk on a fully transparent
// (RGB 0) background — the cut-out whose edge straight-alpha resampling fringes.
func whiteDiskOnTransparent(size int) *raster.Image {
	im := raster.New(size, size)
	c := (float64(size) - 1) / 2
	r := float64(size) * 0.34
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if (float64(x)-c)*(float64(x)-c)+(float64(y)-c)*(float64(y)-c) <= r*r {
				im.Set(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	return im
}

// TestPremultipliedRemovesEdgeFringe is the reason the premultiplied path exists:
// on a white disk over transparent black, straight resampling darkens the
// semi-transparent edge (it averages in the background's zero colour), while the
// premultiplied path keeps the edge white. We assert a partially transparent
// edge pixel exists whose straight colour is visibly dark and whose
// premultiplied colour is near white.
func TestPremultipliedRemovesEdgeFringe(t *testing.T) {
	src := whiteDiskOnTransparent(64)
	for _, mode := range []Mode{Bicubic, Lanczos, Box, Bilinear} {
		straight, err := Resize(src, 24, 24, mode)
		if err != nil {
			t.Fatal(err)
		}
		premul, err := ResizePremultiplied(src, 24, 24, mode)
		if err != nil {
			t.Fatal(err)
		}
		foundEdge := false
		for p := 0; p < 24*24; p++ {
			a := premul.Pix[p*4+3]
			if a < 40 || a > 220 { // a genuine partially transparent edge pixel
				continue
			}
			foundEdge = true
			sr := straight.Pix[p*4]
			pr := premul.Pix[p*4]
			if pr < 250 {
				t.Errorf("mode %d edge pixel %d: premultiplied R=%d, want ~255 (clean edge)", mode, p, pr)
			}
			if int(pr)-int(sr) < 20 {
				t.Errorf("mode %d edge pixel %d: straight R=%d not darker than premultiplied R=%d (no fringe to fix?)", mode, p, sr, pr)
			}
		}
		if !foundEdge {
			t.Fatalf("mode %d: no partially transparent edge pixel found", mode)
		}
	}
}

// On a fully opaque image, premultiplication is the identity, so
// ResizePremultiplied must match Resize byte-for-byte in every mode.
func TestPremultipliedOpaqueEqualsStraight(t *testing.T) {
	src := opaqueImage(20, 16, 3)
	for _, mode := range []Mode{Nearest, Bilinear, Box, Bicubic, Lanczos} {
		a, err := Resize(src, 9, 7, mode)
		if err != nil {
			t.Fatal(err)
		}
		b, err := ResizePremultiplied(src, 9, 7, mode)
		if err != nil {
			t.Fatal(err)
		}
		for i := range a.Pix {
			if a.Pix[i] != b.Pix[i] {
				t.Fatalf("mode %d byte %d: straight %d, premultiplied %d", mode, i, a.Pix[i], b.Pix[i])
			}
		}
	}
}

// --- errors -------------------------------------------------------------------

func TestResizeErrors(t *testing.T) {
	src := opaqueImage(4, 4, 1)
	for _, fn := range []struct {
		name string
		call func(int, int, Mode) (*raster.Image, error)
	}{
		{"Resize", func(w, h int, m Mode) (*raster.Image, error) { return Resize(src, w, h, m) }},
		{"ResizePremultiplied", func(w, h int, m Mode) (*raster.Image, error) { return ResizePremultiplied(src, w, h, m) }},
	} {
		t.Run(fn.name, func(t *testing.T) {
			if _, err := fn.call(0, 4, Bicubic); err == nil {
				t.Error("want error for zero width")
			}
			if _, err := fn.call(4, -1, Bicubic); err == nil {
				t.Error("want error for negative height")
			}
			if _, err := fn.call(4, 4, Mode(99)); err == nil {
				t.Error("want error for unknown mode")
			}
		})
	}
}

// --- parallel / SIMD ----------------------------------------------------------

// TestParallelMatchesSerial forces the parallel path (ParThreshold 0) and checks
// the filtered modes produce the identical bytes as the serial path, so the
// multicore tiling is verified to be independent of the worker count.
func TestParallelMatchesSerial(t *testing.T) {
	src := randomImage(50, 40, 123)
	old := ParThreshold
	defer func() { ParThreshold = old }()
	for _, mode := range []Mode{Bicubic, Lanczos} {
		ParThreshold = 1 << 30 // serial
		serial, err := Resize(src, 33, 27, mode)
		if err != nil {
			t.Fatal(err)
		}
		ParThreshold = 0 // always parallel
		par, err := Resize(src, 33, 27, mode)
		if err != nil {
			t.Fatal(err)
		}
		for i := range serial.Pix {
			if serial.Pix[i] != par.Pix[i] {
				t.Fatalf("mode %d byte %d: serial %d, parallel %d", mode, i, serial.Pix[i], par.Pix[i])
			}
		}
	}
}

// TestParallelHelpers exercises parallelLines directly for the single-worker
// branch (which forLines does not reach above threshold) and confirms a band
// split covers every line exactly once.
func TestParallelHelpers(t *testing.T) {
	const n = 7
	seen := make([]int, n)
	parallelLines(n, 1, func(lo, hi int) {
		for i := lo; i < hi; i++ {
			seen[i]++
		}
	})
	var mu [n]int
	parallelLines(n, 3, func(lo, hi int) {
		for i := lo; i < hi; i++ {
			mu[i]++
		}
	})
	for i := 0; i < n; i++ {
		if seen[i] != 1 || mu[i] != 1 {
			t.Fatalf("line %d covered serial=%d parallel=%d, want 1 and 1", i, seen[i], mu[i])
		}
	}
	if numWorkers(1) != 1 {
		t.Errorf("numWorkers(1) = %d, want 1", numWorkers(1))
	}
}

// TestAxpyMatchesScalar validates the per-arch axpy dispatch against the scalar
// oracle (bit-identical on FMA-baseline targets; a tight tolerance covers a
// GOAMD64=v3 build whose oracle itself fuses), logs which SIMD path this arch
// ran, and covers the empty-slice guard.
func TestAxpyMatchesScalar(t *testing.T) {
	t.Logf("HaveSIMD=%v SIMDName=%s", HaveSIMD, SIMDName)
	rng := rand.New(rand.NewSource(5))
	for _, n := range []int{0, 1, 2, 3, 4, 5, 8, 17, 64} {
		want := make([]float64, n)
		got := make([]float64, n)
		src := make([]float64, n)
		for i := 0; i < n; i++ {
			src[i] = rng.NormFloat64()
			v := rng.NormFloat64()
			want[i], got[i] = v, v
		}
		a := rng.NormFloat64()
		axpyScalar(want, src, a)
		axpy(got, src, a)
		for i := 0; i < n; i++ {
			if d := math.Abs(got[i] - want[i]); d > 1e-9*(1+math.Abs(want[i])) {
				t.Fatalf("n=%d i=%d: axpy=%v scalar=%v", n, i, got[i], want[i])
			}
		}
	}
}
