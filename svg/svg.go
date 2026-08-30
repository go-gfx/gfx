// Copyright (c) 2026, the go-gfx/gfx authors
// All rights reserved.
//
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

// Package svg parses a document from an SVG subset and rasterises it to a
// straight-alpha RGBA bitmap, built on the sibling [github.com/go-gfx/gfx/vector]
// and [github.com/go-gfx/gfx/raster] packages.
//
// It is not a general-purpose SVG renderer: it understands a practical subset —
// a root <svg> with a viewBox (or width/height), nested <g> groups carrying
// transforms and fills, <path> outlines (commands M/L/H/V/C/S/Q/T/Z, absolute
// and relative; arcs are unsupported and cause the path to be skipped), <rect>,
// <circle>, nested <svg> islands with their own viewBox, and embedded raster
// <image> data URIs.
//
// Fills and STROKES resolve black/white/currentColor/none, #rgb and #rrggbb
// literals, and url(#id) references to a <linearGradient> or <radialGradient> —
// in objectBoundingBox units (the SVG default, placed across the filled shape's
// own box) or userSpaceOnUse. A stroke honours stroke-width, carried through the
// transform as the length it is; caps and joins are round. <rect> honours rx/ry.
// An ink recolour and a transparent-or-opaque paper background are available.
//
// A value this subset does not implement leaves the shape UNPAINTED rather than
// inheriting: silently flooding it with the ancestor's colour is what rendered
// every gradient-filled document as a flat square. Malformed shapes are skipped
// rather than failing the page.
//
// Text is NOT rendered: <text> needs a font, which this package deliberately does
// not depend on.
//
// Every <g> element is reported in [Result.Groups] together with the
// device-pixel bounding box of everything it drew. That is the generic seam a
// consumer uses to map a group attribute back to a screen region: filter
// [Result.Groups] by whatever attribute you care about — id, class, aria-*, a
// custom data-* marker — and read its [Group.Bounds]. The package itself
// assigns no meaning to any particular attribute.
package svg

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"image"
	"image/color"
	_ "image/jpeg" // register JPEG decoder for embedded images
	_ "image/png"  // register PNG decoder for embedded images
	"math"
	"strconv"
	"strings"

	"github.com/go-gfx/gfx/raster"
	"github.com/go-gfx/gfx/vector"
)

// Options controls rasterization.
type Options struct {
	Scale float64    // device pixels per SVG user-unit (pt). <=0 defaults to 2.0
	Ink   color.RGBA // colour used for the default black / currentColor fills
	Paper color.RGBA // page background fill (the document's fill="white" rect). If Paper.A==0, no background is painted (transparent).
}

// Group records one <g> element: its attributes (namespace-insensitive local
// names) and the device-pixel bounding box of everything drawn within it,
// including nested descendants. A group that drew nothing has an empty Bounds
// (see [image.Rectangle.Empty]). Groups are reported in document order.
type Group struct {
	Attrs  map[string]string
	Bounds image.Rectangle
}

// Result is a rasterized SVG document.
type Result struct {
	Image  *raster.Image // straight-alpha RGBA surface, W*H pixels
	Groups []Group       // one entry per <g> element, in document order
}

// xnode is a generic XML element used to load the whole document tree.
type xnode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []xnode    `xml:",any"`
}

