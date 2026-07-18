// Package kustomizesource maps a rendered Kubernetes workload back to the
// source file that defines it in a Kustomize tree — the piece the raw-YAML
// locator (internal/pr.FindSource) is missing, so that `kubeloop pr` can patch
// the *source of truth* instead of a rendered file a GitOps controller would
// immediately regenerate.
//
// It works by reading the kustomization directly rather than shelling out to
// `kustomize build`: for the common overlay (a `resources:` list plus optional
// `namePrefix`/`nameSuffix`/`namespace`), the rendered name is just the source
// name with the prefix/suffix added, so stripping them and matching kind+name
// in the referenced files finds the source with no render step and no new
// dependency. Cases this does not yet cover (patches that rename, generators,
// remote/nested bases) are reported as clear errors, never guessed — the same
// "refuse rather than patch the wrong file" posture as FindSource.
package kustomizesource

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Ref identifies the rendered workload to trace back to source.
type Ref struct{ Kind, Name, Namespace string }

// Source is the located source file: its path (for the PR), raw content, and
// the source-side workload name. Name is the rendered name with the overlay's
// namePrefix/nameSuffix stripped — i.e. the name as it appears *in the file* —
// so a caller patching the source targets the right object, not the rendered
// name the cluster reported.
type Source struct {
	Path    string
	Content []byte
	Name    string
}

// kustomization is the subset of kustomization.yaml we read.
type kustomization struct {
	Resources  []string `yaml:"resources"`
	Bases      []string `yaml:"bases"` // deprecated alias for resources
	NamePrefix string   `yaml:"namePrefix"`
	NameSuffix string   `yaml:"nameSuffix"`
	Namespace  string   `yaml:"namespace"`
	// Signals we cannot honour yet; their presence means "don't trust a simple map".
	Patches            []yaml.Node `yaml:"patches"`
	PatchesStrategic   []yaml.Node `yaml:"patchesStrategicMerge"`
	PatchesJSON6902    []yaml.Node `yaml:"patchesJson6902"`
	Components         []string    `yaml:"components"`
	Replacements       []yaml.Node `yaml:"replacements"`
	Transformers       []string    `yaml:"transformers"`
	Vars               []yaml.Node `yaml:"vars"`
	ConfigMapGenerator []yaml.Node `yaml:"configMapGenerator"`
	SecretGenerator    []yaml.Node `yaml:"secretGenerator"`
}

type identity struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
}

var kustomizationNames = []string{"kustomization.yaml", "kustomization.yml", "Kustomization"}

// IsKustomizeDir reports whether dir contains a kustomization file.
func IsKustomizeDir(dir string) bool {
	for _, n := range kustomizationNames {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			return true
		}
	}
	return false
}

// FindSource locates the file in the Kustomize tree rooted at dir that defines
// the rendered workload ref. It descends one level of local `resources:`
// directories (the common base/overlay shape).
func FindSource(dir string, ref Ref) (Source, error) {
	k, kustPath, err := load(dir)
	if err != nil {
		return Source{}, err
	}
	// A rename patch or a generator can change a resource's name between source
	// and rendered output, so name-stripping would map to the wrong file (or
	// miss). Refuse rather than risk patching the wrong manifest.
	if reason := k.unsupported(); reason != "" {
		return Source{}, fmt.Errorf("kustomization %s uses %s; kubeloop can't yet map that back to source — patch the file by hand or use --manifest", kustPath, reason)
	}

	base := ref
	var ok bool
	base.Name, ok = stripAffixes(ref.Name, k.NamePrefix, k.NameSuffix)
	if !ok {
		return Source{}, fmt.Errorf("rendered name %q does not match kustomization namePrefix/nameSuffix in %s", ref.Name, kustPath)
	}

	var matches []Source
	for _, res := range append(k.Resources, k.Bases...) {
		p := filepath.Join(dir, res)
		info, err := os.Stat(p)
		if err != nil {
			continue // remote/URL resource, or missing — not locally mappable
		}
		if info.IsDir() {
			// A nested kustomization: recurse, carrying the still-stripped ref.
			if IsKustomizeDir(p) {
				if s, err := FindSource(p, base); err == nil {
					matches = append(matches, s)
				}
			}
			continue
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return Source{}, err
		}
		if fileDefines(content, base) {
			matches = append(matches, Source{Path: p, Content: content, Name: base.Name})
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return Source{}, fmt.Errorf("no source manifest for %s %q found under %s (checked %d resource(s))", ref.Kind, ref.Name, dir, len(k.Resources)+len(k.Bases))
	default:
		return Source{}, fmt.Errorf("%d source files match %s %q — ambiguous, refusing to guess", len(matches), ref.Kind, ref.Name)
	}
}

// load reads and parses the kustomization file in dir.
func load(dir string) (kustomization, string, error) {
	for _, n := range kustomizationNames {
		p := filepath.Join(dir, n)
		content, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var k kustomization
		if err := yaml.Unmarshal(content, &k); err != nil {
			return kustomization{}, p, fmt.Errorf("parse %s: %w", p, err)
		}
		return k, p, nil
	}
	return kustomization{}, "", fmt.Errorf("no kustomization.yaml in %s", dir)
}

// unsupported names the first transform present that would break a
// name-based source map, or "" if the kustomization is simple enough to trust.
func (k kustomization) unsupported() string {
	switch {
	case len(k.Patches) > 0 || len(k.PatchesStrategic) > 0 || len(k.PatchesJSON6902) > 0:
		return "patches (which can rename resources)"
	case len(k.Components) > 0:
		return "components"
	case len(k.Replacements) > 0 || len(k.Transformers) > 0 || len(k.Vars) > 0:
		return "transformers/replacements (which can rename resources)"
	case len(k.ConfigMapGenerator) > 0 || len(k.SecretGenerator) > 0:
		return "generators"
	}
	return ""
}

func stripAffixes(name, prefix, suffix string) (string, bool) {
	if prefix != "" && !strings.HasPrefix(name, prefix) {
		return "", false
	}
	name = strings.TrimPrefix(name, prefix)
	if suffix != "" && !strings.HasSuffix(name, suffix) {
		return "", false
	}
	return strings.TrimSuffix(name, suffix), true
}

// fileDefines reports whether any YAML document in content is the workload ref.
// Namespace is matched only when both sides declare one (kustomize often sets
// the namespace centrally, so a source file may legitimately omit it).
func fileDefines(content []byte, ref Ref) bool {
	dec := yaml.NewDecoder(strings.NewReader(string(content)))
	for {
		var id identity
		if err := dec.Decode(&id); err != nil {
			break // EOF or a non-object document
		}
		if id.Kind != ref.Kind || id.Metadata.Name != ref.Name {
			continue
		}
		if ref.Namespace != "" && id.Metadata.Namespace != "" && id.Metadata.Namespace != ref.Namespace {
			continue
		}
		return true
	}
	return false
}
