package pr

import rs "github.com/Karthikbangari/kubeloop-/internal/rightsizing"

// Reductions returns the CPU and memory to patch: the proposed value when it
// reduces the current request, else 0 (don't change that resource).
func Reductions(current, proposed rs.Resources) (cpu, mem int64) {
	if proposed.CPU < current.CPU {
		cpu = proposed.CPU
	}
	if proposed.Mem < current.Mem {
		mem = proposed.Mem
	}
	return cpu, mem
}
