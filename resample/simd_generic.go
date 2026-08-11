//go:build !amd64 && !arm64 && !s390x

package resample

// Architectures without a generated SIMD kernel route axpy to the scalar oracle,
// so the package builds and runs uniformly on every target. amd64 (SSE2), arm64
// (NEON) and s390x (z/vector) ship kernels; this is the loong64 / ppc64le /
// riscv64 fallback:
//   - loong64's LSX and ppc64le's VSX expose no vector double arithmetic in the
//     Go assembler, and
//   - riscv64's V extension is optional and absent from the default qemu CPU,
//     so a vector kernel would SIGILL under the arch-qemu CI job.
// For these the scalar inner loop plus multicore tiling carries the speed-up,
// and the per-arch qemu jobs still exercise this dispatch.

// HaveSIMD reports whether this build routes the filtered vertical pass through a
// hand-vectorized SIMD axpy kernel (true on amd64/arm64/s390x) or the scalar
// oracle (false here). The test logs it so each per-arch CI run states which path
// it validated.
const HaveSIMD = false

// SIMDName identifies the vector kernel family for the per-arch test log.
const SIMDName = "scalar"

func axpy(dst, src []float64, a float64) { axpyScalar(dst, src, a) }
