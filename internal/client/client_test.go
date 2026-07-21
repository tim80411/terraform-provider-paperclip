package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDo_SetsBearerAndParsesJSON(t *testing.T) {
	var gotAuth, gotCT, gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"c1","name":"Acme"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok123")
	var out struct{ ID, Name string }
	in := map[string]string{"name": "Acme"}
	if err := c.do(context.Background(), "POST", "/api/companies", in, &out); err != nil {
		t.Fatalf("do returned error: %v", err)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth = %q, want Bearer tok123", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q, want application/json", gotCT)
	}
	if gotMethod != "POST" || gotPath != "/api/companies" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if out.ID != "c1" || out.Name != "Acme" {
		t.Errorf("out = %+v", out)
	}
}

func TestDo_Noningred2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "tok")
	err := c.do(context.Background(), "GET", "/api/companies/x", nil, nil)
	if err == nil {
		t.Fatal("expected error on 403, got nil")
	}
}
