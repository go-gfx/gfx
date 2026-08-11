//go:build ignore

// Command gen produces simd_s390x.s — the z/Architecture vector-facility float64
// multiply-accumulate (axpy) kernel — via go-asmgen. Run with `go run gen.go` or
// `go generate` from the resample package.
//
// The vector facility is part of the s390x baseline the Go toolchain targets, so
// the kernel is always callable with no CPU-feature branch. s390x is big-endian,
// but axpy is elementwise, so lane order is irrelevant.
//
//	axpyVX(dst, src *float64, a float64, n int)
//	  dst[i] += a*src[i]. VFMADB is a packed fused multiply-add; s390x fuses the
//	  scalar oracle to FMADD, so the kernel is bit-identical to it.
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/emit"
	"github.com/go-asmgen/asmgen/s390x"
)

func main() {
	f := emit.NewFile("s390x")
	f.Add(axpyKernel())
	if err := os.WriteFile("simd_s390x.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote simd_s390x.s")
}

// axpyKernel builds axpyVX(dst, src *float64, a float64, n int): dst[i] +=
// a*src[i]. The scalar a is replicated into both lanes of V3 from its stack
// slot; the loop processes 2 doubles per iteration with a fused multiply-add and
// a scalar tail.
func axpyKernel() *emit.Function {
	sig := s390x.Layout(
		[]string{"dst", "src", "a", "n"},
		[]s390x.Type{s390x.Ptr, s390x.Ptr, s390x.Float64, s390x.Int64},
		nil, nil,
	)
	b := s390x.NewFunc("axpyVX", sig, 0)
	b.LoadArg("dst", "R1").
		LoadArg("src", "R2").
		LoadArg("a", "F0").
		LoadArg("n", "R3").
		Raw("VLREPG a+16(FP), V3").
		Raw("block:").
		Raw("CMPBLT R3, $2, tail").
		Raw("VL (R2), V1").
		Raw("VL (R1), V2").
		Raw("VFMADB V1, V3, V2, V2").
		Raw("VST V2, (R1)").
		Raw("ADD $16, R1").
		Raw("ADD $16, R2").
		Raw("SUB $2, R3").
		Raw("BR block").
		Raw("tail:").
		Raw("CMPBEQ R3, $0, done").
		Raw("FMOVD (R2), F1").
		Raw("FMOVD (R1), F2").
		Raw("FMADD F0, F1, F2").
		Raw("FMOVD F2, (R1)").
		Raw("done:").
		Ret()
	return b.Func()
}