// attr returns the value of the named attribute (namespace-insensitive) and
// whether it was present.
func (n *xnode) attr(name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

// attrOr returns the named attribute or def when absent.
func (n *xnode) attrOr(name, def string) string {
	if v, ok := n.attr(name); ok {
		return v
	}
	return def
}

// attrMap returns every attribute keyed by its namespace-insensitive local
// name. When two attributes share a local name the last one wins.
func (n *xnode) attrMap() map[string]string {
	m := make(map[string]string, len(n.Attrs))
	for _, a := range n.Attrs {
		m[a.Name.Local] = a.Value
	}
	return m
}

// errNoSVG is returned when the document has no parseable <svg> root.
var errNoSVG = errors.New("svg: no parseable <svg> root")

// state is the inherited rendering context flowing down the element tree.
type state struct {
	m         matrix     // composed user->device transform
	fill      color.RGBA // resolved current fill colour
	paint     bool       // whether the current fill paints anything
	fillRef   string     // id of the paint server filling the shape, "" for a plain colour
	stroke    color.RGBA // resolved current stroke colour
	strokeOn  bool       // whether the outline is drawn
	strokeRef string     // id of the paint server stroking the shape
	strokeW   float64    // stroke width in USER units; scaled by m at paint time
	groups    []int      // indices into renderer.groups of the enclosing <g> ancestors
	vpW, vpH  float64    // current viewport size, for percentage lengths
}

// renderer holds the target surface and per-page options.
type renderer struct {
	img    *raster.Image
	ink    color.RGBA
	paper  color.RGBA
	groups []Group
	grads  map[string]gradient // paint servers, by id
}

// gradient is one <linearGradient> or <radialGradient>. Coordinates are kept as
// authored: in the default objectBoundingBox units they are fractions of the
// filled shape's bounding box, which is why a gradient cannot be turned into a
// [vector.Paint] until the shape being painted is known.
type gradient struct {
	radial     bool
	x1, y1     float64 // linear: start point
	x2, y2     float64 // linear: end point
	cx, cy, rr float64 // radial: centre and radius
	userSpace  bool    // gradientUnits="userSpaceOnUse"
	stops      []vector.Stop
}

// collectGradients walks the whole document once, recording every paint server it
// finds by id. A pre-pass is required because SVG allows a shape to reference a
// gradient declared after it.
func (r *renderer) collectGradients(n *xnode) {
	switch n.XMLName.Local {
	case "linearGradient", "radialGradient":
		if id, ok := n.attr("id"); ok && id != "" {
			r.grads[id] = parseGradient(n)
		}
	}
	for i := range n.Children {
		r.collectGradients(&n.Children[i])
	}
}

// parseGradient reads one paint server element and its <stop> children.
func parseGradient(n *xnode) gradient {
	g := gradient{
		radial:    n.XMLName.Local == "radialGradient",
		x2:        1, // SVG default: x1,y1,y2 = 0, x2 = 1
		cx:        0.5,
		cy:        0.5,
		rr:        0.5,
		userSpace: n.attrOr("gradientUnits", "") == "userSpaceOnUse",
	}
	num := func(name string, dst *float64) {
		if v, ok := n.attr(name); ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, "%")), 64); err == nil {
				if strings.HasSuffix(strings.TrimSpace(v), "%") {
					f /= 100
				}
				*dst = f
			}
		}
	}
	num("x1", &g.x1)
	num("y1", &g.y1)
	num("x2", &g.x2)
	num("y2", &g.y2)
	num("cx", &g.cx)
	num("cy", &g.cy)
	num("r", &g.rr)
	for i := range n.Children {
		c := &n.Children[i]
		if c.XMLName.Local != "stop" {
			continue
		}
		off := 0.0
		if v, ok := c.attr("offset"); ok {
			t := strings.TrimSpace(v)
			if f, err := strconv.ParseFloat(strings.TrimSuffix(t, "%"), 64); err == nil {
				if strings.HasSuffix(t, "%") {
					f /= 100
				}
				off = f
			}
		}
		col, ok := parseHexColor(c.attrOr("stop-color", ""))
		if !ok {
			switch strings.TrimSpace(c.attrOr("stop-color", "")) {
			case "white":
				col = color.RGBA{R: 255, G: 255, B: 255, A: 255}
			case "black":
				col = color.RGBA{A: 255}
			default:
				continue
			}
		}
		if v, ok := c.attr("stop-opacity"); ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f >= 0 && f <= 1 {
				col.A = uint8(f * 255)
			}
		}
		g.stops = append(g.stops, vector.Stop{Offset: off, Color: col})
	}
	return g
}

