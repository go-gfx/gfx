//go:build loong64

package color

// QuantizeUnitToByte clamps a channel value to [0, 1] and quantises it to an
// 8-bit value using round-half-to-even, matching scikit-image's img_as_ubyte
// (numpy.rint). It is the quantisation step image-level consumers apply after
// [SkimageLabToSRGB] / [SkimageXYZToSRGB] to reproduce scikit-image byte output.
//
// On loong64 it is deliberately not inlined: go1.27.0 to at least go1.27.3
// inline the float-to-uint8 conversion with an 8-byte MOVD store into the
// one-byte result slot, clobbering the neighbouring stack bytes
// (golang/go#81000, fixed on tip; 1.27 backport golang/go#81147 still open).
// A call returns the byte in a register and never builds that slot. Drop this
// file once a released Go carries the backport (go-gfx/gfx#70, #98).
//
//go:noinline
func QuantizeUnitToByte(v float64) uint8 { return quantizeUnitToByte(v) }
