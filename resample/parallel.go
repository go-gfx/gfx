package resample

import (
	"runtime"
	"sync"
)

// Multicore tiling. The separable passes partition their independent lines (rows
// or columns) into one contiguous band per worker and run the same per-line
// kernel on each band, so correctness is inherited from the serial path and the
// result is independent of the worker count. A size threshold keeps small images
// on the serial path, where goroutine scheduling would dominate.

// ParThreshold is the minimum number of output pixels at which a separable pass
// fans out across goroutines. Below it the serial path runs. It is a var so
// tests can force the parallel path on small images.
var ParThreshold = 1 << 14 // 16384 pixels (e.g. 128x128)

// numWorkers returns the worker count for n independent lines: at most
// GOMAXPROCS (always >= 1) and never more than n. Callers pass n >= 1.
func numWorkers(n int) int {
	w := runtime.GOMAXPROCS(0)
	if w > n {
		w = n
	}
	return w
}

// parallelLines splits [0,n) into w contiguous, near-equal bands and runs body
// on each in its own goroutine, waiting for all. body must be safe to call
// concurrently on disjoint [lo,hi) ranges (the separable passes write disjoint
// output rows/columns, so they are).
func parallelLines(n, w int, body func(lo, hi int)) {
	if w <= 1 {
		body(0, n)
		return
	}
	chunk := (n + w - 1) / w
	var wg sync.WaitGroup
	for lo := 0; lo < n; lo += chunk {
		hi := lo + chunk
		if hi > n {
			hi = n
		}
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			body(lo, hi)
		}(lo, hi)
	}
	wg.Wait()
}

// forLines runs body over [0,n), fanning out across goroutines when the work
// (n lines of lineWork elements each) crosses ParThreshold and running serially
// otherwise. The output is identical regardless of the worker count.
func forLines(n, lineWork int, body func(lo, hi int)) {
	if n*lineWork < ParThreshold {
		body(0, n)
		return
	}
	parallelLines(n, numWorkers(n), body)
}
