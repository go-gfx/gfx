package resample

// SIMD dispatch core for the filtered (bicubic / Lanczos) vertical pass.
//
// resampleV builds each output row as a sum of source rows scaled by the tap
// weights: for one tap, acc[i] += w*src[i] over a whole contiguous RGBA row —
// exactly the multiply-accumulate (axpy) a vector unit does best. This file
// holds the scalar oracle; the per-architecture files (simd_amd64.go,
// simd_arm64.go, simd_s390x.go, simd_generic.go) bind the single dispatch point
// `axpy` to a go-asmgen SIMD kernel where one is shipped (amd64 SSE2, arm64
// NEON, s390x z/vector) and to this oracle everywhere else. The kernels are
// validated against the oracle: the per-element computation is identical and,
// on finite float64 data, only the lane width differs.

// axpyScalar computes dst[i] += a*src[i] for every i. The single fused
// multiply-add form is what the gc compiler emits for this loop on FMA-baseline
// targets (amd64-v3, arm64, s390x), so the vector kernels use packed FMA and
// stay bit-identical to this oracle there.
func axpyScalar(dst, src []float64, a float64) {
	for i := range dst {
		dst[i] += a * src[i]
	}
}
