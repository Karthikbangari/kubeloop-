package quantityparse

import "testing"

func TestCPU(t *testing.T) {
	ok := map[string]int64{"100m": 100, "2000m": 2000, "2": 2000, "1.5": 1500, "0.5": 500, "0": 0}
	for s, want := range ok {
		got, err := CPU(s)
		if err != nil || got != want {
			t.Errorf("CPU(%q) = %d, %v; want %d", s, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "10x", "m", "1.5.5", "-1", "-100m", "NaN", "+Inf"} {
		if _, err := CPU(bad); err == nil {
			t.Errorf("CPU(%q) should error", bad)
		}
	}
}

func TestMem(t *testing.T) {
	const Mi, Gi = 1 << 20, 1 << 30
	ok := map[string]int64{
		"512Mi":      512 * Mi,
		"1Gi":        Gi,
		"1G":         1e9,
		"1M":         1e6,
		"1Ki":        1024,
		"1000000000": 1000000000,
		"0":          0,
	}
	for s, want := range ok {
		got, err := Mem(s)
		if err != nil || got != want {
			t.Errorf("Mem(%q) = %d, %v; want %d", s, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "512MB", "Gi", "1.2.3Mi", "-1", "-1Gi", "NaN", "+Inf"} {
		if _, err := Mem(bad); err == nil {
			t.Errorf("Mem(%q) should error", bad)
		}
	}
}

// Round-trips with the shipped formatter's canonical forms (informal check that
// parse(format(x)) is stable for the values we emit).
func TestMem_ParsesCanonicalFormatterOutput(t *testing.T) {
	for s, want := range map[string]int64{"428Mi": 428 << 20, "2Gi": 2 << 30} {
		if got, err := Mem(s); err != nil || got != want {
			t.Errorf("Mem(%q) = %d, %v; want %d", s, got, err, want)
		}
	}
}
