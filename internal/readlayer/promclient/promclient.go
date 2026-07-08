// Package promclient queries a Prometheus HTTP API for a single scalar value.
// Query *construction* (the PromQL strings) is deliberately the caller's job —
// those need validation against a live Prometheus — so this package owns only
// the HTTP plumbing and response parsing, both provable offline with httptest
// and internal/readlayer/promusage.
package promclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/kubeloop/kubeloop/internal/readlayer/promusage"
)

// Client talks to a Prometheus HTTP API. Read-only: it only issues GET queries.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a client for baseURL (e.g. "http://localhost:9090"). A nil
// http.Client uses http.DefaultClient.
func New(baseURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: hc}
}

// Query runs a Prometheus instant query and returns the first sample's value.
// ok is false with a nil error when the query returned no data (a metrics gap),
// which callers treat as missing — the same contract as promusage.Scalar, so a
// gap flows through to safety's exclusion rather than a wrong number.
func (c *Client) Query(ctx context.Context, promQL string) (val float64, ok bool, err error) {
	endpoint := c.baseURL + "/api/v1/query?query=" + url.QueryEscape(promQL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// http wraps failures in *url.Error, whose Error() embeds the full
		// request URL. A real PromQL query percent-encodes into hundreds of
		// characters, so the useful part ("connection refused") drowns in a
		// wall of %28%29%7B. Report the base URL and the underlying cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			return 0, false, fmt.Errorf("prometheus request to %s: %w", c.baseURL, ue.Err)
		}
		return 0, false, fmt.Errorf("prometheus request to %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, false, fmt.Errorf("prometheus returned %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, false, err
	}
	return promusage.Scalar(body)
}
