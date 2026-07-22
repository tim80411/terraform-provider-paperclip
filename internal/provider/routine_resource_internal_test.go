// internal/provider/routine_resource_internal_test.go
package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestBuildRoutineUpdateInput_NothingChanged(t *testing.T) {
	state := routineResourceModel{
		ID:              types.StringValue("rt1"),
		CompanyID:       types.StringValue("c1"),
		Title:           types.StringValue("t"),
		Description:     types.StringValue("d"),
		Status:          types.StringValue("active"),
		AssigneeAgentID: types.StringValue("a1"),
	}
	plan := state

	in := buildRoutineUpdateInput(plan, state)

	if in.Title != nil || in.Description != nil || in.Status != nil || in.AssigneeAgentId != nil {
		t.Errorf("no-op diff must produce empty input, got %+v", in)
	}
}

func TestBuildRoutineUpdateInput_TitleAndStatusChanged(t *testing.T) {
	state := routineResourceModel{
		ID:     types.StringValue("rt1"),
		Title:  types.StringValue("t"),
		Status: types.StringValue("active"),
	}
	plan := state
	plan.Title = types.StringValue("t2")
	plan.Status = types.StringValue("paused")

	in := buildRoutineUpdateInput(plan, state)

	if in.Title == nil || *in.Title != "t2" {
		t.Errorf("Title = %v, want t2", in.Title)
	}
	if in.Status == nil || *in.Status != "paused" {
		t.Errorf("Status = %v, want paused", in.Status)
	}
	if in.AssigneeAgentId != nil {
		t.Errorf("unchanged assignee must stay nil, got %s", in.AssigneeAgentId)
	}
}

func TestBuildRoutineUpdateInput_AssigneeCleared_SendsNull(t *testing.T) {
	state := routineResourceModel{
		ID:              types.StringValue("rt1"),
		AssigneeAgentID: types.StringValue("a1"),
	}
	plan := state
	plan.AssigneeAgentID = types.StringNull() // config 移除 assignee

	in := buildRoutineUpdateInput(plan, state)

	if string(in.AssigneeAgentId) != "null" {
		t.Errorf("cleared assignee must send explicit JSON null, got %q", in.AssigneeAgentId)
	}
}

func TestBuildRoutineUpdateInput_AssigneeSet_SendsQuotedID(t *testing.T) {
	state := routineResourceModel{
		ID: types.StringValue("rt1"),
	}
	plan := state
	plan.AssigneeAgentID = types.StringValue("a2")

	in := buildRoutineUpdateInput(plan, state)

	if string(in.AssigneeAgentId) != `"a2"` {
		t.Errorf("set assignee must send quoted id, got %q", in.AssigneeAgentId)
	}
}
