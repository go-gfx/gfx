package resample

import "math"

// This file holds the low-level resampling kernels operating on straight-alpha
// RGBA byte planes (srcW*srcH -> dstW*dstH). The public entry points in
// resample.go wrap them over raster.Image. The separable filtered passes route
// their hot vertical accumulation through the axpy SIMD seam (simd*.go).

// clampByte rounds v to the nearest byte, clamping to [0, 255].
func clampByte(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// clampIndex clamps i to [0, n).
func clampIndex(i, n int) int {
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

// resizeNearest selects, for each destination pixel, the source pixel nearest
// the point it maps back to. Exact for integer enlargements, blocky otherwise.
func resizeNearest(dst, src []uint8, srcW, srcH, dstW, dstH int) {
	for y := 0; y < dstH; y++ {
		sy := y * srcH / dstH
		for x := 0; x < dstW; x++ {
			sx := x * srcW / dstW
			si := (sy*srcW + sx) * 4
			di := (y*dstW + x) * 4
			dst[di] = src[si]
			dst[di+1] = src[si+1]
			dst[di+2] = src[si+2]
			dst[di+3] = src[si+3]
		}
	}
}

// resizeBilinear scales src into dst by bilinear interpolation, sampling at the
// centre of each destination pixel mapped back into source space with
// clamp-to-edge addressing.
func resizeBilinear(dst, src []uint8, srcW, srcH, dstW, dstH int) {
	scaleX := float64(srcW) / float64(dstW)
	scaleY := float64(srcH) / float64(dstH)
	for y := 0; y < dstH; y++ {
		fy := (float64(y)+0.5)*scaleY - 0.5
		y0 := int(math.Floor(fy))
		wy := fy - float64(y0)
		y0c := clampIndex(y0, srcH)
		y1c := clampIndex(y0+1, srcH)
		for x := 0; x < dstW; x++ {
			fx := (float64(x)+0.5)*scaleX - 0.5
			x0 := int(math.Floor(fx))
			wx := fx - float64(x0)
			x0c := clampIndex(x0, srcW)
			x1c := clampIndex(x0+1, srcW)
			i00 := (y0c*srcW + x0c) * 4
			i01 := (y0c*srcW + x1c) * 4
			i10 := (y1c*srcW + x0c) * 4
			i11 := (y1c*srcW + x1c) * 4
			di := (y*dstW + x) * 4
			for c := 0; c < 4; c++ {
				top := float64(src[i00+c])*(1-wx) + float64(src[i01+c])*wx
				bot := float64(src[i10+c])*(1-wx) + float64(src[i11+c])*wx
				dst[di+c] = clampByte(top*(1-wy) + bot*wy)
			}
		}
	}
}

// resizeArea scales src into dst by area (box) averaging: each destination pixel
// is the mean of the source region it covers, every source pixel weighted by how
// much of it falls inside. Separated horizontal-then-vertical through a float64
// scratch. This is PIL's Image.BOX / OpenCV's INTER_AREA; at integer ratios it
// is the block mean (scikit-image downscale_local_mean); enlarging it reduces to
// nearest-neighbour.
func resizeArea(dst, src []uint8, srcW, srcH, dstW, dstH int) {
	xs, xc, xw := boxSpans(srcW, dstW)
	ys, yc, yw := boxSpans(srcH, dstH)

	tmp := make([]float64, srcH*dstW*4)
	for y := 0; y < srcH; y++ {
		srcRow, tmpRow, wi := y*srcW*4, y*dstW*4, 0
		for x := 0; x < dstW; x++ {
			var acc [4]float64
			var sum float64
			for k := 0; k < xc[x]; k++ {
				w := xw[wi+k]
				si := srcRow + (xs[x]+k)*4
				acc[0] += float64(src[si]) * w
				acc[1] += float64(src[si+1]) * w
				acc[2] += float64(src[si+2]) * w
				acc[3] += float64(src[si+3]) * w
				sum += w
			}
			wi += xc[x]
			ti := tmpRow + x*4
			tmp[ti] = acc[0] / sum
			tmp[ti+1] = acc[1] / sum
			tmp[ti+2] = acc[2] / sum
			tmp[ti+3] = acc[3] / sum
		}
	}

	wi := 0
	for y := 0; y < dstH; y++ {
		dstRow := y * dstW * 4
		for x := 0; x < dstW; x++ {
			var acc [4]float64
			var sum float64
			for k := 0; k < yc[y]; k++ {
				w := yw[wi+k]
				ti := (ys[y]+k)*dstW*4 + x*4
				acc[0] += tmp[ti] * w
				acc[1] += tmp[ti+1] * w
				acc[2] += tmp[ti+2] * w
				acc[3] += tmp[ti+3] * w
				sum += w
			}
			di := dstRow + x*4
			dst[di] = clampByte(acc[0] / sum)
			dst[di+1] = clampByte(acc[1] / sum)
			dst[di+2] = clampByte(acc[2] / sum)
			dst[di+3] = clampByte(acc[3] / sum)
		}
		wi += yc[y]
	}
}

// resampleFiltered scales src into dst with the separable filter f (bicubic or
// Lanczos). The two axes are separated — horizontal into a float64 scratch
// plane, then vertical — so the cost is proportional to the pixels crossed
// rather than to the product of the two kernel widths.
//
// With premultiply set the colour channels are multiplied by alpha before
// filtering and divided out afterwards (the transparent-edge fringe fix); with
// it clear every channel, alpha included, is filtered independently.
func resampleFiltered(dst, src []uint8, srcW, srcH, dstW, dstH int, f Filter, premultiply bool) {
	xc := filterCoeffs(srcW, dstW, f)
	yc := filterCoeffs(srcH, dstH, f)

	rowLen := dstW * 4
	tmp := make([]float64, srcH*rowLen)
	forLines(srcH, dstW, func(lo, hi int) {
		resampleH(tmp, src, srcW, dstW, lo, hi, xc, premultiply)
	})
	forLines(dstH, dstW, func(lo, hi int) {
		resampleV(dst, tmp, rowLen, lo, hi, yc, premultiply)
	})
}

// resampleH filters source rows [lo,hi) horizontally into the float64 plane tmp.
// For each destination column it gathers its (few, scattered) source columns —
// the pass a future dedicated SIMD gather kernel would accelerate; the vertical
// pass already rides the shipped axpy kernel.
func resampleH(tmp []float64, src []uint8, srcW, dstW, lo, hi int, c coeffs, premul bool) {
	for y := lo; y < hi; y++ {
		srcRow := y * srcW * 4
		ti := y * dstW * 4
		for x := 0; x < dstW; x++ {
			st, n, o := c.starts[x], c.counts[x], c.offs[x]
			var r, g, b, a float64
			for k := 0; k < n; k++ {
				wk := c.weights[o+k]
				si := srcRow + (st+k)*4
				av := float64(src[si+3])
				if premul {
					av255 := av / 255
					r += float64(src[si]) * av255 * wk
					g += float64(src[si+1]) * av255 * wk
					b += float64(src[si+2]) * av255 * wk
				} else {
					r += float64(src[si]) * wk
					g += float64(src[si+1]) * wk
					b += float64(src[si+2]) * wk
				}
				a += av * wk
			}
			tmp[ti], tmp[ti+1], tmp[ti+2], tmp[ti+3] = r, g, b, a
			ti += 4
		}
	}
}

// resampleV filters the float64 plane vertically into the byte destination for
// output rows [lo,hi). Each output row is a handful of source rows scaled by the
// tap weights and summed — one flat axpy per tap over a whole contiguous row, so
// amd64/arm64/s390x drive it with their SIMD axpy kernel and the other targets
// with the scalar oracle.
func resampleV(dst []uint8, tmp []float64, rowLen, lo, hi int, c coeffs, premul bool) {
	acc := make([]float64, rowLen)
	for y := lo; y < hi; y++ {
		for i := range acc {
			acc[i] = 0
		}
		st, n, o := c.starts[y], c.counts[y], c.offs[y]
		for k := 0; k < n; k++ {
			row := (st + k) * rowLen
			axpy(acc, tmp[row:row+rowLen], c.weights[o+k])
		}
		di := y * rowLen
		if premul {
			for x := 0; x < rowLen; x += 4 {
				a := acc[x+3]
				ab := clampByte(a)
				// Divide the colour back out of alpha where the pixel is at all
				// visible. Keying on the ROUNDED alpha leaves a pixel that rounds to
				// fully transparent with clean zero colour instead of the bright value
				// a near-zero alpha would divide up to — which is what Pillow does.
				// ab != 0 implies a >= 0.5 > 0, so the division never hits zero; a
				// signed kernel can still push a visible channel past its alpha or
				// below zero at a hard edge, so clamp.
				if ab != 0 {
					s := 255 / a
					dst[di+x] = clampByte(acc[x] * s)
					dst[di+x+1] = clampByte(acc[x+1] * s)
					dst[di+x+2] = clampByte(acc[x+2] * s)
				} else {
					dst[di+x], dst[di+x+1], dst[di+x+2] = 0, 0, 0
				}
				dst[di+x+3] = ab
			}
		} else {
			for i := 0; i < rowLen; i++ {
				dst[di+i] = clampByte(acc[i])
			}
		}
	}
}
