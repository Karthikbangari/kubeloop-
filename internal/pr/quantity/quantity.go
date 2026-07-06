// Package quantity formats numeric requests back into Kubernetes quantity
// strings for patching manifests — the bridge from scan's numeric proposals to
// the PR patcher's string values. CPU is exact millicores; memory rounds UP to
// whole MiB so a proposed request is never below the computed value.
package quantity

import "fmt"

const (
	mib = 1024 * 1024
	gib = 1024 * mib
)

// CPU formats millicores as a k8s quantity, e.g. 492 -> "492m". Millicores form
// is always exact and unambiguous.
func CPU(millicores int64) string {
	return fmt.Sprintf("%dm", millicores)
}

// Mem formats bytes as a k8s quantity, rounding UP to whole MiB (never propose
// less memory than computed). Uses Gi when the result is a whole number of GiB,
// else Mi.
func Mem(bytes int64) string {
	mi := (bytes + mib - 1) / mib // ceil to MiB
	if mi > 0 && mi%1024 == 0 {
		return fmt.Sprintf("%dGi", mi/1024)
	}
	return fmt.Sprintf("%dMi", mi)
}
