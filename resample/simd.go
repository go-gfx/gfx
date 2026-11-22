package resample

import "github.com/go-simd/floats"

// resampleV builds each output row as a sum of source rows scaled by the tap
// weights: for one tap, acc[i] += w*src[i] over a whole contiguous RGBA row —
// exactly the multiply-accumulate (axpy) a vector unit does best. The kernel is
// not kept here: floats.Axpy (github.com/go-simd/floats) is the single shared
// implementation (AVX2 FMA on amd64, NEON FMLA on arm64, portable math.FMA
// elsewhere), fused and bit-identical on every architecture.

// axpy computes dst[i] += a*src[i] for i < len(src), one rounding per element
// (math.FMA). src must not be longer than dst.
func axpy(dst, src []float64, a float64) {
	floats.Axpy(a, src, dst)
}
