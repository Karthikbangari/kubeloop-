package promusage

import (
	"testing"

	rs "github.com/kubeloop/kubeloop/internal/rightsizing"
)

const okResp = `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"pod":"api-abc"},"value":[1720200000,"0.41"]}]}}`

func TestScalar_Success(t *testing.T) {
	v, ok, err := Scalar([]byte(okResp))
	if err != nil || !ok {
		t.Fatalf("err=%v ok=%v, want value ok", err, ok)
	}
	if v != 0.41 {
		t.Errorf("value = %v, want 0.41", v)
	}
}

func TestScalar_EmptyResultIsMissingNotError(t *testing.T) {
	v, ok, err := Scalar([]byte(`{"status":"success","data":{"result":[]}}`))
	if err != nil || ok || v != 0 {
		t.Errorf("got v=%v ok=%v err=%v, want 0/false/nil (missing, not error)", v, ok, err)
	}
}

func TestScalar_NonSuccessStatusErrors(t *testing.T) {
	if _, _, err := Scalar([]byte(`{"status":"error","data":{"result":[]}}`)); err == nil {
		t.Error("want error on non-success status")
	}
}

func TestScalar_MalformedErrors(t *testing.T) {
	if _, _, err := Scalar([]byte("not json")); err == nil {
		t.Error("want error on malformed body")
	}
	// value present but not numeric.
	bad := `{"status":"success","data":{"result":[{"value":[1,"abc"]}]}}`
	if _, _, err := Scalar([]byte(bad)); err == nil {
		t.Error("want error on non-numeric sample value")
	}
}

func TestCoresToMilli_Rounds(t *testing.T) {
	cases := map[float64]int64{0.41: 410, 0.4104: 410, 0.4106: 411, 2: 2000, 0: 0}
	for cores, want := range cases {
		if got := CoresToMilli(cores); got != want {
			t.Errorf("CoresToMilli(%v) = %d, want %d", cores, got, want)
		}
	}
}

func TestAssembleUsage_ConvertsUnits(t *testing.T) {
	got := AssembleUsage(0.41, 0.48, 900*1024*1024)
	want := rs.Usage{P95CPU: 410, P99CPU: 480, MaxMem: 900 * 1024 * 1024}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
