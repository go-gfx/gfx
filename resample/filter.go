package resample

import "math"

// A Filter is a separable resampling kernel: Support is the radius of its
// non-zero region at unit scale, and At evaluates the kernel at a signed
// distance x (in source pixels) from a sample centre. The bicubic and Lanczos
// modes are built from the two filters below; a caller does not construct
// Filters directly.
type Filter struct {
	Support float64
	At      func(x float64) float64
}

// cubicFilter is the Keys cubic with a = -1/2 — the Catmull-Rom spline, and
// Pillow's BICUBIC / x/image/draw's CatmullRom. Support 2: a destination sample
// draws on the four source pixels either side of it before the reduction
// widening.
var cubicFilter = Filter{Support: 2, At: cubicAt}

func cubicAt(x float64) float64 {
	// Keys cubic, a = -0.5: a positive central lobe out to |x| = 1 and a negative
	// side lobe from 1 to 2, zero beyond.
	const a = -0.5
	if x < 0 {
		x = -x
	}
	if x < 1 {
		return ((a+2)*x-(a+3))*x*x + 1
	}
	if x < 2 {
		return (((x-5)*x+8)*x - 4) * a
	}
	return 0
}

// lanczosFilter is the a = 3 Lanczos window (Pillow's LANCZOS, x/image/draw's
// Lanczos): a sinc reconstruction lobe times a sinc window three lobes wide.
// Support 3 — wider and sharper than the cubic at more cost.
var lanczosFilter = Filter{Support: 3, At: lanczosAt}

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	x *= math.Pi
	return math.Sin(x) / x
}

func lanczosAt(x float64) float64 {
	if x <= -3 || x >= 3 {
		return 0
	}
	return sinc(x) * sinc(x/3)
}
