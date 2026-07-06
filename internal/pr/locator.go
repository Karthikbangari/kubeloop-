package pr

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Ref identifies the workload to locate. Namespace is optional.
type Ref struct{ Kind, Name, Namespace string }

// File is a manifest file: its repo path and raw content.
type File struct {
	Path    string
	Content []byte
}

type identity struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
}

// FindSource returns the single file defining ref. It errors on zero matches or
// on ambiguity (more than one) — a wrong patch to a prod manifest is worse than
// none. Assumes one workload document per file (the common raw-YAML layout);
// multi-document files are a later refinement.
func FindSource(files []File, ref Ref) (File, error) {
	var matches []File
	for _, f := range files {
		var id identity
		if err := yaml.Unmarshal(f.Content, &id); err != nil {
			continue // not a single k8s object — skip
		}
		if id.Kind == ref.Kind && id.Metadata.Name == ref.Name &&
			(ref.Namespace == "" || id.Metadata.Namespace == ref.Namespace) {
			matches = append(matches, f)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return File{}, fmt.Errorf("no manifest found for %s %q", ref.Kind, ref.Name)
	default:
		return File{}, fmt.Errorf("%d manifests match %s %q — ambiguous, refusing to guess", len(matches), ref.Kind, ref.Name)
	}
}
