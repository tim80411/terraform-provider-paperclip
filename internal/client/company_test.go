// internal/client/company_test.go
package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// GET /api/companies 回傳 JSON 陣列（server 端 res.json(result.filter(...)) 實證）。
func TestListCompanies_ParsesArray(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`[{"id":"c1","name":"Acme"},{"id":"c2","name":"Beta","description":"d2"}]`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	out, err := c.ListCompanies(context.Background())
	if err != nil {
		t.Fatalf("ListCompanies returned error: %v", err)
	}
	if gotMethod != "GET" || gotPath != "/api/companies" {
		t.Errorf("method/path = %s %s, want GET /api/companies", gotMethod, gotPath)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].ID != "c1" || out[0].Name != "Acme" {
		t.Errorf("out[0] = %+v", out[0])
	}
	if out[1].ID != "c2" || out[1].Name != "Beta" || out[1].Description != "d2" {
		t.Errorf("out[1] = %+v", out[1])
	}
}

func TestListCompanies_EmptyArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	out, err := c.ListCompanies(context.Background())
	if err != nil {
		t.Fatalf("ListCompanies returned error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("len = %d, want 0", len(out))
	}
}
