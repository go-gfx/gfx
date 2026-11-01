// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package vector

import (
	"math"
	"strconv"
)

// Affine is a 2x3 transform written [a b c d e f], mapping a point (x, y) to
// (a*x + c*y + e, b*x + d*y + f).
//
// ⛔ THE ORDER IS THE TRAP, not the arithmetic. SVG's transform="matrix(...)",
// XPS's RenderTransform, ODF's draw:transform and PDF's `cm` operator all write
// an affine as these same six numbers in this same order, which is why one type
// serves all of them -- but the six are NOT a row-major 2x3 matrix, and reading
// them as one silently swaps b with c. That mistake leaves every unrotated
// drawing correct and every rotated one mirrored, so it survives a test suite
// built from rectangles.
type Affine [6]float64

// Identity is the transform that changes nothing.
func Identity() Affine { return Affine{1, 0, 0, 1, 0, 0} }

// Apply maps a point through the transform.
func (m Affine) Apply(x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

// pathScanner tokenises a path "d" string into command letters and numbers.
type pathScanner struct {
	s   string
	i   int
	bad bool
}

// skipSep advances over whitespace and commas.
func (p *pathScanner) skipSep() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r', ',':
			p.i++
		default:
			return
		}
	}
}

// isCmd reports whether c is a path command letter.
func isCmd(c byte) bool {
	switch c {
	case 'M', 'm', 'L', 'l', 'H', 'h', 'V', 'v', 'C', 'c',
		'S', 's', 'Q', 'q', 'T', 't', 'Z', 'z', 'A', 'a':
		return true
	}
	return false
}

// nextCmd returns the next command letter, or 0 at end of input. A non-command,
// non-numeric byte sets bad.
func (p *pathScanner) nextCmd() byte {
	p.skipSep()
	if p.i >= len(p.s) {
		return 0
	}
	c := p.s[p.i]
	if !isCmd(c) {
		p.bad = true
		return 0
	}
	p.i++
	return c
}

// number parses the next float. On failure it sets bad and returns 0.
func (p *pathScanner) number() float64 {
	p.skipSep()
	start := p.i
	if p.i < len(p.s) && (p.s[p.i] == '+' || p.s[p.i] == '-') {
		p.i++
	}
	seenDigit := false
	seenDot := false
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c >= '0' && c <= '9' {
			seenDigit = true
			p.i++
			continue
		}
		if c == '.' {
			// Only one decimal point per number: a second dot begins the NEXT
			// number, as in the compact SVG "1.4.1" (two coordinates 1.4 and .1).
			if seenDot {
				break
			}
			seenDot = true
			p.i++
			continue
		}
		if (c == 'e' || c == 'E') && seenDigit {
			p.i++
			if p.i < len(p.s) && (p.s[p.i] == '+' || p.s[p.i] == '-') {
				p.i++
			}
			continue
		}
		break
	}
	if !seenDigit {
		p.bad = true
		return 0
	}
	v, err := strconv.ParseFloat(p.s[start:p.i], 64)
	if err != nil {
		p.bad = true
		return 0
	}
	return v
}

// flag parses a single arc flag: exactly one '0' or '1', which SVG allows to run
// straight into the next number with no separator ("0 1 5,5", "0110 20"). On
// anything else it sets bad.
func (p *pathScanner) flag() float64 {
	p.skipSep()
	if p.i < len(p.s) && (p.s[p.i] == '0' || p.s[p.i] == '1') {
		v := float64(p.s[p.i] - '0')
		p.i++
		return v
	}
	p.bad = true
	return 0
}

