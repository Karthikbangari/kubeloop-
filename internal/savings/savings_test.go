package savings

import (
	"strings"
	"testing"
)

func TestRealization_PerRequestImmediate(t *testing.T) {
	if r := Realization(PerRequest); !strings.Contains(r, "immediately") {
		t.Errorf("per-request = %q, want immediacy", r)
	}
}

func TestRealization_NodeBasedConsolidate(t *testing.T) {
	if r := Realization(NodeBased); !strings.Contains(r, "consolidate") || strings.Contains(r, "immediately") {
		t.Errorf("node-based = %q, want consolidate + no immediacy claim", r)
	}
}

func TestRealization_UnknownDefaultsConservative(t *testing.T) {
	if r := Realization(Mode(99)); !strings.Contains(r, "consolidate") {
		t.Errorf("unknown mode = %q, want conservative node-based wording", r)
	}
}

func TestHeadline_IncludesDollarAndClause(t *testing.T) {
	if h := Headline(1240, PerRequest); !strings.Contains(h, "$1240.00") || !strings.Contains(h, "immediately") {
		t.Errorf("headline = %q, want $ + clause", h)
	}
}
