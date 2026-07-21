package client

import (
	"context"
	"encoding/json"
	"io"
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

func TestUpdateCompany_OmitsUnsetFields(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"c1","name":"Renamed"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	newName := "Renamed"
	_, err := c.UpdateCompany(context.Background(), "c1", CompanyUpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("UpdateCompany: %v", err)
	}
	if _, ok := gotBody["name"]; !ok {
		t.Error("body missing name")
	}
	// 核心：description / slug 未設 → 不得出現在 body（保留未管欄位）
	if _, ok := gotBody["description"]; ok {
		t.Error("body must NOT contain description when unset (would clobber)")
	}
	if _, ok := gotBody["slug"]; ok {
		t.Error("body must NOT contain slug when unset (would clobber)")
	}
}

func TestGetCompany_404IsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	_, err := c.GetCompany(context.Background(), "missing-id")
	if err == nil {
		t.Fatal("expected error on 404, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(err) = false, want true for 404 response; err = %v", err)
	}
}

func TestGetCompany_500IsNotNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	_, err := c.GetCompany(context.Background(), "some-id")
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
	if IsNotFound(err) {
		t.Errorf("IsNotFound(err) = true, want false for 500 response; err = %v", err)
	}
}

func TestCreateCompany_ParsesID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":"c9","name":"Acme","slug":"acme"}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "tok")
	got, err := c.CreateCompany(context.Background(), CompanyCreateInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	if got.ID != "c9" || got.Slug != "acme" {
		t.Errorf("got %+v", got)
	}
}
