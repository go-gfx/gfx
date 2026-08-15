package color

import (
	"math"
	"testing"
)

// This file proves numeric parity of every colour-space conversion against the
// out-of-tree oracle colour-science (see oracle_data_test.go for the captured
// golden vectors) and against the published CIEDE2000 reference data. Tolerances
// are stated per space and reflect the largest observed deviation, which comes
// from the oracle rebuilding its white point from xy chromaticities rather than
// from any disagreement in the maths.

// hueClose compares two hue angles in degrees, tolerating wrap at 360 and
// treating the angle as meaningless below the chroma floor cfloor.
func hueClose(got, want, chroma, cfloor, tol float64) bool {
	if chroma < cfloor {
		return true // achromatic: hue is arbitrary.
	}
	d := math.Abs(math.Mod(got-want+540, 360) - 180)
	return d <= tol
}

func TestParityHSVHSL(t *testing.T) {
	for _, r := range oracleRows {
		hsv := SRGBToHSV(r.r, r.g, r.b)
		if !approx(hsv.S, r.hsvS, 1e-9) || !approx(hsv.V, r.hsvV, 1e-9) ||
			!hueClose(hsv.H, r.hsvH, hsv.S, 1e-12, 1e-6) {
			t.Errorf("%s SRGBToHSV = %+v, want H=%v S=%v V=%v", r.name, hsv, r.hsvH, r.hsvS, r.hsvV)
		}
		hsl := SRGBToHSL(r.r, r.g, r.b)
		if !approx(hsl.S, r.hslS, 1e-9) || !approx(hsl.L, r.hslL, 1e-9) ||
			!hueClose(hsl.H, r.hslH, hsl.S, 1e-12, 1e-6) {
			t.Errorf("%s SRGBToHSL = %+v, want H=%v S=%v L=%v", r.name, hsl, r.hslH, r.hslS, r.hslL)
		}
	}
}

func TestParityLabLCH(t *testing.T) {
	for _, r := range oracleRows {
		lab := SRGBToLab(r.r, r.g, r.b)
		if !approx(lab.L, r.labL, 1e-3) || !approx(lab.A, r.labA, 1e-3) || !approx(lab.B, r.labB, 1e-3) {
			t.Errorf("%s SRGBToLab = %+v, want (%v,%v,%v)", r.name, lab, r.labL, r.labA, r.labB)
		}
		lch := LabToLCH(lab)
		if !approx(lch.C, r.lchC, 1e-3) || !hueClose(lch.H, r.lchH, lch.C, 1e-3, 1e-3) {
			t.Errorf("%s LabToLCH = %+v, want C=%v H=%v", r.name, lch, r.lchC, r.lchH)
		}
	}
}

func TestParityLuvLCHuv(t *testing.T) {
	for _, r := range oracleRows {
		luv := SRGBToLuv(r.r, r.g, r.b)
		if !approx(luv.L, r.luvL, 1e-3) || !approx(luv.U, r.luvU, 1e-3) || !approx(luv.V, r.luvV, 1e-3) {
			t.Errorf("%s SRGBToLuv = %+v, want (%v,%v,%v)", r.name, luv, r.luvL, r.luvU, r.luvV)
		}
		lch := LuvToLCH(luv)
		if !approx(lch.C, r.lchuvC, 1e-3) || !hueClose(lch.H, r.lchuvH, lch.C, 1e-3, 1e-3) {
			t.Errorf("%s LuvToLCH = %+v, want C=%v H=%v", r.name, lch, r.lchuvC, r.lchuvH)
		}
	}
}

func TestParityOKLab(t *testing.T) {
	for _, r := range oracleRows {
		ok := SRGBToOKLab(r.r, r.g, r.b)
		if !approx(ok.L, r.okL, 1e-3) || !approx(ok.A, r.okA, 1e-3) || !approx(ok.B, r.okB, 1e-3) {
			t.Errorf("%s SRGBToOKLab = %+v, want (%v,%v,%v)", r.name, ok, r.okL, r.okA, r.okB)
		}
		lch := OKLabToOKLCH(ok)
		// OKLCH hue drifts up to ~0.03 deg vs the oracle: colour-science routes
		// OKLab through XYZ (Ottosson's M1) while this package uses Ottosson's
		// canonical linear-sRGB matrices direct (as CSS does); L/a/b/C still
		// agree to 1e-3.
		if !approx(lch.C, r.okC, 1e-3) || !hueClose(lch.H, r.okH, lch.C, 1e-4, 5e-2) {
			t.Errorf("%s OKLabToOKLCH = %+v, want C=%v H=%v", r.name, lch, r.okC, r.okH)
		}
	}
}

// Lab under D50, reached by Bradford-adapting the D65 XYZ, matches the oracle.
func TestParityLabD50Bradford(t *testing.T) {
	for _, r := range oracleRows {
		xyz := LinearRGBToXYZ(SRGBToLinear(r.r), SRGBToLinear(r.g), SRGBToLinear(r.b))
		lab := XYZToLabWP(AdaptD65ToD50(xyz), D50)
		if !approx(lab.L, r.lab50L, 2e-3) || !approx(lab.A, r.lab50A, 2e-3) || !approx(lab.B, r.lab50B, 2e-3) {
			t.Errorf("%s Lab(D50) = %+v, want (%v,%v,%v)", r.name, lab, r.lab50L, r.lab50A, r.lab50B)
		}
	}
}

// CIEDE2000 matches the published reference data of Sharma, Wu & Dalal (via the
// colour-science oracle) to within 1e-4 ΔE units.
func TestParityCIEDE2000(t *testing.T) {
	for _, r := range ciede2000Rows {
		got := DeltaE2000(Lab{r.l1, r.a1, r.b1}, Lab{r.l2, r.a2, r.b2})
		if math.Abs(got-r.want) > 1e-4 {
			t.Errorf("DeltaE2000(%v,%v) = %v, want %v", []float64{r.l1, r.a1, r.b1}, []float64{r.l2, r.a2, r.b2}, got, r.want)
		}
	}
}