// paintExists reports whether a paint reference can be resolved. A reference to
// an id the document never declares must leave the shape UNPAINTED: treating it
// as an inherited colour is what flooded every gradient-filled logo with the
// black root fill.
func (r *renderer) paintExists(ref string) bool {
	if ref == "" {
		return true
	}
	_, ok := r.grads[ref]
	return ok
}

// paintFor turns a resolved paint into a [vector.Paint] for the box the shape
// covers. An objectBoundingBox gradient is placed across that box; a
// userSpaceOnUse one is placed by the current transform.
func (r *renderer) paintFor(st state, ref string, flat color.RGBA, ox, oy, w, h int) vector.Paint {
	g, ok := r.grads[ref]
	if !ok || len(g.stops) == 0 {
		flat.A = 255
		return vector.SolidPaint{Color: flat}
	}
	mapPt := func(x, y float64) (float64, float64) {
		if g.userSpace {
			return st.m.apply(x, y)
		}
		return float64(ox) + x*float64(w), float64(oy) + y*float64(h)
	}
	if g.radial {
		cx, cy := mapPt(g.cx, g.cy)
		rr := g.rr * float64(w)
		if g.userSpace {
			rr = g.rr * st.m.scale()
		}
		return vector.NewRadialGradient(cx, cy, rr, vector.Pad, g.stops...)
	}
	x1, y1 := mapPt(g.x1, g.y1)
	x2, y2 := mapPt(g.x2, g.y2)
	return vector.NewLinearGradient(x1, y1, x2, y2, vector.Pad, g.stops...)
}

// Rasterize parses ONE SVG document and renders it to a Result. It returns an
// error only for a document with no parseable <svg> root.
func Rasterize(doc string, opt Options) (*Result, error) {
	var root xnode
	if err := xml.Unmarshal([]byte(doc), &root); err != nil {
		return nil, errNoSVG
	}
	if root.XMLName.Local != "svg" {
		return nil, errNoSVG
	}

	scale := opt.Scale
	if scale <= 0 {
		scale = 2.0
	}

	vbMinX, vbMinY, vbW, vbH, ok := rootViewport(&root)
	if !ok {
		return nil, errNoSVG
	}

	w := int(math.Round(vbW * scale))
	h := int(math.Round(vbH * scale))
	if w <= 0 || h <= 0 {
		return nil, errNoSVG
	}

	ink := opt.Ink
	if ink == (color.RGBA{}) {
		ink = color.RGBA{0, 0, 0, 255}
	}

	r := &renderer{
		img:   raster.New(w, h),
		ink:   ink,
		paper: opt.Paper,
	}

	// Opaque background: prefill the whole surface with Paper.
	if opt.Paper.A > 0 {
		for i := 0; i+3 < len(r.img.Pix); i += 4 {
			r.img.Pix[i] = opt.Paper.R
			r.img.Pix[i+1] = opt.Paper.G
			r.img.Pix[i+2] = opt.Paper.B
			r.img.Pix[i+3] = opt.Paper.A
		}
	}

	r.grads = map[string]gradient{}
	r.collectGradients(&root)

	st := state{
		// Scale to device pixels AND translate the viewBox minimum to the origin,
		// so a viewBox like "0 -960 960 960" lands its content on the raster.
		m:       matrix{scale, 0, 0, scale, -vbMinX * scale, -vbMinY * scale},
		fill:    ink,
		paint:   true,
		strokeW: 1, // the SVG default
		vpW:     vbW,
		vpH:     vbH,
	}
	// The root <svg> carries presentation attributes like any other element, and
	// they are inherited by everything inside it.
	st = r.applyPaintAttrs(&root, st)
	for i := range root.Children {
		r.render(&root.Children[i], st)
	}

	return &Result{
		Image:  r.img,
		Groups: r.groups,
	}, nil
}

