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
// is made of a piece per segment, a piece per corner and a piece per end; they
// all go into one edge list, wound the same way round, and the whole of it is
// filled once under the nonzero rule.
//
// Rasterising each piece on its own and keeping the greater coverage would
// look like the same thing and is not. Two pieces that meet along a shared
// edge each cover part of the pixel that edge cuts, and the greater of two
// halves is a half. A finely cut curve — the way every plotting program writes
// one — is nothing but such seams, and would come out at half its colour,
// combed through with lighter notches at every vertex.
func (rz *Rasterizer) StrokeWith(pth *Path, style StrokeStyle, clampW, clampH int) (cov []float64, ox, oy, w, h int, ok bool) {
	if pth == nil || style.Width <= 0 {
		return nil, 0, 0, 0, 0, false
	}
	out := rz.strokeEdges(pth, style)
	if len(out.edges) == 0 {
		return nil, 0, 0, 0, 0, false
	}
	ox, oy, w, h, ok = clampBox(out.minX, out.minY, out.maxX, out.maxY, clampW, clampH)
	if !ok {
		return nil, 0, 0, 0, 0, false
	}
	cov = rz.covScratch(w * h)
	coverInto(cov, out.edges, NonZero, ox, oy, w, h, pathSS, &rz.sc)
	return cov, ox, oy, w, h, true
}

// polyline is one run of the stroke: a list of points and whether it closes
// back on itself.
type polyline struct {
	pts    []point
	closed bool
}

// A strokeOut collects what a stroke is made of: the edges of every piece, and
// the box the stroke truly covers. That box is not quite the box of those
// edges — a round end or corner is drawn as a polygon that falls a hair inside
// the circle it stands for, and the box has to be the circle's.
type strokeOut struct {
	edges                  []edge
	minX, minY, maxX, maxY float64
}

// reset empties the accumulator, keeping whatever room the last stroke needed.
func (o *strokeOut) reset(edges []edge) {
	o.edges = edges[:0]
	o.minX, o.minY = math.Inf(1), math.Inf(1)
	o.maxX, o.maxY = math.Inf(-1), math.Inf(-1)
}

// cover widens the box to take in one point.
func (o *strokeOut) cover(x, y float64) {
	o.minX, o.minY = math.Min(o.minX, x), math.Min(o.minY, y)
	o.maxX, o.maxY = math.Max(o.maxX, x), math.Max(o.maxY, y)
}

// poly adds a closed polygon, always wound the same way round. The nonzero
// rule counts which way an outline turns, so a piece wound against the others
// would cancel where the two overlap and cut a hole in the stroke.
func (o *strokeOut) poly(pts []point) {
	if signedArea(pts) < 0 {
		for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
			pts[i], pts[j] = pts[j], pts[i]
		}
	}
	for i := range pts {
		j := (i + 1) % len(pts)
		o.edges = append(o.edges, edge{pts[i].x, pts[i].y, pts[j].x, pts[j].y})
		o.cover(pts[i].x, pts[i].y)
	}
}

// segment adds the rectangle a straight run of the stroke covers. A segment
// that goes nowhere covers no rectangle; the cap or corner at that point is
// what shows.
func (o *strokeOut) segment(a, b point, hw float64) {
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	nx, ny := -dy/l*hw, dx/l*hw
	o.poly([]point{
		{a.x + nx, a.y + ny}, {b.x + nx, b.y + ny},
		{b.x - nx, b.y - ny}, {a.x - nx, a.y - ny},
	})
}

// strokeEdges works out every piece a stroke is made of, over the rasteriser's
// own scratch so a stream of strokes allocates nothing.
func (rz *Rasterizer) strokeEdges(pth *Path, style StrokeStyle) *strokeOut {
	hw := style.Width / 2
	o := &rz.so
	o.reset(o.edges)
	for _, line := range strokeLines(pth, style) {
		pts := line.pts
		for i := 0; i+1 < len(pts); i++ {
			o.segment(pts[i], pts[i+1], hw)
		}
		for i := 1; i+1 < len(pts); i++ {
			o.join(pts[i-1], pts[i], pts[i+1], style, hw)
		}
		if line.closed && len(pts) > 2 {
			// The point where the run meets itself is a corner like any other.
			o.join(pts[len(pts)-2], pts[0], pts[1], style, hw)
			continue
		}
		o.cap(pts[1], pts[0], style, hw)
		o.cap(pts[len(pts)-2], pts[len(pts)-1], style, hw)
	}
	return o
}

// signedArea is twice the area a polygon encloses, negative when it is wound
// the other way round.
func signedArea(pts []point) float64 {
	sum := 0.0
	for i := range pts {
		j := (i + 1) % len(pts)
		sum += pts[i].x*pts[j].y - pts[j].x*pts[i].y
	}
	return sum
}

