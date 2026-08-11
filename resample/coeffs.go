package resample

import "math"

// coeffs holds the per-destination-index sampling plan of a separable pass:
// which source samples each output index reads and their weights, laid out flat.
// starts[i] is the first source index output i reads, counts[i] how many,
// offs[i] where its weights begin in weights, and weights holds them
// consecutively.
type coeffs struct {
	starts, counts, offs []int
	weights              []float64
}

// filterCoeffs precomputes the resampling coefficients for a filtered pass from
// inSize source samples to outSize destination samples, following Pillow's
// precompute_coeffs (Resample.c).
//
// The destination sample centre of index i is (i+0.5)*scale where scale =
// in/out. On a reduction (scale > 1) the filter footprint is widened by the
// reduction factor, turning the reconstruction filter into a matched low-pass
// that antialiases; on an enlargement it stays at unit width so the kernel
// interpolates. Each output pixel's weights are normalised to sum to one.
func filterCoeffs(inSize, outSize int, f Filter) coeffs {
	scale := float64(inSize) / float64(outSize)
	filterscale := scale
	if filterscale < 1 {
		filterscale = 1
	}
	support := f.Support * filterscale
	ss := 1 / filterscale
	c := coeffs{
		starts:  make([]int, outSize),
		counts:  make([]int, outSize),
		offs:    make([]int, outSize),
		weights: make([]float64, 0, outSize*(int(math.Ceil(support))*2+1)),
	}
	for i := 0; i < outSize; i++ {
		center := (float64(i) + 0.5) * scale
		// The read window [center-support, center+support], rounded inward by the
		// truncating casts and clamped to the image. It always keeps at least one
		// sample: center lies in (0, inSize) and inside the window, and support >= 2
		// makes the pre-clamp width exceed three, so counts[i] >= 1 on every ratio.
		xmin := int(center - support + 0.5)
		if xmin < 0 {
			xmin = 0
		}
		xmax := int(center + support + 0.5)
		if xmax > inSize {
			xmax = inSize
		}
		n := xmax - xmin
		c.starts[i], c.counts[i], c.offs[i] = xmin, n, len(c.weights)
		var ww float64
		for j := 0; j < n; j++ {
			w := f.At((float64(xmin+j) - center + 0.5) * ss)
			c.weights = append(c.weights, w)
			ww += w
		}
		// ww is the kernel's zero-frequency response over the covered span — ~1 for
		// a reconstruction filter and always strictly positive for the cubic and
		// Lanczos kernels — so the normalisation never divides by zero (the same
		// reasoning that lets boxSpans divide by its coverage sum).
		base := c.offs[i]
		for j := 0; j < n; j++ {
			c.weights[base+j] /= ww
		}
	}
	return c
}

// boxSpans precomputes, for each destination index, which source pixels it
// overlaps and by how much for an area (box) average. A destination pixel covers
// the source interval [i*scale, (i+1)*scale); a source pixel's weight is the
// length of that interval falling inside it. The weights are coverage, not a
// kernel, and are positive, so a caller divides its accumulator by their sum.
func boxSpans(srcN, dstN int) (starts, counts []int, weights []float64) {
	scale := float64(srcN) / float64(dstN)
	starts = make([]int, dstN)
	counts = make([]int, dstN)
	weights = make([]float64, 0, dstN+srcN)
	for i := 0; i < dstN; i++ {
		lo := float64(i) * scale
		hi := lo + scale
		// Both clamps guard a float that overshoots srcN by one ulp; as branches
		// they are unreachable on any ratio a test can express, so they are kept
		// branch-free. lo is never negative, so its floor needs no clamp.
		i0 := int(math.Floor(lo))
		i1 := max(min(int(math.Ceil(hi)), srcN), i0+1)
		starts[i], counts[i] = i0, i1-i0
		for j := i0; j < i1; j++ {
			weights = append(weights, math.Min(hi, float64(j+1))-math.Max(lo, float64(j)))
		}
	}
	return starts, counts, weights
}
