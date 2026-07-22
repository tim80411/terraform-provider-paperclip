// internal/provider/company_data_source_internal_test.go
package provider

import (
	"strings"
	"testing"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

// live 實證：公司 name 不強制唯一（僅 issuePrefix 唯一），所以 by-name 查詢
// 必須處理 0 / 1 / N 三種命中數。
func TestFindCompanyByName_ExactlyOne(t *testing.T) {
	companies := []client.Company{
		{ID: "c1", Name: "Acme"},
		{ID: "c2", Name: "Beta", Description: "d2"},
	}

	got, err := findCompanyByName(companies, "Beta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "c2" || got.Description != "d2" {
		t.Errorf("got = %+v, want c2/d2", got)
	}
}

func TestFindCompanyByName_NotFound_ErrorContainsName(t *testing.T) {
	companies := []client.Company{{ID: "c1", Name: "Acme"}}

	_, err := findCompanyByName(companies, "Ghost")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Ghost") {
		t.Errorf("error %q should contain the queried name", err)
	}
}

func TestFindCompanyByName_Ambiguous_ErrorSuggestsID(t *testing.T) {
	companies := []client.Company{
		{ID: "c1", Name: "Dup"},
		{ID: "c2", Name: "Dup"},
	}

	_, err := findCompanyByName(companies, "Dup")
	if err == nil {
		t.Fatal("expected ambiguity error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "c1") || !strings.Contains(msg, "c2") {
		t.Errorf("error %q should list both candidate ids", msg)
	}
	if !strings.Contains(msg, "id") {
		t.Errorf("error %q should hint at using id", msg)
	}
}

func TestFindCompanyByName_ExactMatchOnly_NoSubstring(t *testing.T) {
	companies := []client.Company{{ID: "c1", Name: "Acme Corp"}}

	_, err := findCompanyByName(companies, "Acme")
	if err == nil {
		t.Fatal("substring should not match; expected not-found error")
	}
}
