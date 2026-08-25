package vector

import "math"

// A LineCap says how the open end of a stroke is finished.
type LineCap int

// The three caps every drawing model has.
const (
	// ButtCap stops the stroke square at the end point.
	ButtCap LineCap = iota
	// RoundCap finishes it with a half disc.
	RoundCap
	// SquareCap carries it half a width past the end point.
	SquareCap
)

// A LineJoin says how two segments of a stroke meet.
type LineJoin int

// The three joins every drawing model has.
const (
	// MiterJoin carries the two outer edges to where they cross, unless that
	// point is further away than the miter limit allows, in which case the
	// join is bevelled instead.
	MiterJoin LineJoin = iota
	// RoundJoin fills the corner with a disc.
	RoundJoin
	// BevelJoin closes the corner with a straight line.
	BevelJoin
)

// defaultMiterLimit is the ratio every drawing model starts from: a corner
// sharper than about eleven degrees is bevelled rather than spiked.
const defaultMiterLimit = 10

// A StrokeStyle is everything about a stroke except where it goes.
//
// The zero value is butt caps and miter joins, which is what PostScript, PDF,
// SVG and Canvas all start from — set Width to something useful.
type StrokeStyle struct {
	Width      float64
	Cap        LineCap
	Join       LineJoin
	MiterLimit float64 // zero means the usual ten
	// Dash is the pattern of on and off lengths, beginning with an on length.
	// An odd pattern repeats to become even, so a single length means equal
	// dashes and gaps. An empty pattern draws a solid line.
	Dash      []float64
	DashPhase float64
}

// Stroke rasterises the path as a line of the given width into a per-pixel
// coverage grid (row-major, values 0..1) over the integer box [ox,oy,w,h]
// clamped to the [0,clampW) x [0,clampH) surface, with round caps and round
// joins. It is [Rasterizer.StrokeWith] with the style that needs no decisions.
//
// It returns ok=false — and an untouched grid — for a nil path, width <= 0, a
// path with no strokeable segment, or one whose bounds fall entirely off the
// clamp surface. The returned grid aliases the Rasterizer's scratch and is
// valid only until the next Fill or Stroke.
func (rz *Rasterizer) Stroke(pth *Path, width float64, clampW, clampH int) (cov []float64, ox, oy, w, h int, ok bool) {
	return rz.StrokeWith(pth, StrokeStyle{Width: width, Cap: RoundCap, Join: RoundJoin}, clampW, clampH)
}

// StrokeWith rasterises the path as a line drawn in the given style. A stroke
// is the union of a piece per segment, a piece per corner and a piece per end,
// taken as a per-pixel coverage MAX so that overlapping pieces never
// double-darken.
func (rz *Rasterizer) StrokeWith(pth *Path, style StrokeStyle, clampW, clampH int) (cov []float64, ox, oy, w, h int, ok bool) {
	if pth == nil || style.Width <= 0 {
		return nil, 0, 0, 0, 0, false
	}
	pieces := strokePieces(pth, style)
	if len(pieces) == 0 {
		return nil, 0, 0, 0, 0, false
	}
	minX, minY, maxX, maxY := piecesBounds(pieces)
	ox, oy, w, h, ok = clampBox(minX, minY, maxX, maxY, clampW, clampH)
	if !ok {
		return nil, 0, 0, 0, 0, false
	}
	cov = rz.covScratch(w * h)
	for _, p := range pieces {
		if p.edges == nil {
			diskMax(cov, ox, oy, w, h, p.cx, p.cy, p.r)
			continue
		}
		rz.polyMax(cov, ox, oy, w, h, p.edges)
	}
	return cov, ox, oy, w, h, true
}

// A strokePiece is one part of a stroke: a polygon, or a disc when edges is
// nil.
type strokePiece struct {
	edges     []edge
	cx, cy, r float64
}

// piecesBounds is the box every piece fits inside.
func piecesBounds(pieces []strokePiece) (minX, minY, maxX, maxY float64) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for _, p := range pieces {
		a, b, c, d := p.cx-p.r, p.cy-p.r, p.cx+p.r, p.cy+p.r
		if p.edges != nil {
			a, b, c, d = edgeBounds(p.edges)
		}
		minX, minY = math.Min(minX, a), math.Min(minY, b)
		maxX, maxY = math.Max(maxX, c), math.Max(maxY, d)
	}
	return minX, minY, maxX, maxY
}

