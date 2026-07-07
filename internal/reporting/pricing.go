package reporting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	rs "github.com/kubeloop/kubeloop/internal/rightsizing"
)

// Per-unit prices are derived from readable on-demand rates so the numbers are
// auditable. Directional (list prices) — the ranking is the point, not the cent.
const (
	gib          = 1024 * 1024 * 1024
	milliPerVCPU = 1000
)

// cloudRate is a human-readable on-demand rate: dollars per vCPU-hour and per
// GB-hour. Splitting a node's price into compute+memory is inherently fuzzy
// (that's why we call the dollars directional); these track typical general-
// purpose instance rates closely enough to rank waste.
type cloudRate struct {
	perVCPUHour, perGBHour float64
}

var rates = map[string]cloudRate{
	"aws":   {perVCPUHour: 0.031, perGBHour: 0.0042},
	"gcp":   {perVCPUHour: 0.028, perGBHour: 0.0038},
	"azure": {perVCPUHour: 0.030, perGBHour: 0.0041},
}

// DefaultPrice returns list prices for a cloud, falling back to AWS for an
// unknown cloud so a scan still ranks rather than erroring.
func DefaultPrice(cloud string) rs.Price {
	r, ok := rates[cloud]
	if !ok {
		r = rates["aws"]
	}
	return rs.Price{
		PerMilliCPUHour: r.perVCPUHour / milliPerVCPU,
		PerByteMemHour:  r.perGBHour / gib,
	}
}

// PriceFile is the override file shape: readable per-vCPU-hour / per-GB-hour
// rates keyed by cloud, matching how cloud pricing pages quote them.
// ponytail: JSON, not YAML — the module stays zero-dependency and encoding/json
// is already used elsewhere; the CLI names this file pricing.json.
type PriceFile struct {
	Clouds map[string]PriceRate `json:"clouds"`
}

// PriceRate is one cloud's override. Omitted (zero) fields keep the default.
type PriceRate struct {
	PerVCPUHour float64 `json:"perVCPUHour"`
	PerGBHour   float64 `json:"perGBHour"`
}

// LoadPrice returns list prices for a cloud. With no file it's the built-in
// default; with a file, any present override field replaces the default for
// that cloud (per-field, so a file can tweak only CPU or only memory; an
// unlisted cloud keeps its default).
func LoadPrice(cloud, file string) (rs.Price, error) {
	base := DefaultPrice(cloud)
	if file == "" {
		return base, nil
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return rs.Price{}, err
	}
	// Reject unknown fields so a mistyped key (e.g. "perVcpuHour") errors instead
	// of silently ignoring the override and using default prices.
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var f PriceFile
	if err := dec.Decode(&f); err != nil {
		return rs.Price{}, fmt.Errorf("parse %s: %w", file, err)
	}
	r, ok := f.Clouds[cloud]
	if !ok {
		return base, nil
	}
	return rs.Price{
		PerMilliCPUHour: orDefault(r.PerVCPUHour/milliPerVCPU, base.PerMilliCPUHour),
		PerByteMemHour:  orDefault(r.PerGBHour/gib, base.PerByteMemHour),
	}, nil
}

func orDefault(override, base float64) float64 {
	if override > 0 {
		return override
	}
	return base
}
