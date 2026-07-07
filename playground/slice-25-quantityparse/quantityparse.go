// Package quantityparse parses Kubernetes resource quantity strings into the
// numeric units the scanner uses — CPU millicores, memory bytes. It is the
// inverse of internal/pr/quantity and the read-layer's bridge from real kube
// objects (which express requests as quantity strings like "2000m"/"512Mi") to
// numbers. Correct-or-error: a wrong parse would corrupt every downstream
// recommendation, so unsupported forms are refused, not guessed.
//
// Reverses the earlier "defer all quantity parsing to apimachinery" note: the
// project stayed lightweight (yaml.v3 only), the offline read-layer needs this,
// and it's symmetric with the already-shipped quantity formatter.
package quantityparse

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// CPU parses a CPU quantity to millicores: "100m"→100, "2"→2000, "1.5"→1500,
// "0.5"→500. Cores are rounded to the nearest milli.
func CPU(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty CPU quantity")
	}
	if v, ok := strings.CutSuffix(s, "m"); ok {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("bad millicpu %q: %w", s, err)
		}
		if n < 0 {
			return 0, fmt.Errorf("negative CPU quantity %q", s)
		}
		return n, nil
	}
	cores, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("bad cpu %q: %w", s, err)
	}
	if cores < 0 || math.IsNaN(cores) || math.IsInf(cores, 0) {
		return 0, fmt.Errorf("invalid CPU quantity %q", s)
	}
	return int64(cores*1000 + 0.5), nil
}

// memSuffix maps a memory suffix to its multiplier. Binary (Ki/Mi/...) is
// listed before decimal (k/M/...) so the two-letter suffixes match first.
var memSuffix = []struct {
	s   string
	mul float64
}{
	{"Ki", 1 << 10}, {"Mi", 1 << 20}, {"Gi", 1 << 30}, {"Ti", 1 << 40}, {"Pi", 1 << 50},
	{"k", 1e3}, {"M", 1e6}, {"G", 1e9}, {"T", 1e12}, {"P", 1e15},
}

// Mem parses a memory quantity to bytes: "512Mi", "1Gi", "1G" (1e9),
// "1000000000" (plain bytes). Rounds to the nearest byte.
func Mem(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty memory quantity")
	}
	for _, suf := range memSuffix {
		if v, ok := strings.CutSuffix(s, suf.s); ok {
			n, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return 0, fmt.Errorf("bad memory %q: %w", s, err)
			}
			if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
				return 0, fmt.Errorf("invalid memory quantity %q", s)
			}
			return int64(n*suf.mul + 0.5), nil
		}
	}
	n, err := strconv.ParseFloat(s, 64) // no suffix → bytes
	if err != nil {
		return 0, fmt.Errorf("bad memory %q: %w", s, err)
	}
	if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("invalid memory quantity %q", s)
	}
	return int64(n + 0.5), nil
}