// polyline is one run of the stroke: a list of points and whether it closes
// back on itself.
type polyline struct {
	pts    []point
	closed bool
}

// strokePieces works out every piece a stroke is made of.
func strokePieces(pth *Path, style StrokeStyle) []strokePiece {
	hw := style.Width / 2
	var out []strokePiece
	for _, line := range strokeLines(pth, style) {
		pts := line.pts
		for i := 0; i+1 < len(pts); i++ {
			if e := segRectEdges(pts[i].x, pts[i].y, pts[i+1].x, pts[i+1].y, hw); e != nil {
				out = append(out, strokePiece{edges: e})
			}
		}
		for i := 1; i+1 < len(pts); i++ {
			out = append(out, joinPieces(pts[i-1], pts[i], pts[i+1], style, hw)...)
		}
		if line.closed && len(pts) > 2 {
			// The point where the run meets itself is a corner like any other.
			out = append(out, joinPieces(pts[len(pts)-2], pts[0], pts[1], style, hw)...)
			continue
		}
		out = append(out, capPieces(pts[1], pts[0], style, hw)...)
		out = append(out, capPieces(pts[len(pts)-2], pts[len(pts)-1], style, hw)...)
	}
	return out
}

// strokeLines flattens a path and applies the dash pattern, giving the runs
// that actually get drawn. A sub-path with no segment in it is not strokeable
// and contributes nothing.
func strokeLines(pth *Path, style StrokeStyle) []polyline {
	var out []polyline
	for _, s := range pth.flatten(flattenTol) {
		if len(s.pts) < 2 {
			continue
		}
		pts := s.pts
		if s.closed && pts[len(pts)-1] != pts[0] {
			pts = append(append([]point{}, pts...), pts[0])
		}
		line := polyline{pts: pts, closed: s.closed}
		if !dashed(style) {
			out = append(out, line)
			continue
		}
		out = append(out, dashLine(line, style.Dash, style.DashPhase)...)
	}
	return out
}

// dashed reports whether a style asks for a broken line at all.
func dashed(style StrokeStyle) bool {
	total := 0.0
	for _, d := range style.Dash {
		if d < 0 {
			return false // a negative length is not a pattern
		}
		total += d
	}
	return len(style.Dash) > 0 && total > 0
}

// dashLine cuts a run into the pieces a dash pattern leaves behind.
func dashLine(line polyline, dash []float64, phase float64) []polyline {
	pattern := dash
	if len(pattern)%2 == 1 {
		// An odd pattern repeats to become even, so a single length means
		// equal dashes and gaps, which is what every drawing model does.
		pattern = append(append([]float64{}, dash...), dash...)
	}
	index, on, left := dashStart(pattern, phase)

	var out []polyline
	var cur []point
	if on {
		cur = []point{line.pts[0]}
	}
	// toggle ends the run in progress or begins one, at a point the
	// pattern has just reached. A length of zero simply toggles twice in
	// the same place, which leaves nothing behind — as it should.
	toggle := func(p point) {
		if on {
			cur = append(cur, p)
			if len(cur) > 1 {
				out = append(out, polyline{pts: cur})
			}
			cur = nil
		} else {
			cur = []point{p}
		}
		on = !on
	}
	for i := 0; i+1 < len(line.pts); i++ {
		a, b := line.pts[i], line.pts[i+1]
		segLen := math.Hypot(b.x-a.x, b.y-a.y)
		at := 0.0
		for segLen-at > left {
			at += left
			t := at / segLen
			toggle(point{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t})
			index, left = nextDash(pattern, index)
		}
		left -= segLen - at
		if on {
			cur = append(cur, b)
		}
	}
	if len(cur) > 1 {
		out = append(out, polyline{pts: cur})
	}
	return out
}

// nextDash steps to the following length in the pattern.
func nextDash(pattern []float64, index int) (int, float64) {
	index = (index + 1) % len(pattern)
	return index, pattern[index]
}

// dashStart works out where in the pattern a stroke begins, given its phase.
func dashStart(pattern []float64, phase float64) (index int, on bool, left float64) {
	total := 0.0
	for _, d := range pattern {
		total += d
	}
	if phase < 0 {
		phase = 0
	}
	phase = math.Mod(phase, total)
	on = true
	for index = 0; index+1 < len(pattern); index++ {
		if phase < pattern[index] {
			return index, on, pattern[index] - phase
		}
		phase -= pattern[index]
		on = !on
	}
	// Whatever is left belongs to the last length: the phase was reduced
	// modulo the whole pattern, so it cannot run past the end.
	return index, on, pattern[index] - phase
}

