package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// mkSkills builds a known list; mkSkills() is the empty list [], distinct from
// the null list (types.ListNull) used for "attribute absent / unmanaged".
func mkSkills(vals ...string) types.List {
	elems := make([]attr.Value, 0, len(vals))
	for _, v := range vals {
		elems = append(elems, types.StringValue(v))
	}
	return types.ListValueMust(types.StringType, elems)
}

// TestReconcileDesiredSkills is the agent twin of TestReconcileGoalIds. It pins
// the same drift-detection contract for desired_skills so a full out-of-band
// skill clear is surfaced instead of hidden. reconcileDesiredSkills runs ONLY on
// Read/Import (Create/Update set state=plan directly), so there is no create/
// apply convergence to break — but the null/unknown-preserving cases still matter
// to avoid a phantom null↔[] diff for an unmanaged desired_skills.
func TestReconcileDesiredSkills(t *testing.T) {
	ctx := context.Background()

	// server has skills → reflect them (base irrelevant).
	got, d := reconcileDesiredSkills(ctx, types.ListNull(types.StringType), []string{"s1"})
	if d.HasError() {
		t.Fatalf("unexpected diags: %+v", d)
	}
	if got.IsNull() || len(got.Elements()) != 1 {
		t.Errorf("server skills must be reflected, got %+v", got)
	}

	// server empty + base NULL (unmanaged / import) → preserve null.
	got, d = reconcileDesiredSkills(ctx, types.ListNull(types.StringType), nil)
	if d.HasError() {
		t.Fatalf("unexpected diags: %+v", d)
	}
	if !got.IsNull() {
		t.Errorf("server empty + base null must stay null, got %+v", got)
	}

	// server empty + base UNKNOWN → preserve unknown.
	got, _ = reconcileDesiredSkills(ctx, types.ListUnknown(types.StringType), nil)
	if !got.IsUnknown() {
		t.Errorf("server empty + base unknown must stay unknown, got %+v", got)
	}

	// server empty + base EMPTY known list → empty list (no churn).
	got, d = reconcileDesiredSkills(ctx, mkSkills(), nil)
	if d.HasError() {
		t.Fatalf("unexpected diags: %+v", d)
	}
	if got.IsNull() || len(got.Elements()) != 0 {
		t.Errorf("server empty + base empty-list must be empty list, got %+v", got)
	}

	// THE FIX: server empty + base POPULATED known list → EMPTY list so a full
	// out-of-band skill clear (state=[s1], server=[]) surfaces as drift.
	got, d = reconcileDesiredSkills(ctx, mkSkills("s1"), nil)
	if d.HasError() {
		t.Fatalf("unexpected diags: %+v", d)
	}
	if got.IsNull() || len(got.Elements()) != 0 {
		t.Errorf("out-of-band skill clear (state=[s1], server=[]) must reflect [] as drift, got %+v", got)
	}
}

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
	// reports_to null -> set：發出 JSON 字串（不是省略）。
	plan = state
	plan.ReportsTo = types.StringValue("boss-2")
	if in := buildAgentUpdateInput(plan, state); string(in.ReportsTo) != `"boss-2"` {
		t.Errorf("reports_to not carried as JSON string: %s", string(in.ReportsTo))
	}
}

// TestBuildAgentUpdateInput_ReportsToTriState pins the three reports_to cases
// that the omitempty *string field could not express before this fix:
//   - unchanged   → nil RawMessage (omitted from body → live value untouched)
//   - set → value → JSON string
//   - set → null  → JSON null (agent resets to root; live-proven 2026-07-22)
func TestBuildAgentUpdateInput_ReportsToTriState(t *testing.T) {
	// unchanged (both null)
	state := agentBaseModel()
	plan := state
	if in := buildAgentUpdateInput(plan, state); in.ReportsTo != nil {
		t.Errorf("unchanged null reports_to must omit: %s", string(in.ReportsTo))
	}

	// unchanged (both same value)
	state = agentBaseModel()
	state.ReportsTo = types.StringValue("boss-1")
	plan = state
	if in := buildAgentUpdateInput(plan, state); in.ReportsTo != nil {
		t.Errorf("unchanged value reports_to must omit: %s", string(in.ReportsTo))
	}

	// set (boss-1) → null: emit explicit JSON null so agent returns to root.
	state = agentBaseModel()
	state.ReportsTo = types.StringValue("boss-1")
	plan = state
	plan.ReportsTo = types.StringNull()
	if in := buildAgentUpdateInput(plan, state); string(in.ReportsTo) != "null" {
		t.Errorf("set→null reports_to must emit JSON null, got %q", string(in.ReportsTo))
	}

	// changed value (boss-1) → (boss-2): emit new JSON string.
	state = agentBaseModel()
	state.ReportsTo = types.StringValue("boss-1")
	plan = state
	plan.ReportsTo = types.StringValue("boss-2")
	if in := buildAgentUpdateInput(plan, state); string(in.ReportsTo) != `"boss-2"` {
		t.Errorf("changed reports_to must emit new JSON string, got %q", string(in.ReportsTo))
	}
}