// rootViewport determines the SVG user-space origin (minX, minY) and dimensions
// (w, h) from viewBox (preferred) or width/height. A viewBox may carry a
// non-zero — even negative — minimum (e.g. "0 -960 960 960", common in
// Material-style icon sets); the caller translates by (-minX, -minY) so that
// content lands on the raster instead of off it.
func rootViewport(root *xnode) (minX, minY, w, h float64, ok bool) {
	if vb, ok := root.attr("viewBox"); ok {
		f := parseFloats(vb)
		if len(f) == 4 && f[2] > 0 && f[3] > 0 {
			return f[0], f[1], f[2], f[3], true
		}
	}
	w = parseLen(root.attrOr("width", ""), 0)
	h = parseLen(root.attrOr("height", ""), 0)
	if w > 0 && h > 0 {
		return 0, 0, w, h, true
	}
	return 0, 0, 0, 0, false
}

// render walks one element, deriving a child state and drawing leaf shapes.
// notRendered are elements whose children define something for later use and are
// never drawn where they stand: <defs> holds reusable content, <clipPath> and
// <mask> hold shapes that clip or mask rather than paint, and <symbol>,
// <marker> and <pattern> are templates a <use> or a paint reference instantiates.
//
// Painting them is not a small error. pgf wraps every clipped picture in a
// <clipPath> whose shape is the picture's own frame, so drawing it laid an
// opaque plate over the whole figure — a pgfplots axis came out as a black
// rectangle with the data drawn on top of it.
var notRendered = map[string]bool{
	"defs": true, "clipPath": true, "mask": true,
	"symbol": true, "marker": true, "pattern": true,
}

// applyPaintAttrs folds an element's presentation attributes into the inherited
// state. It is separate from render because the ROOT <svg> needs it too, and the
// root is not rendered: Rasterize walks its children.
//
// That gap silently changed what an icon pack looks like. Iconoir puts
// fill="none" and stroke-width="1.5" on the root and stroke="currentColor" on
// each path, so with the root's attributes dropped every closed path inherited
// the default fill and a magnifier came out as a solid disc — 40% of the box
// inked instead of a thin outline — while the open handle path still stroked
// correctly and made the result look deliberate.
func (r *renderer) applyPaintAttrs(n *xnode, st state) state {
	if v, ok := n.attr("fill"); ok {
		st.fill, st.paint, st.fillRef = resolveFill(v, st.fill, st.paint, r.ink, r.paper)
		st.paint = st.paint && r.paintExists(st.fillRef)
	}
	if v, ok := n.attr("stroke"); ok {
		st.stroke, st.strokeOn, st.strokeRef = resolveFill(v, st.stroke, st.strokeOn, r.ink, r.paper)
		st.strokeOn = st.strokeOn && r.paintExists(st.strokeRef)
	}
	if v, ok := n.attr("stroke-width"); ok {
		// An explicit width of zero switches the outline OFF, so the value is
		// taken whatever it is rather than only when positive. A width this
		// subset cannot parse reads as zero, which matches the package's rule
		// that a malformed shape is skipped rather than guessed at.
		st.strokeW = parseLen(v, st.vpW)
	}
	return st
}

func (r *renderer) render(n *xnode, parent state) {
	if notRendered[n.XMLName.Local] {
		return
	}
	st := parent

	if v, ok := n.attr("transform"); ok {
		st.m = parent.m.mul(parseTransform(v))
	}
	st = r.applyPaintAttrs(n, st)
	if n.XMLName.Local == "g" {
		idx := len(r.groups)
		r.groups = append(r.groups, Group{Attrs: n.attrMap()})
		// Extend the ancestor list with a fresh slice so sibling groups never
		// alias each other's backing array.
		ng := make([]int, len(parent.groups)+1)
		copy(ng, parent.groups)
		ng[len(parent.groups)] = idx
		st.groups = ng
	}

	switch n.XMLName.Local {
	case "path":
		r.drawPath(n, st)
	case "rect":
		r.drawRect(n, st)
	case "circle":
		r.drawCircle(n, st)
	case "polygon":
		r.drawPoly(n, st, true)
	case "polyline":
		r.drawPoly(n, st, false)
	case "image":
		r.drawImage(n, st)
	case "svg":
		// A nested <svg> island: apply x/y offset and viewBox scaling, then
		// descend into its children as a group.
		st.m = r.nestedTransform(n, st)
		if vb, ok := n.attr("viewBox"); ok {
			if f := parseFloats(vb); len(f) == 4 && f[2] > 0 && f[3] > 0 {
				st.vpW, st.vpH = f[2], f[3]
			}
		}
		for i := range n.Children {
			r.render(&n.Children[i], st)
		}
	default:
		// <g> and any other container: just descend.
		for i := range n.Children {
			r.render(&n.Children[i], st)
		}
	}
}

