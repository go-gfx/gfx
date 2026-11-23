//go:build !loong64

package color

// QuantizeUnitToByte clamps a channel value to [0, 1] and quantises it to an
// 8-bit value using round-half-to-even, matching scikit-image's img_as_ubyte
// (numpy.rint). It is the quantisation step image-level consumers apply after
// [SkimageLabToSRGB] / [SkimageXYZToSRGB] to reproduce scikit-image byte output.
func QuantizeUnitToByte(v float64) uint8 { return quantizeUnitToByte(v) }
