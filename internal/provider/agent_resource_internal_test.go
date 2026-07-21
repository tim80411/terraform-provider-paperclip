package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func agentBaseModel() agentResourceModel {
	return agentResourceModel{
		ID:           types.StringValue("a1"),
		CompanyID:    types.StringValue("co1"),
		Name:         types.StringValue("Chief"),
		Role:         types.StringValue("ceo"),
		Title:        types.StringValue("Boss"),
		Icon:         types.StringValue("crown"),
		Capabilities: types.StringValue("leads"),
		ReportsTo:    types.StringNull(),
	}
}

func TestBuildAgentUpdateInput_NothingChanged(t *testing.T) {
	state := agentBaseModel()
	plan := state

	in := buildAgentUpdateInput(plan, state)

	if in.Name != nil || in.Role != nil || in.Title != nil || in.Icon != nil ||
		in.Capabilities != nil || in.ReportsTo != nil {
		t.Errorf("expected all nil, got %+v", in)
	}
}

func TestBuildAgentUpdateInput_OnlyNameChanged(t *testing.T) {
	state := agentBaseModel()
	plan := state
	plan.Name = types.StringValue("Renamed")

	in := buildAgentUpdateInput(plan, state)

	if in.Name == nil || *in.Name != "Renamed" {
		t.Fatalf("Name = %v, want pointer to Renamed", in.Name)
	}
	if in.Role != nil || in.Title != nil || in.Icon != nil || in.Capabilities != nil || in.ReportsTo != nil {
		t.Errorf("other fields must stay nil: %+v", in)
	}
}

func TestBuildAgentUpdateInput_EachScalarField(t *testing.T) {
	state := agentBaseModel()

	// role
	plan := state
	plan.Role = types.StringValue("engineer")
	if in := buildAgentUpdateInput(plan, state); in.Role == nil || *in.Role != "engineer" {
		t.Errorf("role not carried: %+v", in)
	}
	// title
	plan = state
	plan.Title = types.StringValue("New Title")
	if in := buildAgentUpdateInput(plan, state); in.Title == nil || *in.Title != "New Title" {
		t.Errorf("title not carried: %+v", in)
	}
	// icon
	plan = state
	plan.Icon = types.StringValue("bot")
	if in := buildAgentUpdateInput(plan, state); in.Icon == nil || *in.Icon != "bot" {
		t.Errorf("icon not carried: %+v", in)
	}
	// capabilities
	plan = state
	plan.Capabilities = types.StringValue("does more")
	if in := buildAgentUpdateInput(plan, state); in.Capabilities == nil || *in.Capabilities != "does more" {
		t.Errorf("capabilities not carried: %+v", in)
	}
	// reports_to null -> set
	plan = state
	plan.ReportsTo = types.StringValue("boss-2")
	if in := buildAgentUpdateInput(plan, state); in.ReportsTo == nil || *in.ReportsTo != "boss-2" {
		t.Errorf("reports_to not carried: %+v", in)
	}
}

func TestSecretRef_SerializesLiveShape(t *testing.T) {
	// live 探測：env secret_ref 完整形狀。provider 送完整 5 欄位，讓 read-back 不漂移。
	ref := secretRef("sec-123")
	if ref["type"] != "secret_ref" {
		t.Errorf(`type = %v, want "secret_ref"`, ref["type"])
	}
	if ref["version"] != "latest" {
		t.Errorf(`version = %v, want "latest"`, ref["version"])
	}
	if ref["secretId"] != "sec-123" {
		t.Errorf(`secretId = %v, want "sec-123"`, ref["secretId"])
	}
	if ref["projectionClass"] != "unclassified" {
		t.Errorf(`projectionClass = %v, want "unclassified"`, ref["projectionClass"])
	}
	v, ok := ref["projectionAllowlistKey"]
	if !ok || v != nil {
		t.Errorf("projectionAllowlistKey = %v (present=%v), want explicit nil", v, ok)
	}
}