// nestedTransform builds the transform for a nested <svg>: the element's own
// transform (already folded into st.m by render) composed with translate(x,y)
// and, if a viewBox is present, an extra scale mapping the viewBox onto
// width/height.
func (r *renderer) nestedTransform(n *xnode, st state) matrix {
	x := parseLen(n.attrOr("x", "0"), st.vpW)
	y := parseLen(n.attrOr("y", "0"), st.vpH)
	m := st.m.mul(matrix{1, 0, 0, 1, x, y})
	if vb, ok := n.attr("viewBox"); ok {
		f := parseFloats(vb)
		if len(f) == 4 && f[2] > 0 && f[3] > 0 {
			w := parseLen(n.attrOr("width", ""), st.vpW)
			h := parseLen(n.attrOr("height", ""), st.vpH)
			if w > 0 && h > 0 {
				m = m.mul(matrix{w / f[2], 0, 0, h / f[3], 0, 0})
			}
		}
	}
	return m
}

// addBounds unions a device-space rectangle into every enclosing <g> group's
// bounding box.
func (r *renderer) addBounds(st state, rect image.Rectangle) {
	for _, gi := range st.groups {
		r.groups[gi].Bounds = r.groups[gi].Bounds.Union(rect)
	}
}

// fillPath rasterizes a device-space path: first its interior, then its outline,
// recording the covered bounding box against the enclosing groups.
//
// The two are composited in turn rather than gathered first, because a
// Rasterizer's coverage grid aliases its own scratch buffer and is only valid
// until the next Fill or Stroke call.
func (r *renderer) fillPath(p *vector.Path, st state) {
	var rz vector.Rasterizer
	if st.paint {
		if cov, ox, oy, w, h, ok := rz.Fill(p, vector.NonZero, r.img.W, r.img.H); ok {
			vector.Composite(r.img, cov, ox, oy, w, h, r.paintFor(st, st.fillRef, st.fill, ox, oy, w, h))
			r.addBounds(st, image.Rect(ox, oy, ox+w, oy+h))
		}
	}
	if !st.strokeOn || st.strokeW <= 0 {
		return
	}
	// A stroke width is a length in user units, so it passes through the
	// transform like any other length.
	if cov, ox, oy, w, h, ok := rz.Stroke(p, st.strokeW*st.m.scale(), r.img.W, r.img.H); ok {
		vector.Composite(r.img, cov, ox, oy, w, h, r.paintFor(st, st.strokeRef, st.stroke, ox, oy, w, h))
		r.addBounds(st, image.Rect(ox, oy, ox+w, oy+h))
	}
}

// drawPath fills a <path> element.
func (r *renderer) drawPath(n *xnode, st state) {
	d, ok := n.attr("d")
	if !ok {
		return
	}
	p, ok := buildPath(d, st.m)
	if !ok {
		return
	}
	r.fillPath(p, st)
}

