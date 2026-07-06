package pr

import (
	"fmt"

	rp "github.com/kubeloop/kubeloop/internal/reporting"
)

// FindRow returns the single row matching name (and namespace, if given). It
// errors on no match, or on ambiguity (same name in multiple namespaces with no
// namespace to disambiguate).
func FindRow(rows []rp.Row, namespace, name string) (rp.Row, error) {
	var matches []rp.Row
	for _, r := range rows {
		if r.Name == name && (namespace == "" || r.Namespace == namespace) {
			matches = append(matches, r)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return rp.Row{}, fmt.Errorf("no rankable workload %q found in scan output", name)
	default:
		return rp.Row{}, fmt.Errorf("workload %q matches %d namespaces — pass --namespace to disambiguate", name, len(matches))
	}
}
