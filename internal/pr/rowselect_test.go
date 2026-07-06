package pr

import (
	"strings"
	"testing"

	rp "github.com/kubeloop/kubeloop/internal/reporting"
)

func rows() []rp.Row {
	return []rp.Row{
		{Workload: rp.Workload{Namespace: "team-a", Name: "api"}},
		{Workload: rp.Workload{Namespace: "team-b", Name: "api"}},
		{Workload: rp.Workload{Namespace: "shop", Name: "checkout"}},
	}
}

func TestFindRow_UniqueName(t *testing.T) {
	r, err := FindRow(rows(), "", "checkout")
	if err != nil || r.Namespace != "shop" {
		t.Errorf("got %+v err=%v, want shop/checkout", r, err)
	}
}

func TestFindRow_AmbiguousWithoutNamespaceErrors(t *testing.T) {
	_, err := FindRow(rows(), "", "api")
	if err == nil || !strings.Contains(err.Error(), "namespace") {
		t.Errorf("want ambiguity error asking for --namespace, got %v", err)
	}
}

func TestFindRow_NamespaceDisambiguates(t *testing.T) {
	r, err := FindRow(rows(), "team-b", "api")
	if err != nil || r.Namespace != "team-b" {
		t.Errorf("got %+v err=%v, want team-b/api", r, err)
	}
}

func TestFindRow_NoMatchErrors(t *testing.T) {
	if _, err := FindRow(rows(), "", "missing"); err == nil {
		t.Error("want error when no row matches")
	}
}
