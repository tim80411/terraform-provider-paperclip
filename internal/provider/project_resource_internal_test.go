package provider

import (
	"context"
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

	in, _ := buildProjectUpdateInput(plan, state)

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
	if in, _ := buildProjectUpdateInput(plan, state); in.Name == nil || *in.Name != "Renamed" {
		t.Errorf("name not carried: %+v", in)
	}

	plan = state
	plan.Description = types.StringValue("new")
	if in, _ := buildProjectUpdateInput(plan, state); in.Description == nil || *in.Description != "new" {
		t.Errorf("description not carried: %+v", in)
	}

	plan = state
	plan.Status = types.StringValue("in_progress")
	if in, _ := buildProjectUpdateInput(plan, state); in.Status == nil || *in.Status != "in_progress" {
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
	if in, _ := buildProjectUpdateInput(plan, state); in.LeadAgentId != nil {
		t.Errorf("unchanged null lead_agent_id must omit: %s", string(in.LeadAgentId))
	}

	// unchanged (both same value)
	state = projectBaseModel()
	state.LeadAgentId = types.StringValue("a1")
	plan = state
	if in, _ := buildProjectUpdateInput(plan, state); in.LeadAgentId != nil {
		t.Errorf("unchanged value lead_agent_id must omit: %s", string(in.LeadAgentId))
	}

	// set (a1) → null: emit explicit JSON null so the lead clears.
	state = projectBaseModel()
	state.LeadAgentId = types.StringValue("a1")
	plan = state
	plan.LeadAgentId = types.StringNull()
	if in, _ := buildProjectUpdateInput(plan, state); string(in.LeadAgentId) != "null" {
		t.Errorf("set→null lead_agent_id must emit JSON null, got %q", string(in.LeadAgentId))
	}

	// changed value (a1) → (a2): emit new JSON string.
	state = projectBaseModel()
	state.LeadAgentId = types.StringValue("a1")
	plan = state
	plan.LeadAgentId = types.StringValue("a2")
	if in, _ := buildProjectUpdateInput(plan, state); string(in.LeadAgentId) != `"a2"` {
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
	if in, _ := buildProjectUpdateInput(plan, state); in.GoalIds != nil {
		t.Errorf("unchanged null goal_ids must omit, got %+v", *in.GoalIds)
	}

	// unchanged (both [g1])
	state = projectBaseModel()
	state.GoalIds = mkGoalIds("g1")
	plan = state
	if in, _ := buildProjectUpdateInput(plan, state); in.GoalIds != nil {
		t.Errorf("unchanged value goal_ids must omit, got %+v", *in.GoalIds)
	}

	// changed [g1] → [g1,g2]: send both (Set → order not asserted).
	state = projectBaseModel()
	state.GoalIds = mkGoalIds("g1")
	plan = state
	plan.GoalIds = mkGoalIds("g1", "g2")
	in, _ := buildProjectUpdateInput(plan, state)
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
	in, _ = buildProjectUpdateInput(plan, state)
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
	in, _ = buildProjectUpdateInput(plan, state)
	if in.GoalIds == nil {
		t.Fatal("removed goal_ids must clear (non-nil empty slice), got nil")
	}
	if len(*in.GoalIds) != 0 {
		t.Errorf("removed goal_ids must send [], got %+v", *in.GoalIds)
	}
}

// TestReconcileGoalIds pins the drift-detection contract of reconcileGoalIds,
// the load-bearing regression guard for the out-of-band-clear blind spot:
//   - server has goals            → reflect them (regardless of base).
//   - server empty + base null    → stay null (config absent / fresh / import:
//                                    avoids the phantom null↔[] diff).
//   - server empty + base unknown → stay unknown (never invent [] for unknown).
//   - server empty + base []      → empty set (matches; no churn).
//   - server empty + base [g1]    → EMPTY set (a full out-of-band clear MUST be
//                                    visible as drift, not hidden by returning base).
func TestReconcileGoalIds(t *testing.T) {
	ctx := context.Background()

	// server has goals → reflect them (base irrelevant).
	got, d := reconcileGoalIds(ctx, types.SetNull(types.StringType), []string{"g1", "g2"})
	if d.HasError() {
		t.Fatalf("unexpected diags: %+v", d)
	}
	if got.IsNull() || len(got.Elements()) != 2 {
		t.Errorf("server goals must be reflected, got %+v", got)
	}

	// server empty + base NULL → preserve null (phantom-diff-safe; import/create-safe).
	got, d = reconcileGoalIds(ctx, types.SetNull(types.StringType), nil)
	if d.HasError() {
		t.Fatalf("unexpected diags: %+v", d)
	}
	if !got.IsNull() {
		t.Errorf("server empty + base null must stay null, got %+v", got)
	}

	// server empty + base UNKNOWN → preserve unknown.
	got, _ = reconcileGoalIds(ctx, types.SetUnknown(types.StringType), nil)
	if !got.IsUnknown() {
		t.Errorf("server empty + base unknown must stay unknown, got %+v", got)
	}

	// server empty + base EMPTY known set → empty set (no churn).
	got, d = reconcileGoalIds(ctx, mkGoalIds(), nil)
	if d.HasError() {
		t.Fatalf("unexpected diags: %+v", d)
	}
	if got.IsNull() || len(got.Elements()) != 0 {
		t.Errorf("server empty + base empty-set must be empty set, got %+v", got)
	}

	// THE FIX: server empty + base POPULATED known set → EMPTY set so a full
	// out-of-band clear (state=[g1], server=[]) surfaces as drift on the next plan.
	got, d = reconcileGoalIds(ctx, mkGoalIds("g1"), nil)
	if d.HasError() {
		t.Fatalf("unexpected diags: %+v", d)
	}
	if got.IsNull() {
		t.Fatal("out-of-band clear must NOT be null (would suppress the diff differently), got null")
	}
	if len(got.Elements()) != 0 {
		t.Errorf("out-of-band goal clear (state=[g1], server=[]) must reflect [] so drift is visible, got %+v", got)
	}
}

// TestGoalIdsToSlice pins goalIdsToSlice's contract: null/unknown → empty slice
// (so a cleared attribute sends []), a known set → its ids, and neither yields a
// diagnostic for well-typed input. The non-String else-branch is defensive only:
// goal_ids' ElementType is StringType, so the framework's Set type cannot hold a
// non-String element — it is unconstructable via the public API, hence not unit-
// testable here; the diagnostic exists so a future refactor can never silently
// drop an id.
func TestGoalIdsToSlice(t *testing.T) {
	if ids, d := goalIdsToSlice(types.SetNull(types.StringType)); d.HasError() || len(ids) != 0 {
		t.Errorf("null set → empty slice, no diags; got ids=%v diags=%+v", ids, d)
	}
	if ids, d := goalIdsToSlice(types.SetUnknown(types.StringType)); d.HasError() || len(ids) != 0 {
		t.Errorf("unknown set → empty slice, no diags; got ids=%v diags=%+v", ids, d)
	}
	ids, d := goalIdsToSlice(mkGoalIds("g1", "g2"))
	if d.HasError() {
		t.Fatalf("well-typed set must not error: %+v", d)
	}
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	if len(ids) != 2 || !set["g1"] || !set["g2"] {
		t.Errorf("known set must yield its ids, got %v", ids)
	}
}

func TestProjectUpdateInputEmpty(t *testing.T) {
	in, _ := buildProjectUpdateInput(projectBaseModel(), projectBaseModel())
	if !projectUpdateInputEmpty(in) {
		t.Error("no changes must report empty")
	}
	state := projectBaseModel()
	plan := state
	plan.Name = types.StringValue("Renamed")
	in, _ = buildProjectUpdateInput(plan, state)
	if projectUpdateInputEmpty(in) {
		t.Error("changed name must NOT report empty")
	}
	state = projectBaseModel()
	state.GoalIds = mkGoalIds("g1")
	plan = state
	plan.GoalIds = mkGoalIds()
	in, _ = buildProjectUpdateInput(plan, state)
	if projectUpdateInputEmpty(in) {
		t.Error("cleared goal_ids must NOT report empty")
	}
	state = projectBaseModel()
	state.LeadAgentId = types.StringValue("a1")
	plan = state
	plan.LeadAgentId = types.StringNull()
	in, _ = buildProjectUpdateInput(plan, state)
	if projectUpdateInputEmpty(in) {
		t.Error("cleared lead_agent_id must NOT report empty")
	}
}
