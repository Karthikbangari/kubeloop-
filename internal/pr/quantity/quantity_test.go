package quantity

import "testing"

func TestCPU(t *testing.T) {
	cases := map[int64]string{492: "492m", 1080: "1080m", 0: "0m"}
	for milli, want := range cases {
		if got := CPU(milli); got != want {
			t.Errorf("CPU(%d) = %q, want %q", milli, got, want)
		}
	}
}

func TestMem_RoundsUpAndPrefersGi(t *testing.T) {
	cases := map[int64]string{
		428 * mib:   "428Mi",
		428*mib + 1: "429Mi", // rounds up, never under-provisions
		1035 * mib:  "1035Mi",
		2 * gib:     "2Gi", // whole GiB
		1073741824:  "1Gi",
		0:           "0Mi",
	}
	for bytes, want := range cases {
		if got := Mem(bytes); got != want {
			t.Errorf("Mem(%d) = %q, want %q", bytes, got, want)
		}
	}
}
