package promclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQuery_FetchesAndParses(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("query")
		if r.URL.Path != "/api/v1/query" {
			t.Errorf("path = %q, want /api/v1/query", r.URL.Path)
		}
		w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"value":[1720200000,"0.41"]}]}}`))
	}))
	defer srv.Close()

	v, ok, err := New(srv.URL, srv.Client()).Query(context.Background(), `avg(rate(cpu[5m]))`)
	if err != nil || !ok {
		t.Fatalf("err=%v ok=%v, want a value", err, ok)
	}
	if v != 0.41 {
		t.Errorf("value = %v, want 0.41", v)
	}
	if gotQuery != `avg(rate(cpu[5m]))` {
		t.Errorf("server received query %q, want it URL-decoded intact", gotQuery)
	}
}

func TestQuery_EmptyResultIsMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
	}))
	defer srv.Close()

	v, ok, err := New(srv.URL, srv.Client()).Query(context.Background(), "q")
	if err != nil || ok || v != 0 {
		t.Errorf("got v=%v ok=%v err=%v, want 0/false/nil (missing)", v, ok, err)
	}
}

func TestQuery_Non200Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, _, err := New(srv.URL, srv.Client()).Query(context.Background(), "q"); err == nil {
		t.Error("want error on non-200 response")
	}
}

// A transport failure must report the cause, not bury it under a few hundred
// characters of percent-encoded PromQL (which is what *url.Error prints).
func TestQuery_TransportErrorIsReadable(t *testing.T) {
	c := New("http://127.0.0.1:1", nil) // nothing listens on port 1
	_, _, err := c.Query(context.Background(), `max(quantile_over_time(0.95, sum by (pod) (rate(x{a!="",b=~"c-.*"}[5m]))[7d:5m]))`)
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "%28") || strings.Contains(err.Error(), "quantile_over_time") {
		t.Errorf("the encoded query leaked into the error: %v", err)
	}
	if !strings.Contains(err.Error(), "http://127.0.0.1:1") {
		t.Errorf("error should name the Prometheus base URL: %v", err)
	}
}

func TestNew_TrimsTrailingSlash(t *testing.T) {
	if c := New("http://x:9090/", nil); c.baseURL != "http://x:9090" {
		t.Errorf("baseURL = %q, want trailing slash trimmed", c.baseURL)
	}
}