// joinPieces fills the corner at b, where the run turns from a towards c.
func joinPieces(a, b, c point, style StrokeStyle, hw float64) []strokePiece {
	if style.Join == RoundJoin {
		return []strokePiece{{cx: b.x, cy: b.y, r: hw}}
	}
	inx, iny, ok1 := unit(b.x-a.x, b.y-a.y)
	outx, outy, ok2 := unit(c.x-b.x, c.y-b.y)
	if !ok1 || !ok2 {
		return nil
	}
	// Which side the corner opens on decides which pair of offsets to join.
	cross := inx*outy - iny*outx
	if cross == 0 {
		return nil // straight on, or doubling back: no corner to fill
	}
	sign := 1.0
	if cross > 0 {
		sign = -1
	}
	p1 := point{b.x + sign*-iny*hw, b.y + sign*inx*hw}
	p2 := point{b.x + sign*-outy*hw, b.y + sign*outx*hw}
	if style.Join == MiterJoin {
		if m, ok := miterPoint(b, p1, p2, inx, iny, outx, outy, hw, style.MiterLimit); ok {
			return []strokePiece{{edges: polyEdges([]point{b, p1, m, p2})}}
		}
	}
	return []strokePiece{{edges: polyEdges([]point{b, p1, p2})}}
}

// miterPoint is where the two outer edges cross, when the corner is not so
// sharp that the spike would run away.
func miterPoint(b, p1, p2 point, inx, iny, outx, outy, hw, limit float64) (point, bool) {
	if limit <= 0 {
		limit = defaultMiterLimit
	}
	// The miter reaches 1/sin(theta/2) half-widths, where theta is the angle
	// between the two segments. Straight on, that is one; a hairpin sends it
	// to infinity, which is what the limit is for.
	cosTurn := inx*outx + iny*outy
	halfCos := math.Sqrt(math.Max(0, (1+cosTurn)/2))
	if halfCos <= 0 || 1/halfCos > limit {
		return point{}, false
	}
	// The crossing lies along the bisector of the two offset points.
	// halfCos above is greater than zero, so the two offsets are not
	// opposite and their sum has a direction.
	bx, by, _ := unit((p1.x-b.x)+(p2.x-b.x), (p1.y-b.y)+(p2.y-b.y))
	return point{b.x + bx*hw/halfCos, b.y + by*hw/halfCos}, true
}

// capPieces finishes the run at end, which the run reached from prev.
func capPieces(prev, end point, style StrokeStyle, hw float64) []strokePiece {
	switch style.Cap {
	case RoundCap:
		return []strokePiece{{cx: end.x, cy: end.y, r: hw}}
	case SquareCap:
		dx, dy, ok := unit(end.x-prev.x, end.y-prev.y)
		if !ok {
			// A run that goes nowhere still gets its square.
			dx, dy = 1, 0
		}
		return []strokePiece{{edges: segRectEdges(end.x, end.y, end.x+dx*hw, end.y+dy*hw, hw)}}
	}
	return nil
}

// unit is the direction of a vector, or false when it has none.
func unit(dx, dy float64) (x, y float64, ok bool) {
	l := math.Hypot(dx, dy)
	if l == 0 {
		return 0, 0, false
	}
	return dx / l, dy / l, true
}

// polyEdges turns a closed polygon into the edges a rasteriser wants.
func polyEdges(pts []point) []edge {
	out := make([]edge, 0, len(pts))
	for i := range pts {
		j := (i + 1) % len(pts)
		out = append(out, edge{pts[i].x, pts[i].y, pts[j].x, pts[j].y})
	}
	return out
}

// polyMax rasterises one piece of a stroke and unions it into the accumulator.
// Coverage is computed from absolute pixel positions, so a piece drawn over
// its own small box gives the same values as over the whole one.
func (rz *Rasterizer) polyMax(cov []float64, ox, oy, w, h int, edges []edge) {
	minX, minY, maxX, maxY := edgeBounds(edges)
	sox, soy, sw, sh, ok := subBox(minX, minY, maxX, maxY, ox, oy, w, h)
	if !ok {
		return
	}
	tmp := rz.tmpScratch(sw * sh)
	rz.xs = coverInto(tmp, edges, NonZero, sox, soy, sw, sh, pathSS, rz.xs[:0])
	maxSub(cov, w, tmp, sox-ox, soy-oy, sw, sh)
}