// drawRect fills a <rect> element.
func (r *renderer) drawRect(n *xnode, st state) {
	if !st.paint && !st.strokeOn {
		return
	}
	x := parseLen(n.attrOr("x", "0"), st.vpW)
	y := parseLen(n.attrOr("y", "0"), st.vpH)
	w := parseLen(n.attrOr("width", "0"), st.vpW)
	h := parseLen(n.attrOr("height", "0"), st.vpH)
	if w <= 0 || h <= 0 {
		return
	}
	rx := parseLen(n.attrOr("rx", n.attrOr("ry", "0")), st.vpW)
	ry := parseLen(n.attrOr("ry", n.attrOr("rx", "0")), st.vpH)
	if rx > w/2 {
		rx = w / 2
	}
	if ry > h/2 {
		ry = h / 2
	}
	p := vector.NewPath()
	if rx > 0 && ry > 0 {
		buildRoundRect(p, x, y, w, h, rx, ry, st.m)
	} else {
		x0, y0 := st.m.apply(x, y)
		x1, y1 := st.m.apply(x+w, y)
		x2, y2 := st.m.apply(x+w, y+h)
		x3, y3 := st.m.apply(x, y+h)
		p.MoveTo(x0, y0)
		p.LineTo(x1, y1)
		p.LineTo(x2, y2)
		p.LineTo(x3, y3)
		p.Close()
	}
	r.fillPath(p, st)
}

// drawCircle fills a <circle> element, approximating the disc with four cubics.
func (r *renderer) drawCircle(n *xnode, st state) {
	if !st.paint && !st.strokeOn {
		return
	}
	cx := parseLen(n.attrOr("cx", "0"), st.vpW)
	cy := parseLen(n.attrOr("cy", "0"), st.vpH)
	rr := parseLen(n.attrOr("r", "0"), st.vpW)
	if rr <= 0 {
		return
	}
	const k = 0.5522847498307936
	p := vector.NewPath()
	move := func(x, y float64) { dx, dy := st.m.apply(x, y); p.MoveTo(dx, dy) }
	cube := func(c1x, c1y, c2x, c2y, x, y float64) {
		a1x, a1y := st.m.apply(c1x, c1y)
		a2x, a2y := st.m.apply(c2x, c2y)
		ex, ey := st.m.apply(x, y)
		p.CubicTo(a1x, a1y, a2x, a2y, ex, ey)
	}
	move(cx+rr, cy)
	cube(cx+rr, cy+k*rr, cx+k*rr, cy+rr, cx, cy+rr)
	cube(cx-k*rr, cy+rr, cx-rr, cy+k*rr, cx-rr, cy)
	cube(cx-rr, cy-k*rr, cx-k*rr, cy-rr, cx, cy-rr)
	cube(cx+k*rr, cy-rr, cx+rr, cy-k*rr, cx+rr, cy)
	p.Close()
	r.fillPath(p, st)
}

// drawPoly fills a <polygon> (closed) or draws a <polyline> (left open) from its
// "points" list — a flat run of x,y pairs separated by commas and/or whitespace.
// Fewer than two points draws nothing. Fill and stroke follow the paint state
// exactly like every other shape (an open polyline with a fill fills as if
// closed, the SVG rule).
func (r *renderer) drawPoly(n *xnode, st state, closed bool) {
	if !st.paint && !st.strokeOn {
		return
	}
	pts, ok := n.attr("points")
	if !ok {
		return
	}
	f := parseFloats(pts)
	if len(f) < 4 {
		return
	}
	p := vector.NewPath()
	x0, y0 := st.m.apply(f[0], f[1])
	p.MoveTo(x0, y0)
	for i := 2; i+1 < len(f); i += 2 {
		x, y := st.m.apply(f[i], f[i+1])
		p.LineTo(x, y)
	}
	if closed {
		p.Close()
	}
	r.fillPath(p, st)
}