// wedge adds the pie slice at c between the rim points p1 and p2, going the
// way round that sweeps less than a full turn past them. Its rim is stepped as
// finely as a whole disc of the same size would be.
func (o *strokeOut) wedge(c, p1, p2 point, r float64) {
	a1 := math.Atan2(p1.y-c.y, p1.x-c.x)
	a2 := math.Atan2(p2.y-c.y, p2.x-c.x)
	sweep := math.Mod(a2-a1, 2*math.Pi)
	if sweep < 0 {
		sweep += 2 * math.Pi
	}
	if sweep > math.Pi {
		sweep -= 2 * math.Pi
	}
	// At least one flat, however slight the turn: a wedge of no width still
	// has to be a polygon rather than a division by nothing.
	step := 2 * math.Pi / float64(discSteps(r))
	n := 1 + int(math.Abs(sweep)/step)
	pts := make([]point, 0, n+2)
	pts = append(pts, c)
	for i := 0; i <= n; i++ {
		a := a1 + sweep*float64(i)/float64(n)
		pts = append(pts, point{c.x + r*math.Cos(a), c.y + r*math.Sin(a)})
	}
	o.poly(pts)
}

// disc adds a disc as a polygon fine enough that its rim cannot be told from a
// circle. The corners sit on the circle and the flats fall just inside it,
// never outside — so the pieces never spill past where the geometry says the
// stroke reaches, and the box is widened to the circle rather than to the
// polygon.
func (o *strokeOut) disc(cx, cy, r float64) {
	n := discSteps(r)
	pts := make([]point, n)
	for i := range pts {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = point{cx + r*math.Cos(a), cy + r*math.Sin(a)}
	}
	o.poly(pts)
	o.cover(cx-r, cy-r)
	o.cover(cx+r, cy+r)
}

// discTolerance is how far inside the true rim a disc's outline may fall — a
// hundredth of a pixel, which is well under what a coverage value can show.
const discTolerance = 0.01

// discSteps is how many flats a disc of the given radius needs to stay within
// that. A large disc needs more of them; past a point more would say nothing a
// pixel could show, so there is a ceiling.
func discSteps(r float64) int {
	n := 8
	if c := 1 - discTolerance/r; c > -1 {
		n = int(math.Ceil(math.Pi / math.Acos(c)))
	}
	switch {
	case n < 8:
		return 8
	case n > 512:
		return 512
	}
	return n
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

// join fills the corner at b, where the run turns from a towards c. Only the
// outside of the turn is left open by the two segments meeting there — the
// inside of it they cover between them — so that wedge is all any join draws,
// whatever shape it is. On a curve cut into thousands of segments, where each
// turn is a fraction of a degree, that is a sliver rather than a disc, which is
// most of what makes such a curve quick to draw.
func (o *strokeOut) join(a, b, c point, style StrokeStyle, hw float64) {
	inx, iny, ok1 := unit(b.x-a.x, b.y-a.y)
	outx, outy, ok2 := unit(c.x-b.x, c.y-b.y)
	if !ok1 || !ok2 {
		return
	}
	// Which side the corner opens on decides which pair of offsets to join.
	cross := inx*outy - iny*outx
	if cross == 0 {
		// Straight on leaves no corner to fill. Doubling back leaves the whole
		// of the far side of the corner open, and only a round join has
		// anything to put there: a miter would run away and a bevel would join
		// a point to itself.
		if style.Join == RoundJoin && inx*outx+iny*outy < 0 {
			o.disc(b.x, b.y, hw)
		}
		return
	}
	sign := 1.0
	if cross > 0 {
		sign = -1
	}
	p1 := point{b.x + sign*-iny*hw, b.y + sign*inx*hw}
	p2 := point{b.x + sign*-outy*hw, b.y + sign*outx*hw}
	if style.Join == RoundJoin {
		o.wedge(b, p1, p2, hw)
		return
	}
	if style.Join == MiterJoin {
		if m, ok := miterPoint(b, p1, p2, inx, iny, outx, outy, hw, style.MiterLimit); ok {
			o.poly([]point{b, p1, m, p2})
			return
		}
	}
	o.poly([]point{b, p1, p2})
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

// cap finishes the run at end, which the run reached from prev.
func (o *strokeOut) cap(prev, end point, style StrokeStyle, hw float64) {
	switch style.Cap {
	case RoundCap:
		dx, dy, ok := unit(end.x-prev.x, end.y-prev.y)
		if !ok {
			// A run that goes nowhere is a dot: the whole disc shows.
			o.disc(end.x, end.y, hw)
			return
		}
		// The half of the disc that lies past the end; the run itself covers
		// the other half. It is drawn as two quarters, because which way round
		// a half turn goes is not decided by its ends alone.
		far := point{end.x + dx*hw, end.y + dy*hw}
		o.wedge(end, point{end.x - dy*hw, end.y + dx*hw}, far, hw)
		o.wedge(end, far, point{end.x + dy*hw, end.y - dx*hw}, hw)
	case SquareCap:
		dx, dy, ok := unit(end.x-prev.x, end.y-prev.y)
		if !ok {
			// A run that goes nowhere still gets its square.
			dx, dy = 1, 0
		}
		o.segment(end, point{end.x + dx*hw, end.y + dy*hw}, hw)
	}
}

// unit is the direction of a vector, or false when it has none.
func unit(dx, dy float64) (x, y float64, ok bool) {
	l := math.Hypot(dx, dy)
	if l == 0 {
		return 0, 0, false
	}
	return dx / l, dy / l, true
}
