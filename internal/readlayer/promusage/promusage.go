// Package promusage parses Prometheus instant-query responses into the usage
// numbers the recommender needs, and converts units. This is the offline-
// provable half of the usage read-layer: response parsing is fully testable
// here. Building the PromQL query strings is a separate slice — they need
// validation against a live Prometheus, so they are deliberately NOT guessed
// here where they couldn't be checked.
package promusage

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	rs "github.com/kubeloop/kubeloop/internal/rightsizing"
)

// instantResponse is the subset of Prometheus GET /api/v1/query we read: a
// vector result whose first sample's value is the scalar answer.
type instantResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			// value is [ <unix ts number>, "<value as string>" ].
			Value [2]json.RawMessage `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// Scalar parses a Prometheus instant-query response and returns the first
// sample's numeric value. An empty result is (0, false, nil): a legitimate
// "no data", which callers treat as missing rather than an error — that's how
// a degraded metric (e.g. missing P99) becomes the safe fallback downstream.
func Scalar(body []byte) (val float64, ok bool, err error) {
	var r instantResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return 0, false, fmt.Errorf("parse prometheus response: %w", err)
	}
	if r.Status != "success" {
		return 0, false, fmt.Errorf("prometheus status %q, not success", r.Status)
	}
	if len(r.Data.Result) == 0 {
		return 0, false, nil
	}
	var s string
	if err := json.Unmarshal(r.Data.Result[0].Value[1], &s); err != nil {
		return 0, false, fmt.Errorf("parse sample value: %w", err)
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false, fmt.Errorf("sample value %q not numeric: %w", s, err)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false, fmt.Errorf("sample value %q not finite", s)
	}
	return v, true, nil
}

// CoresToMilli converts a CPU value in cores (as Prometheus reports rate) to
// millicores, rounding to nearest.
func CoresToMilli(cores float64) int64 {
	return int64(cores*1000 + 0.5)
}

// AssembleUsage builds rs.Usage from query results: CPU percentiles in cores,
// memory max in bytes. Missing values stay zero, which the recommender's floors
// already handle safely (a missing P99 falls back to P95).
func AssembleUsage(p95Cores, p99Cores, maxMemBytes float64) rs.Usage {
	return rs.Usage{
		P95CPU: CoresToMilli(p95Cores),
		P99CPU: CoresToMilli(p99Cores),
		MaxMem: int64(maxMemBytes + 0.5),
	}
}
