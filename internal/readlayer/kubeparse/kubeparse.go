// Package kubeparse turns a serialized Kubernetes workload object (as returned
// by the API or `kubectl get -o json`) into the inventory the scanner needs,
// parsing request strings via quantityparse. Parsing captured JSON is offline-
// provable; the live API LIST call is the remaining cluster-gated piece.
package kubeparse

import (
	"encoding/json"
	"fmt"

	"github.com/Karthikbangari/kubeloop-/internal/inventory"
	"github.com/Karthikbangari/kubeloop-/internal/readlayer/quantityparse"
)

// Workload is one parsed object: identity plus its containers, ready for
// inventory.PodRequest / inventory.DetectRuntime.
type Workload struct {
	Kind           string
	Namespace      string
	Name           string
	Replicas       int
	Containers     []inventory.Container
	InitContainers []inventory.Container
}

// object is the subset of a k8s workload manifest we read.
type object struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Replicas *int `json:"replicas"`
		Template struct {
			Spec struct {
				Containers     []rawContainer `json:"containers"`
				InitContainers []rawContainer `json:"initContainers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
}

type rawContainer struct {
	Name      string   `json:"name"`
	Image     string   `json:"image"`
	Command   []string `json:"command"`
	Resources struct {
		Requests struct {
			CPU string `json:"cpu"`
			Mem string `json:"memory"`
		} `json:"requests"`
	} `json:"resources"`
}

// Parse reads one workload object. Replicas defaults to 1 when unset (the k8s
// default). A malformed request quantity is an error, not a silent zero — a
// wrong current request would corrupt the waste math.
func Parse(doc []byte) (Workload, error) {
	var o object
	if err := json.Unmarshal(doc, &o); err != nil {
		return Workload{}, fmt.Errorf("parse workload: %w", err)
	}
	replicas := 1
	if o.Spec.Replicas != nil {
		replicas = *o.Spec.Replicas
		if replicas < 0 {
			return Workload{}, fmt.Errorf("negative replicas %d", replicas)
		}
	}
	reg, err := toContainers(o.Spec.Template.Spec.Containers)
	if err != nil {
		return Workload{}, err
	}
	init, err := toContainers(o.Spec.Template.Spec.InitContainers)
	if err != nil {
		return Workload{}, err
	}
	return Workload{
		Kind:           o.Kind,
		Namespace:      o.Metadata.Namespace,
		Name:           o.Metadata.Name,
		Replicas:       replicas,
		Containers:     reg,
		InitContainers: init,
	}, nil
}

func toContainers(raw []rawContainer) ([]inventory.Container, error) {
	out := make([]inventory.Container, len(raw))
	for i, c := range raw {
		cpu, err := parseOptional(c.Resources.Requests.CPU, quantityparse.CPU)
		if err != nil {
			return nil, fmt.Errorf("container %q cpu: %w", c.Name, err)
		}
		mem, err := parseOptional(c.Resources.Requests.Mem, quantityparse.Mem)
		if err != nil {
			return nil, fmt.Errorf("container %q memory: %w", c.Name, err)
		}
		out[i] = inventory.Container{Image: c.Image, Command: c.Command, CPU: cpu, Mem: mem}
	}
	return out, nil
}

// parseOptional treats an absent request ("") as 0 (unset), but a present-but-
// malformed one as an error.
func parseOptional(s string, parse func(string) (int64, error)) (int64, error) {
	if s == "" {
		return 0, nil
	}
	return parse(s)
}