// TestBuildAdapterConfigPatch covers the adapterConfig clear-on-removal logic.
// live-proven (2026-07-22): null-under-merge clears scalars but NOT object keys
// like env; a computed full-config replace clears all managed keys uniformly
// while preserving every unmanaged key.
func TestBuildAdapterConfigPatch(t *testing.T) {
	unmanaged := func() map[string]any {
		return map[string]any{
			"model":                  "claude-sonnet-4-6",
			"chrome":                 true,
			"engine":                 "old-engine",
			"env":                    map[string]any{"GH_TOKEN": map[string]any{"secretId": "s1"}},
			"paperclipSkillSync":     map[string]any{"desiredSkills": []any{"paperclipai/paperclip/paperclip-board"}},
			"instructionsEntryFile":  "AGENTS.md",
			"instructionsBundleMode": "managed",
			"someUnknownFutureKey":   "keepme",
		}
	}

	assertUnmanagedIntact := func(t *testing.T, out map[string]any) {
		t.Helper()
		for _, k := range []string{"paperclipSkillSync", "instructionsEntryFile", "instructionsBundleMode", "someUnknownFutureKey"} {
			if _, ok := out[k]; !ok {
				t.Errorf("unmanaged key %q was dropped by clear — MUST be preserved", k)
			}
		}
	}

	t.Run("no clear → merge overlay, replace=false (working path unchanged)", func(t *testing.T) {
		current := unmanaged()
		planManaged := map[string]any{"model": "claude-opus-4-8", "chrome": true, "engine": "old-engine", "env": map[string]any{"GH_TOKEN": map[string]any{"secretId": "s1"}}}
		priorManaged := map[string]any{"model": "claude-sonnet-4-6", "chrome": true, "engine": "old-engine", "env": map[string]any{"GH_TOKEN": map[string]any{"secretId": "s1"}}}

		out, replace := buildAdapterConfigPatch(current, planManaged, priorManaged)
		if replace {
			t.Error("no cleared key → replace must stay false (do not disturb the working merge path)")
		}
		if out["model"] != "claude-opus-4-8" {
			t.Errorf("model not overlaid: %v", out["model"])
		}
		assertUnmanagedIntact(t, out)
	})

	t.Run("clear scalar chrome → key REMOVED, replace=true, unmanaged intact", func(t *testing.T) {
		current := unmanaged()
		// plan dropped chrome; kept model/engine/env
		planManaged := map[string]any{"model": "claude-sonnet-4-6", "engine": "old-engine", "env": map[string]any{"GH_TOKEN": map[string]any{"secretId": "s1"}}}
		priorManaged := map[string]any{"model": "claude-sonnet-4-6", "chrome": true, "engine": "old-engine", "env": map[string]any{"GH_TOKEN": map[string]any{"secretId": "s1"}}}

		out, replace := buildAdapterConfigPatch(current, planManaged, priorManaged)
		if !replace {
			t.Error("cleared key → replace must be true (null-under-merge cannot clear env; full-config replace clears uniformly)")
		}
		if _, ok := out["chrome"]; ok {
			t.Errorf("chrome must be REMOVED (not left as null): %v", out["chrome"])
		}
		if out["engine"] != "old-engine" || out["model"] != "claude-sonnet-4-6" {
			t.Errorf("surviving managed keys wrong: %+v", out)
		}
		assertUnmanagedIntact(t, out)
	})

	t.Run("clear object env → key REMOVED, replace=true, unmanaged intact", func(t *testing.T) {
		current := unmanaged()
		// plan dropped env; kept model/chrome/engine
		planManaged := map[string]any{"model": "claude-sonnet-4-6", "chrome": true, "engine": "old-engine"}
		priorManaged := map[string]any{"model": "claude-sonnet-4-6", "chrome": true, "engine": "old-engine", "env": map[string]any{"GH_TOKEN": map[string]any{"secretId": "s1"}}}

		out, replace := buildAdapterConfigPatch(current, planManaged, priorManaged)
		if !replace {
			t.Error("cleared env → replace must be true")
		}
		if _, ok := out["env"]; ok {
			t.Errorf("env must be REMOVED: %v", out["env"])
		}
		assertUnmanagedIntact(t, out)
	})

	t.Run("clear + change together: drop chrome, change model", func(t *testing.T) {
		current := unmanaged()
		planManaged := map[string]any{"model": "claude-opus-4-8", "engine": "old-engine", "env": map[string]any{"GH_TOKEN": map[string]any{"secretId": "s1"}}}
		priorManaged := map[string]any{"model": "claude-sonnet-4-6", "chrome": true, "engine": "old-engine", "env": map[string]any{"GH_TOKEN": map[string]any{"secretId": "s1"}}}

		out, replace := buildAdapterConfigPatch(current, planManaged, priorManaged)
		if !replace {
			t.Error("cleared chrome → replace must be true")
		}
		if _, ok := out["chrome"]; ok {
			t.Error("chrome must be removed")
		}
		if out["model"] != "claude-opus-4-8" {
			t.Errorf("model not overlaid alongside clear: %v", out["model"])
		}
		assertUnmanagedIntact(t, out)
	})

	t.Run("does not mutate the current bag", func(t *testing.T) {
		current := unmanaged()
		planManaged := map[string]any{"model": "claude-sonnet-4-6"}
		priorManaged := map[string]any{"model": "claude-sonnet-4-6", "chrome": true}
		_, _ = buildAdapterConfigPatch(current, planManaged, priorManaged)
		if _, ok := current["chrome"]; !ok {
			t.Error("buildAdapterConfigPatch mutated the caller's current bag (removed chrome)")
		}
	})
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
