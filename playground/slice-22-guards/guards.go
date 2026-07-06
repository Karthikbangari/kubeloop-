// Package guards refuses PRs the pod-level scan model can't produce honestly.
// The scan proposal is for the whole pod; applying it to one container of a
// multi-container pod (e.g. app + sidecar) leaves the siblings untouched, so
// the patched pod request no longer matches the PR's stated before/after.
// Until per-container proposals exist, refuse rather than emit a misleading PR.
package guards

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type podManifest struct {
	Spec struct {
		Template struct {
			Spec struct {
				Containers []struct {
					Name string `yaml:"name"`
				} `yaml:"containers"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

// ContainerCount returns how many containers the workload manifest declares.
func ContainerCount(doc []byte) (int, error) {
	var m podManifest
	if err := yaml.Unmarshal(doc, &m); err != nil {
		return 0, fmt.Errorf("parse manifest: %w", err)
	}
	return len(m.Spec.Template.Spec.Containers), nil
}

// RequireSingleContainer errors when the pod has more than one container, since
// a pod-level proposal can't be split across containers yet.
func RequireSingleContainer(doc []byte) error {
	n, err := ContainerCount(doc)
	if err != nil {
		return err
	}
	if n > 1 {
		return fmt.Errorf("workload has %d containers; per-container rightsizing isn't supported yet (proposals are pod-level) — refusing to open a PR that would overstate the change", n)
	}
	return nil
}
