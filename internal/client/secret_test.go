package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateSecret_SendsManagedModeAndParsesID(t *testing.T) {
	var gotBody map[string]any
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(201)
		// live 探測：API 把 key 正規化成小寫（GH_TOKEN → gh_token），response 不含 value/hasValue。
		_, _ = w.Write([]byte(`{"id":"s1","name":"gh-token","key":"gh_token","managedMode":"paperclip_managed"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	got, err := c.CreateSecret(context.Background(), "co1", SecretCreateInput{
		Name: "gh-token", Key: "GH_TOKEN", Value: "ghp_xxx", ManagedMode: "paperclip_managed",
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/api/companies/co1/secrets" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if gotBody["managedMode"] != "paperclip_managed" {
		t.Errorf("body missing managedMode=paperclip_managed: %+v", gotBody)
	}
	if gotBody["value"] != "ghp_xxx" {
		t.Errorf("body missing value: %+v", gotBody)
	}
	if got.ID != "s1" || got.Key != "gh_token" {
		t.Errorf("got = %+v", got)
	}
}

func TestGetSecret_FindsByIDInCompanyList(t *testing.T) {
	// live 探測：paperclip 沒有 GET /api/secrets/{id}（回 404 "API route not found"）。
	// GetSecret 靠 list-under-company 端點 + 用 id 過濾實作。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/companies/co1/secrets" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`[
			{"id":"other","name":"n0","key":"k0","managedMode":"paperclip_managed"},
			{"id":"s1","name":"gh-token","key":"gh_token","managedMode":"paperclip_managed"}
		]`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	got, err := c.GetSecret(context.Background(), "co1", "s1")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got.ID != "s1" || got.Name != "gh-token" || got.Key != "gh_token" {
		t.Errorf("got = %+v", got)
	}
}

func TestGetSecret_NotInList_SyntheticNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`[{"id":"other","name":"n0","key":"k0"}]`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	_, err := c.GetSecret(context.Background(), "co1", "missing")
	if err == nil {
		t.Fatal("expected error for id not in list, got nil")
	}
	if !IsNotFound(err) || !IsGone(err) {
		t.Errorf("expected IsNotFound/IsGone true, err = %v", err)
	}
}

func TestGetSecret_CompanyGone403_IsGone(t *testing.T) {
	// live 探測：company 已刪除時 list 端點回 403（"User does not have access to this company"）。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":"User does not have access to this company"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	_, err := c.GetSecret(context.Background(), "gone-co", "s1")
	if err == nil || !IsGone(err) {
		t.Errorf("expected IsGone true, err = %v", err)
	}
}

func TestUpdateSecret_OmitsUnsetFields(t *testing.T) {
	var gotBody map[string]any
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"s1","name":"renamed","key":"gh_token"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	newName := "renamed"
	_, err := c.UpdateSecret(context.Background(), "s1", SecretUpdateInput{Name: &newName})
	if err != nil {
		t.Fatalf("UpdateSecret: %v", err)
	}
	if gotPath != "/api/secrets/s1" {
		t.Errorf("path = %s, want /api/secrets/s1", gotPath)
	}
	if _, ok := gotBody["name"]; !ok {
		t.Error("body missing name")
	}
	if _, ok := gotBody["key"]; ok {
		t.Error("body must NOT contain key when unset (would clobber)")
	}
	// value 從不透過 PATCH 送——SecretUpdateInput 結構上就沒有這個欄位。
	if _, ok := gotBody["value"]; ok {
		t.Error("body must NOT contain value — value only travels through rotate")
	}
}

func TestDeleteSecret(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if err := c.DeleteSecret(context.Background(), "s1"); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	if gotMethod != "DELETE" || gotPath != "/api/secrets/s1" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
}

func TestRotateSecret_SendsValueToRotateEndpoint(t *testing.T) {
	var gotBody map[string]any
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"s1","name":"gh-token","key":"gh_token"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	got, err := c.RotateSecret(context.Background(), "s1", "new-value")
	if err != nil {
		t.Fatalf("RotateSecret: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/api/secrets/s1/rotate" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if gotBody["value"] != "new-value" {
		t.Errorf("body missing value: %+v", gotBody)
	}
	if got.ID != "s1" {
		t.Errorf("got = %+v", got)
	}
}
