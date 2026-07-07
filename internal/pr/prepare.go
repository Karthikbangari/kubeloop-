package pr

import "fmt"

// Request is one workload's proposed change plus the repo files to search.
type Request struct {
	Files                    []File
	Ref                      Ref
	Container                string
	CurrentCPU, CurrentMem   string
	ProposedCPU, ProposedMem string
	MonthlyUSD               float64
	Confidence, Realization  string
	Caution                  string
}

// Prepared is everything needed to open the PR, all computed offline.
type Prepared struct {
	Path    string // source file to change
	Content []byte // patched file content
	Title   string
	Body    string
}

// Prepare locates the file, patches it, and composes the PR text. Any step's
// error (not found, ambiguous, identity mismatch, missing requests) short-
// circuits — a partial PR is never returned.
func Prepare(r Request) (Prepared, error) {
	src, err := FindSource(r.Files, r.Ref)
	if err != nil {
		return Prepared{}, err
	}
	// A pod-level proposal can't be split across containers yet — refuse rather
	// than emit a PR whose before/after doesn't match the patched result.
	if err := RequireSingleContainer(src.Content); err != nil {
		return Prepared{}, err
	}
	// Decide what actually changes at the quantity-string level (what the patch
	// writes), not the raw numeric level. A sub-Mi reduction can round to the
	// same string — refuse rather than emit a no-op PR that claims a saving.
	cpu := quantityIfChanged(r.CurrentCPU, r.ProposedCPU)
	mem := quantityIfChanged(r.CurrentMem, r.ProposedMem)
	if cpu == "" && mem == "" {
		return Prepared{}, fmt.Errorf("no effective request change for %q — the proposal rounds to the current request", r.Ref.Name)
	}
	patched, err := Patch(src.Content, Target{
		Kind: r.Ref.Kind, Name: r.Ref.Name, Namespace: r.Ref.Namespace, Container: r.Container,
	}, cpu, mem)
	if err != nil {
		return Prepared{}, err
	}
	change := Change{
		Namespace: r.Ref.Namespace, Name: r.Ref.Name, Container: r.Container,
		CurrentCPU: r.CurrentCPU, ProposedCPU: r.ProposedCPU,
		CurrentMem: r.CurrentMem, ProposedMem: r.ProposedMem,
		MonthlyUSD: r.MonthlyUSD, Confidence: r.Confidence, Realization: r.Realization,
		Caution: r.Caution,
	}
	return Prepared{
		Path:    src.Path,
		Content: patched,
		Title:   Title(change),
		Body:    Body(change),
	}, nil
}

func quantityIfChanged(current, proposed string) string {
	if changed(current, proposed) {
		return proposed
	}
	return ""
}