// arcToCubics converts one elliptical-arc segment (endpoint parameterisation:
// from (x1,y1) to (x2,y2), radii rx/ry, x-axis rotation phiDeg in degrees, the
// large-arc and sweep flags) into a series of cubic Béziers, each spanning at
// most a quarter turn — the standard approximation. Coordinates are in user
// space; the caller applies its transform. A degenerate arc (zero radius or
// coincident endpoints) yields no segments, so the subpath just skips it.
func arcToCubics(x1, y1, rx, ry, phiDeg float64, largeArc, sweep bool, x2, y2 float64) [][6]float64 {
	if rx == 0 || ry == 0 || (x1 == x2 && y1 == y2) {
		return nil
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	phi := phiDeg * math.Pi / 180
	cosP, sinP := math.Cos(phi), math.Sin(phi)

	// Step 1: (x1', y1') in the rotated frame.
	dx, dy := (x1-x2)/2, (y1-y2)/2
	x1p := cosP*dx + sinP*dy
	y1p := -sinP*dx + cosP*dy

	// Correct out-of-range radii.
	if lam := x1p*x1p/(rx*rx) + y1p*y1p/(ry*ry); lam > 1 {
		s := math.Sqrt(lam)
		rx, ry = rx*s, ry*s
	}

	// Step 2: centre in the rotated frame.
	num := rx*rx*ry*ry - rx*rx*y1p*y1p - ry*ry*x1p*x1p
	den := rx*rx*y1p*y1p + ry*ry*x1p*x1p
	co := 0.0
	if den != 0 {
		co = math.Sqrt(math.Max(0, num/den))
	}
	if largeArc == sweep {
		co = -co
	}
	cxp := co * rx * y1p / ry
	cyp := -co * ry * x1p / rx

	// Centre in user space.
	cx := cosP*cxp - sinP*cyp + (x1+x2)/2
	cy := sinP*cxp + cosP*cyp + (y1+y2)/2

	ang := func(ux, uy, vx, vy float64) float64 {
		d := ux*vx + uy*vy
		// The vectors here run from the ellipse centre to points ON the ellipse
		// (radii already non-zero), so their lengths are positive — no zero-length
		// guard is needed.
		l := math.Hypot(ux, uy) * math.Hypot(vx, vy)
		a := math.Acos(math.Max(-1, math.Min(1, d/l)))
		if ux*vy-uy*vx < 0 {
			a = -a
		}
		return a
	}
	theta1 := ang(1, 0, (x1p-cxp)/rx, (y1p-cyp)/ry)
	dtheta := ang((x1p-cxp)/rx, (y1p-cyp)/ry, (-x1p-cxp)/rx, (-y1p-cyp)/ry)
	if !sweep && dtheta > 0 {
		dtheta -= 2 * math.Pi
	}
	if sweep && dtheta < 0 {
		dtheta += 2 * math.Pi
	}

	// At least one Bézier segment: the endpoints differ and the radii are
	// non-zero (both guarded above), so the swept angle is non-zero and the
	// quarter-turn ceiling is at least 1.
	n := int(math.Ceil(math.Abs(dtheta) / (math.Pi / 2)))
	delta := dtheta / float64(n)
	tan := 4.0 / 3.0 * math.Tan(delta/4)

	pt := func(th float64) (float64, float64) {
		ex, ey := rx*math.Cos(th), ry*math.Sin(th)
		return cosP*ex - sinP*ey + cx, sinP*ex + cosP*ey + cy
	}
	rot := func(a, b float64) (float64, float64) { return cosP*a - sinP*b, sinP*a + cosP*b }

	var segs [][6]float64
	th := theta1
	for i := 0; i < n; i++ {
		th2 := th + delta
		xi, yi := pt(th)
		xe, ye := pt(th2)
		tix, tiy := rot(-rx*math.Sin(th), ry*math.Cos(th))
		tex, tey := rot(-rx*math.Sin(th2), ry*math.Cos(th2))
		segs = append(segs, [6]float64{
			xi + tan*tix, yi + tan*tiy,
			xe - tan*tex, ye - tan*tey,
			xe, ye,
		})
		th = th2
	}
	return segs
}

// more reports whether the next non-separator byte can begin a number, marking a
// repeated implicit coordinate set for the current command.
func (p *pathScanner) more() bool {
	p.skipSep()
	if p.i >= len(p.s) {
		return false
	}
	c := p.s[p.i]
	return c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9')
}

// up upper-cases a command letter.
func up(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 32
	}
	return c
}

