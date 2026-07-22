package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// mkGoalIds builds a known set; mkGoalIds() is the empty set [], distinct from
// the null set (types.SetNull) used for "attribute absent". goal_ids is a Set
// (not a List) because the API does not preserve goalIds order.
func mkGoalIds(vals ...string) types.Set {
	elems := make([]attr.Value, 0, len(vals))
	for _, v := range vals {
		elems = append(elems, types.StringValue(v))
	}
	return types.SetValueMust(types.StringType, elems)
}

func projectBaseModel() projectResourceModel {
	return projectResourceModel{
		ID:          types.StringValue("p1"),
		CompanyID:   types.StringValue("co1"),
		Name:        types.StringValue("Proj"),
		Description: types.StringValue("d"),
		Status:      types.StringValue("planned"),
		LeadAgentId: types.StringNull(),
		GoalIds:     types.SetNull(types.StringType),
		Workspace:   types.ObjectNull(workspaceAttrTypes),
	}
}

func TestBuildProjectUpdateInput_NothingChanged(t *testing.T) {
	state := projectBaseModel()
	plan := state

	in := buildProjectUpdateInput(plan, state)

	if in.Name != nil || in.Description != nil || in.Status != nil {
		t.Errorf("expected all scalar pointers nil, got %+v", in)
	}
	if in.LeadAgentId != nil {
		t.Errorf("expected lead_agent_id omitted (nil RawMessage), got %s", string(in.LeadAgentId))
	}
	if in.GoalIds != nil {
		t.Errorf("expected goal_ids omitted (nil pointer), got %+v", *in.GoalIds)
	}
	if !projectUpdateInputEmpty(in) {
		t.Error("no changes must report empty")
	}
}

func TestBuildProjectUpdateInput_EachScalarField(t *testing.T) {
	state := projectBaseModel()

	plan := state
	plan.Name = types.StringValue("Renamed")
	if in := buildProjectUpdateInput(plan, state); in.Name == nil || *in.Name != "Renamed" {
		t.Errorf("name not carried: %+v", in)
	}

	plan = state
	plan.Description = types.StringValue("new")
	if in := buildProjectUpdateInput(plan, state); in.Description == nil || *in.Description != "new" {
		t.Errorf("description not carried: %+v", in)
	}

	plan = state
	plan.Status = types.StringValue("in_progress")
	if in := buildProjectUpdateInput(plan, state); in.Status == nil || *in.Status != "in_progress" {
		t.Errorf("status not carried: %+v", in)
	}
}

// TestBuildProjectUpdateInput_LeadAgentTriState pins the three cases the
// removal=clear policy requires for lead_agent_id (mirrors goal/agent refs):
//   - unchanged   → nil RawMessage (omitted → live value untouched)
//   - set → value → JSON string
//   - set → null  → JSON null (clears the lead; live-proven 2026-07-22)
func TestBuildProjectUpdateInput_LeadAgentTriState(t *testing.T) {
	// unchanged (both null)
	state := projectBaseModel()
	plan := state
	if in := buildProjectUpdateInput(plan, state); in.LeadAgentId != nil {
		t.Errorf("unchanged null lead_agent_id must omit: %s", string(in.LeadAgentId))
	}

	// unchanged (both same value)
	state = projectBaseModel()
	state.LeadAgentId = types.StringValue("a1")
	plan = state
	if in := buildProjectUpdateInput(plan, state); in.LeadAgentId != nil {
		t.Errorf("unchanged value lead_agent_id must omit: %s", string(in.LeadAgentId))
	}

	// set (a1) → null: emit explicit JSON null so the lead clears.
	state = projectBaseModel()
	state.LeadAgentId = types.StringValue("a1")
	plan = state
	plan.LeadAgentId = types.StringNull()
	if in := buildProjectUpdateInput(plan, state); string(in.LeadAgentId) != "null" {
		t.Errorf("set→null lead_agent_id must emit JSON null, got %q", string(in.LeadAgentId))
	}

	// changed value (a1) → (a2): emit new JSON string.
	state = projectBaseModel()
	state.LeadAgentId = types.StringValue("a1")
	plan = state
	plan.LeadAgentId = types.StringValue("a2")
	if in := buildProjectUpdateInput(plan, state); string(in.LeadAgentId) != `"a2"` {
		t.Errorf("changed lead_agent_id must emit new JSON string, got %q", string(in.LeadAgentId))
	}
}

