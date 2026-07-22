// internal/client/budget_test.go
package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdateCompanyBudget_PatchesCents(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"c1","name":"Acme","budgetMonthlyCents":50000}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	out, err := c.UpdateCompanyBudget(context.Background(), "c1", 50000)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if gotMethod != "PATCH" || gotPath != "/api/companies/c1/budgets" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if gotBody["budgetMonthlyCents"] != float64(50000) {
		t.Errorf("body = %v", gotBody)
	}
	if out.BudgetMonthlyCents != 50000 {
		t.Errorf("out.BudgetMonthlyCents = %d", out.BudgetMonthlyCents)
	}
}

// destroy 語意：PATCH 0 = 還原 DB 預設（budget_monthly_cents default 0）。
func TestUpdateCompanyBudget_ZeroIsValid(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"c1","budgetMonthlyCents":0}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if _, err := c.UpdateCompanyBudget(context.Background(), "c1", 0); err != nil {
		t.Fatalf("error: %v", err)
	}
	if v, has := gotBody["budgetMonthlyCents"]; !has || v != float64(0) {
		t.Errorf("zero must be sent explicitly, body = %v", gotBody)
	}
}

func TestGetCompany_ParsesBudgetCents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"c1","name":"Acme","budgetMonthlyCents":12300}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	out, err := c.GetCompany(context.Background(), "c1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if out.BudgetMonthlyCents != 12300 {
		t.Errorf("BudgetMonthlyCents = %d, want 12300", out.BudgetMonthlyCents)
	}
}
