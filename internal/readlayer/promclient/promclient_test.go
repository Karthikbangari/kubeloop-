package promclient

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestNew_TrimsTrailingSlash(t *testing.T) {
	if c := New("http://x:9090/", nil); c.baseURL != "http://x:9090" {
		t.Errorf("baseURL = %q, want trailing slash trimmed", c.baseURL)
	}
}
