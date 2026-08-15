package color

import "testing"

// srgbClose reports whether two gamma sRGB triples agree within tol per channel.
func srgbClose(r1, g1, b1, r2, g2, b2, tol float64) bool {
	return approx(r1, r2, tol) && approx(g1, g2, tol) && approx(b1, b2, tol)
}

// Every space that owns an sRGB<->space pair round-trips back to the original
// sRGB colour across the whole synthetic sample set, exercising each inverse
// (HSVToSRGB, HSLToSRGB, HWBToSRGB, LCHToSRGB, LuvToSRGB, OKLabToSRGB,
// OKLCHToSRGB and the LCh<->Cartesian inverses).
func TestRoundTripsAllSpaces(t *testing.T) {
	const tol = 1e-9
	for _, s := range oracleRows {
		r, g, b := s.r, s.g, s.b

		if rr, gg, bb := HSVToSRGB(SRGBToHSV(r, g, b)); !srgbClose(r, g, b, rr, gg, bb, tol) {
			t.Errorf("%s HSV round trip = (%v,%v,%v)", s.name, rr, gg, bb)
		}
		if rr, gg, bb := HSLToSRGB(SRGBToHSL(r, g, b)); !srgbClose(r, g, b, rr, gg, bb, tol) {
			t.Errorf("%s HSL round trip = (%v,%v,%v)", s.name, rr, gg, bb)
		}
		if rr, gg, bb := HWBToSRGB(SRGBToHWB(r, g, b)); !srgbClose(r, g, b, rr, gg, bb, tol) {
			t.Errorf("%s HWB round trip = (%v,%v,%v)", s.name, rr, gg, bb)
		}
		if rr, gg, bb := LCHToSRGB(SRGBToLCH(r, g, b)); !srgbClose(r, g, b, rr, gg, bb, 1e-5) {
			t.Errorf("%s LCh(ab) round trip = (%v,%v,%v)", s.name, rr, gg, bb)
		}
		if rr, gg, bb := LuvToSRGB(SRGBToLuv(r, g, b)); !srgbClose(r, g, b, rr, gg, bb, 1e-5) {
			t.Errorf("%s Luv round trip = (%v,%v,%v)", s.name, rr, gg, bb)
		}
		if rr, gg, bb := OKLabToSRGB(SRGBToOKLab(r, g, b)); !srgbClose(r, g, b, rr, gg, bb, 1e-5) {
			t.Errorf("%s OKLab round trip = (%v,%v,%v)", s.name, rr, gg, bb)
		}
		if rr, gg, bb := OKLCHToSRGB(SRGBToOKLCH(r, g, b)); !srgbClose(r, g, b, rr, gg, bb, 1e-5) {
			t.Errorf("%s OKLCH round trip = (%v,%v,%v)", s.name, rr, gg, bb)
		}
	}
}

// The cylindrical <-> Cartesian inverses of Luv (LCHuv) round-trip exactly.
func TestLCHuvRoundTrip(t *testing.T) {
	for _, s := range oracleRows {
		luv := SRGBToLuv(s.r, s.g, s.b)
		back := LCHToLuv(LuvToLCH(luv))
		if !approx(back.L, luv.L, 1e-9) || !approx(back.U, luv.U, 1e-9) || !approx(back.V, luv.V, 1e-9) {
			t.Errorf("%s LCh(uv) round trip = %+v, want %+v", s.name, back, luv)
		}
	}
}

// The XYZ<->Luv and XYZ<->Lab white-point-aware pairs round-trip under D50.
func TestXYZLuvLabD50RoundTrip(t *testing.T) {
	samples := []XYZ{{0.3, 0.4, 0.2}, {0.05, 0.02, 0.5}, {0.9, 0.9, 0.9}}
	for _, c := range samples {
		if back := LuvToXYZWP(XYZToLuvWP(c, D50), D50); !approx(back.X, c.X, 1e-9) ||
			!approx(back.Y, c.Y, 1e-9) || !approx(back.Z, c.Z, 1e-9) {
			t.Errorf("Luv(D50) round trip of %+v = %+v", c, back)
		}
		if back := LabToXYZWP(XYZToLabWP(c, D50), D50); !approx(back.X, c.X, 1e-9) ||
			!approx(back.Y, c.Y, 1e-9) || !approx(back.Z, c.Z, 1e-9) {
			t.Errorf("Lab(D50) round trip of %+v = %+v", c, back)
		}
	}
}
