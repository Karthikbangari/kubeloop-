package safety

import (
	"strings"
	"testing"

	rs "github.com/Karthikbangari/kubeloop-/internal/rightsizing"
)

// stable is a healthy usage sample (real CPU + memory signal) so Assess tests
// exercise the batch/history rules without tripping the missing-signal gate.
var stable = rs.Usage{P95CPU: 410, P99CPU: 480, MaxMem: 512 * 1024 * 1024}

func TestAssess_ExcludesNoCPUSignal(t *testing.T) {
	e := Assess(Meta{Kind: "Deployment", HistoryDays: 30}, rs.Usage{MaxMem: 1 << 30})
	if !e.Excluded || !strings.Contains(e.Reason, "CPU") {
		t.Errorf("no CPU signal = %+v, want excluded citing CPU", e)
	}
}

func TestAssess_ExcludesNoMemSignal(t *testing.T) {
	e := Assess(Meta{Kind: "Deployment", HistoryDays: 30}, rs.Usage{P95CPU: 410, P99CPU: 480, MaxMem: 0})
	if !e.Excluded || !strings.Contains(e.Reason, "memory") {
		t.Errorf("no memory signal = %+v, want excluded citing memory", e)
	}
}

func TestAssess_ExcludesBatch(t *testing.T) {
	e := Assess(Meta{Kind: "CronJob", HistoryDays: 30}, stable)
	if !e.Excluded || !strings.Contains(e.Reason, "batch") {
		t.Errorf("CronJob = %+v, want excluded with batch reason", e)
	}
}

func TestAssess_ExcludesBatch_CaseInsensitive(t *testing.T) {
	if e := Assess(Meta{Kind: "cronjob", HistoryDays: 30}, stable); !e.Excluded {
		t.Errorf("lowercase 'cronjob' = %+v, want excluded (casing must not skip the rule)", e)
	}
}

func TestAssess_ExcludesShortHistory(t *testing.T) {
	e := Assess(Meta{Kind: "Deployment", HistoryDays: 3}, stable)
	if !e.Excluded || !strings.Contains(e.Reason, "3d") {
		t.Errorf("3d history = %+v, want excluded citing 3d", e)
	}
}

func TestAssess_KeepsNormal(t *testing.T) {
	if e := Assess(Meta{Kind: "Deployment", HistoryDays: 30}, stable); e.Excluded {
		t.Errorf("normal Deployment = %+v, want kept", e)
	}
}

func TestScore_HighForStableLongHistory(t *testing.T) {
	c := Score(Meta{Kind: "Deployment", HistoryDays: 30}, stable)
	if c.Level != "high" || c.Caution != "" {
		t.Errorf("stable+long = %+v, want high/no-caution", c)
	}
}

func TestScore_MedForSpiky(t *testing.T) {
	spiky := rs.Usage{P95CPU: 400, P99CPU: 800} // 2× → spiky
	if c := Score(Meta{Kind: "Deployment", HistoryDays: 30}, spiky); c.Level != "med" {
		t.Errorf("spiky = %+v, want med", c)
	}
}

func TestScore_ShortHistoryDowngrades(t *testing.T) {
	if c := Score(Meta{Kind: "Deployment", HistoryDays: 10}, stable); c.Level != "med" {
		t.Errorf("10d history = %+v, want med (downgraded)", c)
	}
}

func TestScore_JVMNeverHighAndCautions(t *testing.T) {
	// Uppercase runtime must still trigger the JVM caution.
	c := Score(Meta{Kind: "Deployment", HistoryDays: 30, Runtime: "JVM"}, stable)
	if c.Level != "med" || !strings.Contains(c.Caution, "JVM") {
		t.Errorf("JVM stable = %+v, want med with JVM caution", c)
	}
}