// ParseData parses a path-data string and returns the outline it describes,
// with m applied to every coordinate.
//
// The syntax is SVG's "d" attribute: M/m L/l H/h V/v C/c S/s Q/q T/t A/a Z/z,
// absolute and relative, with implicit repetition of a command's operands and
// SVG's compact number forms ("1.4.1" is two numbers, and the arc flags may run
// into the coordinate that follows). Arcs are converted to cubics.
//
// # One parser, three formats
//
// It lives here rather than in the SVG reader because the same syntax is what
// ODF writes in draw:path's svg:d, and what OpenXPS calls its ABBREVIATED
// GEOMETRY SYNTAX. ECMA-388 Annex C gives that syntax as an algorithm, and
// reading it against this one shows XPS to be a strict subset with a prefix:
//
//   - the same command letters, except that XPS has no T/t;
//   - the same arc operands in the same order, and the same sense for the
//     sweep flag (ECMA-388's "FLAG2 == 1 ? Clockwise");
//   - plus a leading F0 / F1 selecting the fill rule, which belongs to the
//     SHAPE and not to the outline. A caller strips it and uses EvenOdd or
//     NonZero accordingly.
//
// So an XPS or ODF reader hands its geometry here and gets the arc maths, the
// smooth-curve reflection and the number scanning that SVG already needed,
// rather than a third transcription of them.
//
// ⚠ It accepts T/t whatever the caller is reading, so it is laxer than XPS.
// Being liberal about an input nobody can write by accident is cheaper than a
// mode flag, and a converter's job is to read what it is given.
//
// ok is false when the data names a command this does not know, begins a
// drawing command with no preceding move, or is otherwise malformed — so a
// caller can skip ONE shape rather than fail the page. It is also false when
// the data draws nothing at all: a lone move, or a move and a close, has no
// interior and no outline, and returning an empty Path would make a caller
// paint nothing while believing it painted something.
func ParseData(d string, m Affine) (*Path, bool) {
	sc := &pathScanner{s: d}
	path := NewPath()

	var (
		curX, curY     float64 // current point, user space
		startX, startY float64 // subpath start, user space
		prevCX, prevCY float64 // last cubic control, user space
		prevQX, prevQY float64 // last quad control, user space
		hasStart       bool
		prevCmd        byte
		emitted        bool
	)

	moveTo := func(x, y float64) {
		dx, dy := m.Apply(x, y)
		path.MoveTo(dx, dy)
	}
	lineTo := func(x, y float64) {
		dx, dy := m.Apply(x, y)
		path.LineTo(dx, dy)
		emitted = true
	}
	quadTo := func(cx, cy, x, y float64) {
		dcx, dcy := m.Apply(cx, cy)
		dx, dy := m.Apply(x, y)
		path.QuadTo(dcx, dcy, dx, dy)
		emitted = true
	}
	cubicTo := func(c1x, c1y, c2x, c2y, x, y float64) {
		d1x, d1y := m.Apply(c1x, c1y)
		d2x, d2y := m.Apply(c2x, c2y)
		dx, dy := m.Apply(x, y)
		path.CubicTo(d1x, d1y, d2x, d2y, dx, dy)
		emitted = true
	}

	for {
		cmd := sc.nextCmd()
		if sc.bad {
			return nil, false
		}
		if cmd == 0 {
			break
		}
		abs := cmd >= 'A' && cmd <= 'Z'

		switch up(cmd) {
		case 'M':
			first := true
			for {
				x := sc.number()
				y := sc.number()
				if sc.bad {
					return nil, false
				}
				if !abs && hasStart {
					x += curX
					y += curY
				}
				curX, curY = x, y
				if first {
					hasStart = true
					startX, startY = curX, curY
					moveTo(curX, curY)
					first = false
				} else {
					lineTo(curX, curY)
				}
				if !sc.more() {
					break
				}
			}
		case 'L':
			if !hasStart {
				return nil, false
			}
			for {
				x := sc.number()
				y := sc.number()
				if sc.bad {
					return nil, false
				}
				if !abs {
					x += curX
					y += curY
				}
				curX, curY = x, y
				lineTo(curX, curY)
				if !sc.more() {
					break
				}
			}
		case 'H':
			if !hasStart {
				return nil, false
			}
			for {
				x := sc.number()
				if sc.bad {
					return nil, false
				}
				if !abs {
					x += curX
				}
				curX = x
				lineTo(curX, curY)
				if !sc.more() {
					break
				}
			}
		case 'V':
			if !hasStart {
				return nil, false
			}
			for {
				y := sc.number()
				if sc.bad {
					return nil, false
				}
				if !abs {
					y += curY
				}
				curY = y
				lineTo(curX, curY)
				if !sc.more() {
					break
				}
			}
		case 'C':
			if !hasStart {
				return nil, false
			}
			for {
				c1x := sc.number()
				c1y := sc.number()
				c2x := sc.number()
				c2y := sc.number()
				x := sc.number()
				y := sc.number()
				if sc.bad {
					return nil, false
				}
				if !abs {
					c1x += curX
					c1y += curY
					c2x += curX
					c2y += curY
					x += curX
					y += curY
				}
				cubicTo(c1x, c1y, c2x, c2y, x, y)
				prevCX, prevCY = c2x, c2y
				curX, curY = x, y
				if !sc.more() {
					break
				}
			}
		case 'S':
			if !hasStart {
				return nil, false
			}
			for {
				c2x := sc.number()
				c2y := sc.number()
				x := sc.number()
				y := sc.number()
				if sc.bad {
					return nil, false
				}
				if !abs {
					c2x += curX
					c2y += curY
					x += curX
					y += curY
				}
				c1x, c1y := curX, curY
				if prevCmd == 'C' || prevCmd == 'S' {
					c1x = 2*curX - prevCX
					c1y = 2*curY - prevCY
				}
				cubicTo(c1x, c1y, c2x, c2y, x, y)
				prevCX, prevCY = c2x, c2y
				curX, curY = x, y
				prevCmd = 'S'
				if !sc.more() {
					break
				}
			}
		case 'Q':
			if !hasStart {
				return nil, false
			}
			for {
				cx := sc.number()
				cy := sc.number()
				x := sc.number()
				y := sc.number()
				if sc.bad {
					return nil, false
				}
				if !abs {
					cx += curX
					cy += curY
					x += curX
					y += curY
				}
				quadTo(cx, cy, x, y)
				prevQX, prevQY = cx, cy
				curX, curY = x, y
				if !sc.more() {
					break
				}
			}
		case 'T':
			if !hasStart {
				return nil, false
			}
			for {
				x := sc.number()
				y := sc.number()
				if sc.bad {
					return nil, false
				}
				if !abs {
					x += curX
					y += curY
				}
				cx, cy := curX, curY
				if prevCmd == 'Q' || prevCmd == 'T' {
					cx = 2*curX - prevQX
					cy = 2*curY - prevQY
				}
				quadTo(cx, cy, x, y)
				prevQX, prevQY = cx, cy
				curX, curY = x, y
				prevCmd = 'T'
				if !sc.more() {
					break
				}
			}
		case 'A':
			if !hasStart {
				return nil, false
			}
			for {
				rx := sc.number()
				ry := sc.number()
				rot := sc.number()
				large := sc.flag()
				sweep := sc.flag()
				x := sc.number()
				y := sc.number()
				if sc.bad {
					return nil, false
				}
				if !abs {
					x += curX
					y += curY
				}
				for _, s := range arcToCubics(curX, curY, rx, ry, rot, large != 0, sweep != 0, x, y) {
					cubicTo(s[0], s[1], s[2], s[3], s[4], s[5])
				}
				curX, curY = x, y
				if !sc.more() {
					break
				}
			}
		case 'Z':
			path.Close()
			curX, curY = startX, startY
		}

		// Record the command family for the next smooth-curve reflection. S and
		// T set prevCmd themselves inside their loops.
		switch up(cmd) {
		case 'S', 'T':
			// already recorded
		default:
			prevCmd = up(cmd)
		}
	}

	if !emitted {
		return nil, false
	}
	return path, true
}
