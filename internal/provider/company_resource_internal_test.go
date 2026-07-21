// internal/provider/company_resource_internal_test.go
package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestBuildCompanyUpdateInput_NothingChanged(t *testing.T) {
	state := companyResourceModel{
		ID:          types.StringValue("c1"),
		Name:        types.StringValue("Acme"),
		Description: types.StringValue("d1"),
	}
	plan := state

	in := buildCompanyUpdateInput(plan, state)

	if in.Name != nil {
		t.Errorf("Name = %v, want nil", *in.Name)
	}
	if in.Description != nil {
		t.Errorf("Description = %v, want nil", *in.Description)
	}
}

func TestBuildCompanyUpdateInput_OnlyNameChanged(t *testing.T) {
	state := companyResourceModel{
		ID:          types.StringValue("c1"),
		Name:        types.StringValue("Acme"),
		Description: types.StringValue("d1"),
	}
	plan := state
	plan.Name = types.StringValue("new")

	in := buildCompanyUpdateInput(plan, state)

	if in.Name == nil || *in.Name != "new" {
		t.Fatalf("Name = %v, want pointer to \"new\"", in.Name)
	}
	if in.Description != nil {
		t.Errorf("Description = %v, want nil", *in.Description)
	}
}

func TestBuildCompanyUpdateInput_OnlyDescriptionChanged(t *testing.T) {
	state := companyResourceModel{
		ID:          types.StringValue("c1"),
		Name:        types.StringValue("Acme"),
		Description: types.StringValue("d1"),
	}
	plan := state
	plan.Description = types.StringValue("d2")

	in := buildCompanyUpdateInput(plan, state)

	if in.Description == nil || *in.Description != "d2" {
		t.Fatalf("Description = %v, want pointer to \"d2\"", in.Description)
	}
	if in.Name != nil {
		t.Errorf("Name = %v, want nil", *in.Name)
	}
}
