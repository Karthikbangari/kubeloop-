// Package ghclient opens a pull request through the GitHub REST API.
//
// Plain net/http, no SDK — one POST does not justify a dependency tree, and the
// project's only dependency stays yaml.v3.
//
// This is the single place kubeloop reaches an external write API. It creates
// pull requests and nothing else: no merging, no pushing to a base branch, no
// repository administration. The token is never logged, never embedded in a URL,
// and is scrubbed from every error this package returns.
package ghclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// DefaultBaseURL is github.com's API. GitHub Enterprise sets its own.
const DefaultBaseURL = "https://api.github.com"

// TokenFromEnv reads the conventional token variables. Returns "" when unset;
// the caller decides whether that is fatal, so `--dry-run` needs no token.
func TokenFromEnv() string {
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

// Client talks to the GitHub REST API.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New returns a client for github.com.
func New(token string, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{BaseURL: DefaultBaseURL, Token: token, HTTP: hc}
}

// PullRequest is the pull request to open. Head and Base are branch names.
type PullRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Head  string `json:"head"`
	Base  string `json:"base"`
}

// Created is the subset of GitHub's response we surface.
type Created struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
}

// CreatePR opens a pull request and returns its number and URL.
//
// Refuses head == base up front: GitHub would reject it, but a clear local
// error beats a 422 the user has to decode.
func (c *Client) CreatePR(ctx context.Context, owner, repo string, pr PullRequest) (Created, error) {
	if c.Token == "" {
		return Created{}, fmt.Errorf("no GitHub token: set GITHUB_TOKEN (needs `repo` scope) to open a pull request")
	}
	if owner == "" || repo == "" {
		return Created{}, fmt.Errorf("owner and repo are required")
	}
	if pr.Head == pr.Base {
		return Created{}, fmt.Errorf("refusing to open a pull request from %q onto itself", pr.Head)
	}
	body, err := json.Marshal(pr)
	if err != nil {
		return Created{}, err
	}
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls", strings.TrimRight(base, "/"), owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Created{}, c.scrub(err)
	}
	// The token rides in a header, never in the URL — URLs land in proxy logs,
	// shell history, and error strings.
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return Created{}, c.scrub(fmt.Errorf("github request: %w", err))
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Created{}, c.scrub(err)
	}
	if resp.StatusCode != http.StatusCreated {
		return Created{}, c.scrub(apiError(resp.StatusCode, owner, repo, pr, respBody))
	}
	var out Created
	if err := json.Unmarshal(respBody, &out); err != nil {
		return Created{}, c.scrub(fmt.Errorf("parse github response: %w", err))
	}
	if out.HTMLURL == "" {
		return Created{}, fmt.Errorf("github reported success but returned no pull request URL")
	}
	return out, nil
}

// apiError turns GitHub's status codes into messages that say what to do.
func apiError(status int, owner, repo string, pr PullRequest, body []byte) error {
	var ghErr struct {
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(body, &ghErr)
	detail := ghErr.Message
	for _, e := range ghErr.Errors {
		if e.Message != "" {
			detail += ": " + e.Message
		}
	}
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("github rejected the token (401): check GITHUB_TOKEN is valid and unexpired")
	case http.StatusForbidden:
		return fmt.Errorf("github forbade the request (403): the token likely lacks `repo` scope, or you hit a rate limit")
	case http.StatusNotFound:
		// 404 (not 403) is what GitHub returns for a private repo the token
		// cannot see — saying "does not exist" alone would send the user to
		// debug the wrong thing.
		return fmt.Errorf("github returned 404 for %s/%s: the repository does not exist, or the token cannot see it", owner, repo)
	case http.StatusUnprocessableEntity:
		return fmt.Errorf("github rejected the pull request (422) from %q onto %q: %s "+
			"(a pull request for this branch may already be open, or the branch was never pushed)", pr.Head, pr.Base, detail)
	}
	if detail == "" {
		detail = strings.TrimSpace(string(body))
	}
	return fmt.Errorf("github returned %d: %s", status, detail)
}

// scrub removes the token from an error, in case a transport embedded the
// request (and its headers) in the message. Errors get logged and pasted into
// issue reports; a leaked PAT there is a live credential.
func (c *Client) scrub(err error) error {
	if err == nil || c.Token == "" {
		return err
	}
	msg := err.Error()
	if !strings.Contains(msg, c.Token) {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(msg, c.Token, "[REDACTED]"))
}
