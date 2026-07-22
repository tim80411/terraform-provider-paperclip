package provider

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func goalBaseModel() goalResourceModel {
	return goalResourceModel{
		ID:           types.StringValue("g1"),
		CompanyID:    types.StringValue("co1"),
		Title:        types.StringValue("Child Goal"),
		Description:  types.StringValue("child"),
		Level:        types.StringValue("task"),
		Status:       types.StringValue("planned"),
		OwnerAgentId: types.StringNull(),
		ParentId:     types.StringNull(),
	}
}

func TestBuildGoalUpdateInput_NothingChanged(t *testing.T) {
	state := goalBaseModel()
	plan := state

	in := buildGoalUpdateInput(plan, state)

	if in.Title != nil || in.Description != nil || in.Level != nil || in.Status != nil {
		t.Errorf("expected all scalar pointers nil, got %+v", in)
	}
	if in.OwnerAgentId != nil || in.ParentId != nil {
		t.Errorf("expected both refs omitted (nil RawMessage), got %+v", in)
	}
}

func TestBuildGoalUpdateInput_EachScalarField(t *testing.T) {
	state := goalBaseModel()

	plan := state
	plan.Title = types.StringValue("Renamed")
	if in := buildGoalUpdateInput(plan, state); in.Title == nil || *in.Title != "Renamed" {
		t.Errorf("title not carried: %+v", in)
	}

	plan = state
	plan.Description = types.StringValue("new description")
	if in := buildGoalUpdateInput(plan, state); in.Description == nil || *in.Description != "new description" {
		t.Errorf("description not carried: %+v", in)
	}

	plan = state
	plan.Level = types.StringValue("company")
	if in := buildGoalUpdateInput(plan, state); in.Level == nil || *in.Level != "company" {
		t.Errorf("level not carried: %+v", in)
	}

	plan = state
	plan.Status = types.StringValue("active")
	if in := buildGoalUpdateInput(plan, state); in.Status == nil || *in.Status != "active" {
		t.Errorf("status not carried: %+v", in)
	}
}

// TestBuildGoalUpdateInput_RefTriState pins the three cases the removal=clear
// policy requires for owner_agent_id/parent_id (mirrors agent's reports_to fix):
//   - unchanged        → nil RawMessage (omitted from body → live value untouched)
//   - set → value      → JSON string
//   - set → null       → JSON null (clears the ref; live-proven 2026-07-22)
func TestBuildGoalUpdateInput_RefTriState(t *testing.T) {
	t.Run("owner_agent_id", func(t *testing.T) {
		// unchanged (both null)
		state := goalBaseModel()
		plan := state
		if in := buildGoalUpdateInput(plan, state); in.OwnerAgentId != nil {
			t.Errorf("unchanged null owner_agent_id must omit: %s", string(in.OwnerAgentId))
		}

		// unchanged (both same value)
		state = goalBaseModel()
		state.OwnerAgentId = types.StringValue("a1")
		plan = state
		if in := buildGoalUpdateInput(plan, state); in.OwnerAgentId != nil {
			t.Errorf("unchanged value owner_agent_id must omit: %s", string(in.OwnerAgentId))
		}

		// set (a1) → null: emit explicit JSON null so the ref clears.
		state = goalBaseModel()
		state.OwnerAgentId = types.StringValue("a1")
		plan = state
		plan.OwnerAgentId = types.StringNull()
		if in := buildGoalUpdateInput(plan, state); string(in.OwnerAgentId) != "null" {
			t.Errorf("set→null owner_agent_id must emit JSON null, got %q", string(in.OwnerAgentId))
		}

		// changed value (a1) → (a2): emit new JSON string.
		state = goalBaseModel()
		state.OwnerAgentId = types.StringValue("a1")
		plan = state
		plan.OwnerAgentId = types.StringValue("a2")
		if in := buildGoalUpdateInput(plan, state); string(in.OwnerAgentId) != `"a2"` {
			t.Errorf("changed owner_agent_id must emit new JSON string, got %q", string(in.OwnerAgentId))
		}
	})

	t.Run("parent_id", func(t *testing.T) {
		// unchanged (both null)
		state := goalBaseModel()
		plan := state
		if in := buildGoalUpdateInput(plan, state); in.ParentId != nil {
			t.Errorf("unchanged null parent_id must omit: %s", string(in.ParentId))
		}

		// unchanged (both same value)
		state = goalBaseModel()
		state.ParentId = types.StringValue("p1")
		plan = state
		if in := buildGoalUpdateInput(plan, state); in.ParentId != nil {
			t.Errorf("unchanged value parent_id must omit: %s", string(in.ParentId))
		}

		// set (p1) → null: emit explicit JSON null so the ref clears.
		state = goalBaseModel()
		state.ParentId = types.StringValue("p1")
		plan = state
		plan.ParentId = types.StringNull()
		if in := buildGoalUpdateInput(plan, state); string(in.ParentId) != "null" {
			t.Errorf("set→null parent_id must emit JSON null, got %q", string(in.ParentId))
		}

		// reparent (p1) → (p2): emit new JSON string.
		state = goalBaseModel()
		state.ParentId = types.StringValue("p1")
		plan = state
		plan.ParentId = types.StringValue("p2")
		if in := buildGoalUpdateInput(plan, state); string(in.ParentId) != `"p2"` {
			t.Errorf("reparent must emit new JSON string, got %q", string(in.ParentId))
		}
	})
}

func TestGoalUpdateInputEmpty(t *testing.T) {
	if !goalUpdateInputEmpty(buildGoalUpdateInput(goalBaseModel(), goalBaseModel())) {
		t.Error("no changes must report empty")
	}
	state := goalBaseModel()
	plan := state
	plan.Title = types.StringValue("Renamed")
	if goalUpdateInputEmpty(buildGoalUpdateInput(plan, state)) {
		t.Error("changed title must NOT report empty")
	}
	state = goalBaseModel()
	state.ParentId = types.StringValue("p1")
	plan = state
	plan.ParentId = types.StringNull()
	if goalUpdateInputEmpty(buildGoalUpdateInput(plan, state)) {
		t.Error("cleared parent_id must NOT report empty")
	}
}

func TestGoalRefRawMessage_NullVsValue(t *testing.T) {
	// direct sanity on the tri-state helper used by buildGoalUpdateInput.
	if got := goalRefRawMessage(types.StringNull()); string(got) != "null" {
		t.Errorf("plan null must yield JSON null, got %q", string(got))
	}
	v := goalRefRawMessage(types.StringValue("abc"))
	var s string
	if err := json.Unmarshal(v, &s); err != nil || s != "abc" {
		t.Errorf("plan value must yield JSON string, got %q (err=%v)", string(v), err)
	}
}
