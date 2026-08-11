//go:build ignore

// Command gen produces simd_amd64.s — the SSE2 float64 multiply-accumulate
// (axpy) kernel — via go-asmgen. Run with `go run gen.go` or `go generate` from
// the resample package.
//
// SSE2 is part of the amd64 baseline (GOAMD64=v1), so the kernel is always
// callable with no CPU-feature branch.
//
//	axpySSE2(dst, src *float64, a float64, n int)
//	  dst[i] += a*src[i] for i in [0,n). The filtered resampling vertical pass
//	  applies one kernel tap weight a to a whole shifted RGBA row. The packed
//	  form is MULPD then ADDPD — exactly the two instructions the gc compiler
//	  emits for the scalar oracle at the GOAMD64=v1 baseline (it does NOT fuse
//	  there), so at v1 the kernel is bit-identical; at v3 the scalar oracle
//	  itself fuses to VFMADD, a <=0.5 ULP regrouping, so axpy is validated to a
//	  tight relative tolerance rather than held bit-identical across GOAMD64
//	  levels.
package main

import (
	"fmt"
	"os"

	"github.com/go-asmgen/asmgen/amd64"
	"github.com/go-asmgen/asmgen/emit"
)

func main() {
	f := emit.NewFile("amd64")
	f.Add(axpyKernel())
	if err := os.WriteFile("simd_amd64.s", []byte(f.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote simd_amd64.s")
}

// axpyKernel builds axpySSE2(dst, src *float64, a float64, n int): dst[i] +=
// a*src[i]. The scalar a is broadcast into both lanes of X0; the loop processes
// 4 doubles (two XMM pairs) per iteration, with a scalar tail.
func axpyKernel() *emit.Function {
	sig := amd64.Layout(
		[]string{"dst", "src", "a", "n"},
		[]amd64.Type{amd64.Ptr, amd64.Ptr, amd64.Float64, amd64.Int64},
		nil, nil,
	)
	b := amd64.NewFunc("axpySSE2", sig, 0)
	b.LoadArg("dst", "AX").
		LoadArg("src", "BX").
		LoadArg("a", "X0").
		LoadArg("n", "CX").
		Raw("MOVDDUP X0, X0").
		Raw("block:").
		Raw("CMPQ CX, $4").
		Raw("JL tail").
		Raw("MOVUPD (BX), X1").
		Raw("MOVUPD 16(BX), X2").
		Raw("MULPD X0, X1").
		Raw("MULPD X0, X2").
		Raw("MOVUPD (AX), X3").
		Raw("MOVUPD 16(AX), X4").
		Raw("ADDPD X1, X3").
		Raw("ADDPD X2, X4").
		Raw("MOVUPD X3, (AX)").
		Raw("MOVUPD X4, 16(AX)").
		Raw("ADDQ $32, AX").
		Raw("ADDQ $32, BX").
		Raw("SUBQ $4, CX").
		Raw("JMP block").
		Raw("tail:").
		Raw("TESTQ CX, CX").
		Raw("JZ done").
		Raw("MOVSD (BX), X1").
		Raw("MULSD X0, X1").
		Raw("MOVSD (AX), X2").
		Raw("ADDSD X1, X2").
		Raw("MOVSD X2, (AX)").
		Raw("ADDQ $8, AX").
		Raw("ADDQ $8, BX").
		Raw("DECQ CX").
		Raw("JMP tail").
		Raw("done:").
		Ret()
	return b.Func()
}
