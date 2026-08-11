//go:build ignore

// Command gen produces simd_arm64.s — the NEON float64 multiply-accumulate
// (axpy) kernel — via go-asmgen. Run with `go run gen.go` or `go generate` from
// the resample package.
//
// NEON (ASIMD) is part of the arm64 baseline the Go toolchain requires, so the
// kernel is always callable with no CPU-feature branch.
//
//	axpyNEON(dst, src *float64, a float64, n int)
//	  dst[i] += a*src[i]. VFMLA is a packed fused multiply-add; arm64 fuses the
//	  scalar oracle to FMADDD, so the kernel is bit-identical to it.
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/arm64"
	"github.com/go-asmgen/asmgen/emit"
)

func main() {
	f := emit.NewFile("arm64")
	f.Add(axpyKernel())
	if err := os.WriteFile("simd_arm64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote simd_arm64.s")
}

// axpyKernel builds axpyNEON(dst, src *float64, a float64, n int): dst[i] +=
// a*src[i]. The scalar a is duplicated into both lanes of V16; the loop
// processes 4 doubles (two D2 registers) per iteration with a fused
// multiply-add, and a scalar tail.
func axpyKernel() *emit.Function {
	sig := arm64.Layout(
		[]string{"dst", "src", "a", "n"},
		[]arm64.Type{arm64.Ptr, arm64.Ptr, arm64.Float64, arm64.Int64},
		nil, nil,
	)
	b := arm64.NewFunc("axpyNEON", sig, 0)
	b.LoadArg("dst", "R0").
		LoadArg("src", "R1").
		LoadArg("a", "F8").
		LoadArg("n", "R2").
		Raw("FMOVD F8, R3").
		Raw("VDUP R3, V16.D2").
		Raw("block:").
		Raw("CMP $4, R2").
		Raw("BLT tail").
		Raw("VLD1 (R1), [V0.D2, V1.D2]").
		Raw("VLD1 (R0), [V2.D2, V3.D2]").
		Raw("VFMLA V0.D2, V16.D2, V2.D2").
		Raw("VFMLA V1.D2, V16.D2, V3.D2").
		Raw("VST1 [V2.D2, V3.D2], (R0)").
		Raw("ADD $32, R0, R0").
		Raw("ADD $32, R1, R1").
		Raw("SUB $4, R2").
		Raw("B block").
		Raw("tail:").
		Raw("CBZ R2, done").
		Raw("FMOVD.P 8(R1), F1").
		Raw("FMOVD (R0), F2").
		Raw("FMADDD F8, F2, F1, F2").
		Raw("FMOVD.P F2, 8(R0)").
		Raw("SUB $1, R2").
		Raw("B tail").
		Raw("done:").
		Ret()
	return b.Func()
}
