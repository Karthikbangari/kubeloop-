package reporting

import (
	"os"
	"path/filepath"
	"testing"
)

func priceFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "pricing.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadPrice_NoFileIsDefault(t *testing.T) {
	got, err := LoadPrice("aws", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != DefaultPrice("aws") {
		t.Errorf("no file = %v, want default aws price", got)
	}
}

func TestLoadPrice_OverridesOnlyGivenField(t *testing.T) {
	// Override CPU only; memory must keep the aws default.
	got, err := LoadPrice("aws", priceFile(t, `{"clouds":{"aws":{"perVCPUHour":0.05}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.PerMilliCPUHour != 0.05/milliPerVCPU {
		t.Errorf("cpu = %v, want overridden 0.05/1000", got.PerMilliCPUHour)
	}
	if got.PerByteMemHour != DefaultPrice("aws").PerByteMemHour {
		t.Errorf("mem = %v, want unchanged aws default", got.PerByteMemHour)
	}
}

func TestLoadPrice_UnlistedCloudKeepsDefault(t *testing.T) {
	got, err := LoadPrice("gcp", priceFile(t, `{"clouds":{"aws":{"perVCPUHour":0.05}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got != DefaultPrice("gcp") {
		t.Errorf("gcp = %v, want gcp default (not in file)", got)
	}
}

func TestLoadPrice_BadFileErrors(t *testing.T) {
	if _, err := LoadPrice("aws", priceFile(t, "not json")); err == nil {
		t.Error("want parse error on bad file")
	}
	if _, err := LoadPrice("aws", "/no/such/file.json"); err == nil {
		t.Error("want error on missing file")
	}
}

func TestPlural(t *testing.T) {
	cases := map[int]string{0: "0 workloads", 1: "1 workload", 2: "2 workloads"}
	for n, want := range cases {
		if got := Plural(n, "workload"); got != want {
			t.Errorf("Plural(%d) = %q, want %q", n, got, want)
		}
	}
}
