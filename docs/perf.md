# Performance — `go-gfx/gfx/resample` vs Pillow

> **The bar:** honest, like-for-like timings on the *same machine*, at realistic
> sizes, for operations with matched semantics — the wins **and** the losses.
> A fake win is worse than a real loss.

## Method

- **Hardware:** Apple Silicon (arm64), macOS, 16 logical CPUs. Both stacks run
  on the same machine, back to back.
- **go-gfx:** Go 1.26.4, `GOWORK=off`, `go test -bench` (`ns/op`, best of the
  timed iterations). The filtered (Bicubic/Lanczos) vertical pass runs the
  **NEON** SIMD axpy kernel plus multicore tiling; Box runs its serial
  separable running-sum. `Mpix/s` is over the **source** pixels processed.
- **Pillow 12.2.0:** `Image.resize`, `time.perf_counter`, best of 30 after a
  warm-up. Pillow's resampler is single-threaded C with SIMD.
- **Matched semantics:** `Bicubic` ⇔ `Image.Resampling.BICUBIC` (both the Keys
  cubic, a = -1/2), `Lanczos` ⇔ `LANCZOS` (both a = 3), `Box` ⇔ `BOX`. Numeric
  parity is checked separately (`parity_test.go`, premultiplied-space PSNR vs
  Pillow references) — a fast wrong answer is not parity.

## Correctness first (premultiplied-space PSNR vs Pillow, `parity_test.go`)

| Case | Bicubic | Lanczos |
|---|--:|--:|
| Opaque gradient / checker, down & up | 52.99 – 61.15 dB | 52.99 – 61.15 dB |
| Transparent disk (premultiplied), 64→24 | 50.47 dB | 45.74 dB |
| Transparent disk (premultiplied), 64→100 | 47.29 dB | 42.72 dB |

We stay in float64 end-to-end; Pillow clips an 8-bit intermediate between its two
passes and keeps a sub-LSB alpha halo where we round to zero, which sets the
ceiling. Pillow's RGBA resize is itself premultiplied-alpha aware, so the
transparent cases are compared against `ResizePremultiplied`.

## Results (arm64, lower `ns/op` / higher `Mpix/s` is better)

### Reduction 1024² → 256² (the thumbnail case)

| Mode | go-gfx `ns/op` | go-gfx Mpix/s | Pillow `ns/op` | Pillow Mpix/s | go-gfx vs Pillow |
|---|--:|--:|--:|--:|--:|
| Bicubic | 1,886,402 | 555.9 | 3,139,167 | 334.0 | **1.66× faster** |
| Lanczos | 2,660,899 | 394.1 | 4,587,000 | 228.6 | **1.72× faster** |
| Box     | 3,530,593 | 297.0 | 2,285,292 | 458.8 | 0.65× (slower) |

### Enlargement 256² → 1024²

| Mode | go-gfx `ns/op` | go-gfx Mpix/s | Pillow `ns/op` | Pillow Mpix/s | go-gfx vs Pillow |
|---|--:|--:|--:|--:|--:|
| Bicubic | 1,621,422 | 40.42 | 8,010,583 | 8.2 | **4.9× faster** |
| Lanczos | 3,298,516 | 19.87 | 10,446,834 | 6.3 | **3.2× faster** |
| Box     | 6,247,909 | 10.49 | 4,813,333 | 13.6 | 0.77× (slower) |

## Reading the numbers

- **Bicubic and Lanczos beat Pillow 1.7–4.9×.** The vertical pass is one flat
  NEON multiply-accumulate per tap over whole RGBA rows, and both passes tile
  across cores; Pillow's resampler is single-threaded. The gap is widest on the
  enlargement, where the per-pixel kernel is narrow and throughput is dominated
  by how many cores share the rows.
- **Box is currently slower than Pillow** (0.65–0.77×). Pillow's BOX is
  hand-tuned SIMD C, and go-gfx's Box still runs the **serial** running-sum
  (it does not yet ride the multicore tiling the filtered modes use, and its
  accumulation is not vectorised). Making Box tile across cores and vectorise
  its running sum is the obvious next step; it is reported as a loss until then.

The SIMD coverage the fleet documents: **amd64 SSE2**, **arm64 NEON**, **s390x
z/vector** ship the axpy kernel (`asmgen/`), validated bit-for-bit against the
scalar oracle in `resample_test.go`; **loong64 / ppc64le / riscv64** keep the
scalar inner loop plus multicore tiling. The numbers above are the arm64 (NEON)
path.
