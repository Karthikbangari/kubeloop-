package pr

import (
	"testing"

	rs "github.com/kubeloop/kubeloop/internal/rightsizing"
)

const Mi = 1024 * 1024

func TestReductions_BothReduce(t *testing.T) {
	cpu, mem := Reductions(rs.Resources{CPU: 2000, Mem: 512 * Mi}, rs.Resources{CPU: 576, Mem: 428 * Mi})
	if cpu != 576 || mem != 428*Mi {
		t.Errorf("got cpu=%d mem=%d, want 576 / 428Mi", cpu, mem)
	}
}

func TestReductions_MemoryIncreaseIsSkipped(t *testing.T) {
	// The bug this fixes: current mem 0 (unset), floor proposes 128Mi. That's an
	// increase, so a savings PR must leave memory alone and only cut CPU.
	cpu, mem := Reductions(rs.Resources{CPU: 2000, Mem: 0}, rs.Resources{CPU: 576, Mem: 128 * Mi})
	if cpu != 576 {
		t.Errorf("cpu = %d, want 576 (reduces)", cpu)
	}
	if mem != 0 {
		t.Errorf("mem = %d, want 0 (increase must be skipped)", mem)
	}
}

func TestReductions_NoChangeWhenNotLower(t *testing.T) {
	// Equal or higher on both -> nothing to patch.
	cpu, mem := Reductions(rs.Resources{CPU: 500, Mem: 100 * Mi}, rs.Resources{CPU: 500, Mem: 200 * Mi})
	if cpu != 0 || mem != 0 {
		t.Errorf("got cpu=%d mem=%d, want 0/0 (no reduction)", cpu, mem)
	}
}