// drawImage decodes an embedded raster <image> and blits it into the transformed
// destination rectangle with nearest-neighbour sampling. Decode failures are
// silently skipped.
func (r *renderer) drawImage(n *xnode, st state) {
	href, ok := n.attr("href")
	if !ok {
		if href, ok = n.attr("xlink:href"); !ok {
			return
		}
	}
	data, ok := decodeDataURI(href)
	if !ok {
		return
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return
	}
	x := parseLen(n.attrOr("x", "0"), st.vpW)
	y := parseLen(n.attrOr("y", "0"), st.vpH)
	w := parseLen(n.attrOr("width", "0"), st.vpW)
	h := parseLen(n.attrOr("height", "0"), st.vpH)
	if w <= 0 || h <= 0 {
		return
	}

	// Destination bounding box from the four transformed corners.
	xs := []float64{}
	ys := []float64{}
	for _, c := range [][2]float64{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}} {
		dx, dy := st.m.apply(c[0], c[1])
		xs = append(xs, dx)
		ys = append(ys, dy)
	}
	dx0 := int(math.Floor(minf(xs)))
	dy0 := int(math.Floor(minf(ys)))
	dx1 := int(math.Ceil(maxf(xs)))
	dy1 := int(math.Ceil(maxf(ys)))
	dx0, dy0 = clampi(dx0, 0, r.img.W), clampi(dy0, 0, r.img.H)
	dx1, dy1 = clampi(dx1, 0, r.img.W), clampi(dy1, 0, r.img.H)
	if dx1 <= dx0 || dy1 <= dy0 {
		return
	}

	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	spanX := float64(dx1 - dx0)
	spanY := float64(dy1 - dy0)
	// Nearest-neighbour: map each destination pixel centre onto a source index
	// in [0, sw-1] / [0, sh-1]. Rounding against (dim-1) keeps the index in range
	// without a boundary clamp.
	for py := dy0; py < dy1; py++ {
		fy := (float64(py-dy0) + 0.5) / spanY
		syc := sb.Min.Y + int(math.Round(fy*float64(sh-1)))
		for px := dx0; px < dx1; px++ {
			fx := (float64(px-dx0) + 0.5) / spanX
			sxc := sb.Min.X + int(math.Round(fx*float64(sw-1)))
			cr, cg, cb, ca := src.At(sxc, syc).RGBA()
			off := r.img.PixOffset(px, py)
			blendOver(r.img.Pix[off:off+4], uint8(cr>>8), uint8(cg>>8), uint8(cb>>8), uint8(ca>>8))
		}
	}
	r.addBounds(st, image.Rect(dx0, dy0, dx1, dy1))
}

// blendOver composites a straight-alpha source pixel over a straight-alpha
// destination pixel in place.
func blendOver(dst []uint8, sr, sg, sb, sa uint8) {
	if sa == 0 {
		return
	}
	if sa == 255 {
		dst[0], dst[1], dst[2], dst[3] = sr, sg, sb, 255
		return
	}
	af := float64(sa) / 255
	da := float64(dst[3]) / 255
	outA := af + da*(1-af) // > 0 because af > 0 here
	blend := func(s, d uint8) uint8 {
		sv := float64(s) / 255 * af
		dv := float64(d) / 255 * da * (1 - af)
		return uint8(math.Round((sv + dv) / outA * 255))
	}
	dst[0] = blend(sr, dst[0])
	dst[1] = blend(sg, dst[1])
	dst[2] = blend(sb, dst[2])
	dst[3] = uint8(math.Round(outA * 255))
}

// decodeDataURI extracts the raw bytes of a base64 data: URI. It reports
// ok=false for non-data or non-base64 URIs.
func decodeDataURI(href string) ([]byte, bool) {
	href = strings.TrimSpace(href)
	if !strings.HasPrefix(href, "data:") {
		return nil, false
	}
	comma := strings.IndexByte(href, ',')
	if comma < 0 {
		return nil, false
	}
	meta := href[len("data:"):comma]
	if !strings.Contains(meta, "base64") {
		return nil, false
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(href[comma+1:]))
	if err != nil {
		return nil, false
	}
	return data, true
}

func minf(v []float64) float64 {
	m := v[0]
	for _, x := range v[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

func maxf(v []float64) float64 {
	m := v[0]
	for _, x := range v[1:] {
		if x > m {
			m = x
		}
	}
	return m
}

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
