// Package labels qualifies workload names with their namespace only when the
// same name appears in more than one namespace, so normal output stays a clean
// single column but collisions are unambiguous. Shared by the ranked table and
// the excluded list so both disambiguate the same way.
package labels

// Item is a namespaced name to label.
type Item struct{ Namespace, Name string }

// Qualify returns one label per item, in order: the bare Name normally, or
// "Namespace/Name" when that Name collides across namespaces (and Namespace is
// set). An item with an empty Namespace always stays bare.
func Qualify(items []Item) []string {
	namespaces := map[string]map[string]struct{}{}
	for _, it := range items {
		if namespaces[it.Name] == nil {
			namespaces[it.Name] = map[string]struct{}{}
		}
		namespaces[it.Name][it.Namespace] = struct{}{}
	}
	out := make([]string, len(items))
	for i, it := range items {
		if len(namespaces[it.Name]) > 1 && it.Namespace != "" {
			out[i] = it.Namespace + "/" + it.Name
		} else {
			out[i] = it.Name
		}
	}
	return out
}
