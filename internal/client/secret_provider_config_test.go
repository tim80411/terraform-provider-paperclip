// internal/client/secret_provider_config_test.go
package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateSecretProviderConfig_PostsToCompanyPath(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"spc1","provider":"aws_secrets_manager","displayName":"prod-aws","status":"ready","isDefault":false,"config":{"region":"ap-northeast-1"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	out, err := c.CreateSecretProviderConfig(context.Background(), "c1", SecretProviderConfigCreateInput{
		Provider:    "aws_secrets_manager",
		DisplayName: "prod-aws",
		Config:      map[string]any{"region": "ap-northeast-1"},
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if gotPath != "/api/companies/c1/secret-provider-configs" {
		t.Errorf("path = %s", gotPath)
	}
	if gotBody["provider"] != "aws_secrets_manager" || gotBody["displayName"] != "prod-aws" {
		t.Errorf("body = %v", gotBody)
	}
	cfg, _ := gotBody["config"].(map[string]any)
	if cfg["region"] != "ap-northeast-1" {
		t.Errorf("config = %v", cfg)
	}
	if out.ID != "spc1" || out.Status != "ready" {
		t.Errorf("out = %+v", out)
	}
}

func TestGetSecretProviderConfig_UsesFlatPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/secret-provider-configs/spc1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"spc1","provider":"local_encrypted","displayName":"n","isDefault":true,"config":{}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	out, err := c.GetSecretProviderConfig(context.Background(), "spc1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !out.IsDefault || out.Provider != "local_encrypted" {
		t.Errorf("out = %+v", out)
	}
}

func TestUpdateSecretProviderConfig_OmitsUnsetFields(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"spc1"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	name := "renamed"
	if _, err := c.UpdateSecretProviderConfig(context.Background(), "spc1", SecretProviderConfigUpdateInput{DisplayName: &name}); err != nil {
		t.Fatalf("error: %v", err)
	}
	if gotBody["displayName"] != "renamed" {
		t.Errorf("displayName = %v", gotBody["displayName"])
	}
	for _, k := range []string{"status", "isDefault", "config", "provider"} {
		if _, has := gotBody[k]; has {
			t.Errorf("unset %s must be omitted", k)
		}
	}
}

func TestDeleteSecretProviderConfig_SendsDelete(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if err := c.DeleteSecretProviderConfig(context.Background(), "spc1"); err != nil {
		t.Fatalf("error: %v", err)
	}
	if gotMethod != "DELETE" || gotPath != "/api/secret-provider-configs/spc1" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
}
