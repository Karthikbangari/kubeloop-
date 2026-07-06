package pr

import (
	"strings"
	"testing"
)

func sample() Change {
	return Change{
		Namespace: "shop", Name: "checkout-api", Container: "app",
		CurrentCPU: "2000m", ProposedCPU: "492m",
		CurrentMem: "512Mi", ProposedMem: "428Mi",
		MonthlyUSD:  131.2,
		Confidence:  "high",
		Realization: "realized when nodes consolidate (Cluster Autoscaler / Karpenter)",
	}
}

func TestTitle_LeadsWithDollars(t *testing.T) {
	got := Title(sample())
	if !strings.Contains(got, "checkout-api") || !strings.Contains(got, "$131") {
		t.Errorf("title = %q, want name + rounded $131", got)
	}
}

func TestBody_HasEvidenceRollbackAndReadOnly(t *testing.T) {
	b := Body(sample())
	for _, want := range []string{
		"$131.20/month",
		"realized when nodes consolidate",
		"| CPU | 2000m | 492m |",
		"| Memory | 512Mi | 428Mi |",
		"Confidence: **high**",
		"P99×1.2",
		"revert this commit",
		"CPU `2000m` / memory `512Mi`",
		"read-only",
	} {
		if !strings.Contains(b, want) {
			t.Errorf("body missing %q:\n%s", want, b)
		}
	}
}

func TestBody_OmitsUnchangedResource(t *testing.T) {
	c := sample()
	c.CurrentMem, c.ProposedMem = "512Mi", "512Mi" // unchanged
	b := Body(c)
	if strings.Contains(b, "| Memory") {
		t.Errorf("memory row should be omitted when unchanged:\n%s", b)
	}
	if !strings.Contains(b, "| CPU | 2000m | 492m |") {
		t.Errorf("cpu row missing:\n%s", b)
	}
	if strings.Contains(b, "memory `") {
		t.Errorf("rollback should not mention memory when unchanged:\n%s", b)
	}
}