// TestBuildProjectUpdateInput_GoalIds pins goal_ids serialization, the critical
// removal=clear behavior R5 depends on:
//   - unchanged           → nil pointer (omitted → links untouched)
//   - changed to values   → &[…] (set)
//   - emptied ([g1]→[])   → &[] (empty array → CLEARS all links, live-proven)
//   - removed ([g1]→null) → &[] (also clears — removing the attr must not no-op)
func TestBuildProjectUpdateInput_GoalIds(t *testing.T) {
	// unchanged (both null)
	state := projectBaseModel()
	plan := state
	if in := buildProjectUpdateInput(plan, state); in.GoalIds != nil {
		t.Errorf("unchanged null goal_ids must omit, got %+v", *in.GoalIds)
	}

	// unchanged (both [g1])
	state = projectBaseModel()
	state.GoalIds = mkGoalIds("g1")
	plan = state
	if in := buildProjectUpdateInput(plan, state); in.GoalIds != nil {
		t.Errorf("unchanged value goal_ids must omit, got %+v", *in.GoalIds)
	}

	// changed [g1] → [g1,g2]: send both (Set → order not asserted).
	state = projectBaseModel()
	state.GoalIds = mkGoalIds("g1")
	plan = state
	plan.GoalIds = mkGoalIds("g1", "g2")
	in := buildProjectUpdateInput(plan, state)
	if in.GoalIds == nil || len(*in.GoalIds) != 2 {
		t.Fatalf("changed goal_ids must send 2 ids, got %+v", in.GoalIds)
	}
	got := map[string]bool{}
	for _, id := range *in.GoalIds {
		got[id] = true
	}
	if !got["g1"] || !got["g2"] {
		t.Errorf("changed goal_ids must contain g1 and g2, got %+v", *in.GoalIds)
	}

	// emptied [g1] → []: send empty (non-nil) slice → clears links.
	state = projectBaseModel()
	state.GoalIds = mkGoalIds("g1")
	plan = state
	plan.GoalIds = mkGoalIds()
	in = buildProjectUpdateInput(plan, state)
	if in.GoalIds == nil {
		t.Fatal("emptied goal_ids must send a non-nil (empty) slice, got nil")
	}
	if len(*in.GoalIds) != 0 {
		t.Errorf("emptied goal_ids must send [], got %+v", *in.GoalIds)
	}

	// removed [g1] → null: also clears (send empty slice, not omit).
	state = projectBaseModel()
	state.GoalIds = mkGoalIds("g1")
	plan = state
	plan.GoalIds = types.SetNull(types.StringType)
	in = buildProjectUpdateInput(plan, state)
	if in.GoalIds == nil {
		t.Fatal("removed goal_ids must clear (non-nil empty slice), got nil")
	}
	if len(*in.GoalIds) != 0 {
		t.Errorf("removed goal_ids must send [], got %+v", *in.GoalIds)
	}
}

func TestProjectUpdateInputEmpty(t *testing.T) {
	if !projectUpdateInputEmpty(buildProjectUpdateInput(projectBaseModel(), projectBaseModel())) {
		t.Error("no changes must report empty")
	}
	state := projectBaseModel()
	plan := state
	plan.Name = types.StringValue("Renamed")
	if projectUpdateInputEmpty(buildProjectUpdateInput(plan, state)) {
		t.Error("changed name must NOT report empty")
	}
	state = projectBaseModel()
	state.GoalIds = mkGoalIds("g1")
	plan = state
	plan.GoalIds = mkGoalIds()
	if projectUpdateInputEmpty(buildProjectUpdateInput(plan, state)) {
		t.Error("cleared goal_ids must NOT report empty")
	}
	state = projectBaseModel()
	state.LeadAgentId = types.StringValue("a1")
	plan = state
	plan.LeadAgentId = types.StringNull()
	if projectUpdateInputEmpty(buildProjectUpdateInput(plan, state)) {
		t.Error("cleared lead_agent_id must NOT report empty")
	}
}
