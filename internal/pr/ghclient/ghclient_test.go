package ghclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const token = "ghp_supersecrettokenvalue"

func samplePR() PullRequest {
	return PullRequest{Title: "rightsize checkout-api", Body: "saves $32/mo", Head: "kubeloop/rightsize", Base: "main"}
}

func TestCreatePR_PostsAndReturnsURL(t *testing.T) {
	var gotPath, gotAuth, gotAccept, gotVersion string
	var gotBody PullRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		gotAccept, gotVersion = r.Header.Get("Accept"), r.Header.Get("X-GitHub-Api-Version")
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"number":7,"html_url":"https://github.com/o/r/pull/7"}`)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Token: token, HTTP: srv.Client()}
	got, err := c.CreatePR(context.Background(), "Karthikbangari", "kubeloop-", samplePR())
	if err != nil {
		t.Fatal(err)
	}
	if got.Number != 7 || got.HTMLURL != "https://github.com/o/r/pull/7" {
		t.Errorf("created = %+v", got)
	}
	// The repo name's trailing dash must survive into the path.
	if gotPath != "/repos/Karthikbangari/kubeloop-/pulls" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer "+token {
		t.Errorf("auth header = %q", gotAuth)
	}
	if gotAccept != "application/vnd.github+json" || gotVersion != "2022-11-28" {
		t.Errorf("accept=%q version=%q", gotAccept, gotVersion)
	}
	if gotBody != samplePR() {
		t.Errorf("body = %+v, want %+v", gotBody, samplePR())
	}
}

// The token must never appear in the request URL — URLs reach proxy logs and
// shell history.
func TestCreatePR_TokenNotInURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.String(), token) {
			t.Errorf("token leaked into URL: %s", r.URL)
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"number":1,"html_url":"https://x/pull/1"}`)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Token: token, HTTP: srv.Client()}
	if _, err := c.CreatePR(context.Background(), "o", "r", samplePR()); err != nil {
		t.Fatal(err)
	}
}

// A transport that echoes the request (headers included) into its error must
// not leak the token through our error. Errors get pasted into bug reports.
type leakyTransport struct{}

func (leakyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, errors.New("dial failed for request with header Authorization: " + r.Header.Get("Authorization"))
}

func TestCreatePR_ScrubsTokenFromErrors(t *testing.T) {
	c := &Client{BaseURL: "https://api.example.invalid", Token: token, HTTP: &http.Client{Transport: leakyTransport{}}}
	_, err := c.CreatePR(context.Background(), "o", "r", samplePR())
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("token leaked into error: %v", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Errorf("want the token redacted, got %v", err)
	}
}

func TestCreatePR_RequiresToken(t *testing.T) {
	_, err := (&Client{BaseURL: "http://x", Token: ""}).CreatePR(context.Background(), "o", "r", samplePR())
	if err == nil || !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("want an actionable no-token error, got %v", err)
	}
}

func TestCreatePR_RefusesHeadEqualsBase(t *testing.T) {
	pr := samplePR()
	pr.Head = pr.Base
	_, err := (&Client{BaseURL: "http://x", Token: token}).CreatePR(context.Background(), "o", "r", pr)
	if err == nil || !strings.Contains(err.Error(), "onto itself") {
		t.Fatalf("want refusal, got %v", err)
	}
}

func TestCreatePR_StatusErrorsAreActionable(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusUnauthorized, `{"message":"Bad credentials"}`, "check GITHUB_TOKEN"},
		{http.StatusForbidden, `{"message":"Resource not accessible"}`, "`repo` scope"},
		{http.StatusNotFound, `{"message":"Not Found"}`, "cannot see it"},
		{http.StatusUnprocessableEntity, `{"message":"Validation Failed","errors":[{"message":"A pull request already exists"}]}`, "already be open"},
		{http.StatusInternalServerError, `{"message":"boom"}`, "github returned 500"},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			fmt.Fprint(w, tc.body)
		}))
		c := &Client{BaseURL: srv.URL, Token: token, HTTP: srv.Client()}
		_, err := c.CreatePR(context.Background(), "o", "r", samplePR())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("status %d: want error containing %q, got %v", tc.status, tc.want, err)
		}
		if err != nil && strings.Contains(err.Error(), token) {
			t.Errorf("status %d: token leaked into error", tc.status)
		}
		srv.Close()
	}
}

// A 201 with no URL means GitHub changed its contract; don't report success.
func TestCreatePR_SuccessWithoutURLIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"number":7}`)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Token: token, HTTP: srv.Client()}
	if _, err := c.CreatePR(context.Background(), "o", "r", samplePR()); err == nil {
		t.Error("want error when GitHub returns no pull request URL")
	}
}

// Only pull-request creation. No merge, no force-push, no repo admin.
func TestCreatePR_OnlyEverPostsToPullsEndpoint(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"number":1,"html_url":"https://x/pull/1"}`)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Token: token, HTTP: srv.Client()}
	if _, err := c.CreatePR(context.Background(), "o", "r", samplePR()); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "POST /repos/o/r/pulls" {
		t.Errorf("requests = %v, want exactly one POST to the pulls endpoint", paths)
	}
	for _, p := range paths {
		if strings.Contains(p, "/merge") || strings.Contains(p, "PUT") || strings.Contains(p, "DELETE") {
			t.Errorf("unexpected mutating request %q", p)
		}
	}
}

func TestTokenFromEnv(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	if got := TokenFromEnv(); got != "" {
		t.Errorf("want empty, got %q", got)
	}
	t.Setenv("GH_TOKEN", "  from-gh  ")
	if got := TokenFromEnv(); got != "from-gh" {
		t.Errorf("want trimmed GH_TOKEN, got %q", got)
	}
	t.Setenv("GITHUB_TOKEN", "from-github")
	if got := TokenFromEnv(); got != "from-github" {
		t.Errorf("GITHUB_TOKEN should win, got %q", got)
	}
}
