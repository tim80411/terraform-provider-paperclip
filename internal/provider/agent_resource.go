// internal/provider/agent_resource.go
// paperclip_agent resource: declarative CRUD + import over the
// /api/companies/{cid}/agents + /api/agents/{id}(+/skills/sync) endpoints.
//
// The load-bearing concern here is adapterConfig: an OPAQUE bag where paperclip
// itself stores paperclipSkillSync + instructions* + unknown keys alongside the
// four keys this provider manages (model/engine/chrome/env). Update must GET the
// current bag, overlay only the managed keys (client.MergeAdapterConfig), and
// PATCH with replaceAdapterConfig=false so nothing paperclip owns is wiped.
package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

// defaultAdapterType is the only adapterType paperclip uses for these agents
// (live 探測：CEO + engineers all "claude_local"). It is not a user-facing
// attribute (not in the task's attr list) — hardcoded here.
const defaultAdapterType = "claude_local"

// adapterAttrTypes / envSecretRefAttrTypes describe the nested `adapter` object
// and the per-env-var secret reference the TF side models minimally as {secret_id}.
var envSecretRefAttrTypes = map[string]attr.Type{
	"secret_id": types.StringType,
}

var adapterAttrTypes = map[string]attr.Type{
	"model":  types.StringType,
	"engine": types.StringType,
	"chrome": types.BoolType,
	"env":    types.MapType{ElemType: types.ObjectType{AttrTypes: envSecretRefAttrTypes}},
}

type agentResource struct {
	client *client.Client
}

func NewAgentResource() resource.Resource { return &agentResource{} }

type agentResourceModel struct {
	ID            types.String `tfsdk:"id"`
	CompanyID     types.String `tfsdk:"company_id"`
	Name          types.String `tfsdk:"name"`
	Role          types.String `tfsdk:"role"`
	Title         types.String `tfsdk:"title"`
	Icon          types.String `tfsdk:"icon"`
	Capabilities  types.String `tfsdk:"capabilities"`
	ReportsTo     types.String `tfsdk:"reports_to"`
	DesiredSkills types.List   `tfsdk:"desired_skills"`
	Adapter       types.Object `tfsdk:"adapter"`
}

// adapterModel decodes the nested `adapter` object.
type adapterModel struct {
	Model  types.String `tfsdk:"model"`
	Engine types.String `tfsdk:"engine"`
	Chrome types.Bool   `tfsdk:"chrome"`
	Env    types.Map    `tfsdk:"env"`
}

type envSecretRefModel struct {
	SecretID types.String `tfsdk:"secret_id"`
}

func (r *agentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agent"
}

func (r *agentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A paperclip agent within a company. Manages identity (name/role/title/icon/" +
			"capabilities), chain-of-command (reports_to), the adapter config (model/engine/chrome/env), and " +
			"desired skills. The adapter config is merged server-side so paperclip-owned keys " +
			"(skill sync, instruction pointers) survive updates.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"company_id": schema.StringAttribute{
				Required:      true,
				Description:   "父 company 的 id。agent 不能跨 company 搬動，改這個欄位會整個重建。",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{Required: true},
			"role": schema.StringAttribute{
				Optional:    true,
				Description: `agent 角色，例如 "ceo" / "engineer" / "general"。`,
			},
			"title": schema.StringAttribute{Optional: true},
			"icon": schema.StringAttribute{
				Optional: true,
				Description: "圖示。live 探測：這是受限的 enum（bot|cpu|brain|zap|rocket|code|terminal|shield|" +
					"eye|search|wrench|hammer|lightbulb|sparkles|star|heart|flame|bug|cog|database|globe|lock|" +
					"mail|message-square|file-code|git-branch|package|puzzle|target|wand|atom|circuit-board|" +
					"radar|swords|telescope|microscope|crown|gem|hexagon|pentagon|fingerprint）。非法值由 API 擋下。",
			},
			"capabilities": schema.StringAttribute{
				Optional:    true,
				Description: "agent 能力描述。live 探測：這是「單一字串」（不是清單）——一段說明文字。",
			},
			"reports_to": schema.StringAttribute{
				Optional: true,
				Description: "上級 agent 的 id（參照另一個 paperclip_agent.id）。根 agent 留空（null）。" +
					"Terraform 的依賴圖會自動確保上級先建立。把已設定的 reports_to 從 config 移除（改回 null）" +
					"會送出 JSON null，讓 agent 重新變回根 agent（live 實證 2026-07-22），無須重建。",
			},
			"desired_skills": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "要掛給 agent 的 skill 參照清單（例如 \"paperclipai/paperclip/paperclip-board\"）。" +
					"Create/Update 後透過 POST /api/agents/{id}/skills/sync 套用；寫入 " +
					"adapterConfig.paperclipSkillSync.desiredSkills。skill 必須已存在於 company（未知參照 API 回 422）。",
			},
			"adapter": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "adapter 設定。只有這裡列出的 key 由 provider 管理；paperclip 自己維護的 key（skill 同步、指令檔路徑等）在 Update 時會被保留。",
				Attributes: map[string]schema.Attribute{
					"model":  schema.StringAttribute{Required: true, Description: `模型 id，例如 "claude-opus-4-8"。`},
					"engine": schema.StringAttribute{Optional: true},
					"chrome": schema.BoolAttribute{Optional: true, Description: "是否啟用 chrome 工具。"},
					"env": schema.MapNestedAttribute{
						Optional:    true,
						Description: "環境變數注入，key 是環境變數名，value 參照一個 secret。序列化成 live secret_ref 形狀。",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"secret_id": schema.StringAttribute{
									Required:    true,
									Description: "被參照的 paperclip_company_secret.id。",
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *agentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected ProviderData", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *agentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan agentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managed, diags := buildManagedAdapterConfig(ctx, plan.Adapter)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := client.AgentCreateInput{
		Name:          plan.Name.ValueString(),
		AdapterType:   defaultAdapterType,
		AdapterConfig: managed,
	}
	if !plan.Role.IsNull() {
		in.Role = plan.Role.ValueString()
	}
	if !plan.Title.IsNull() {
		in.Title = plan.Title.ValueString()
	}
	if !plan.Icon.IsNull() {
		in.Icon = plan.Icon.ValueString()
	}
	if !plan.Capabilities.IsNull() {
		in.Capabilities = plan.Capabilities.ValueString()
	}
	if !plan.ReportsTo.IsNull() {
		v := plan.ReportsTo.ValueString()
		in.ReportsTo = &v
	}

	got, err := r.client.CreateAgent(ctx, plan.CompanyID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Create agent failed", err.Error())
		return
	}

	// desired_skills 一律走 skills/sync（Create 與 Update 統一路徑）。
	if diags := r.syncSkills(ctx, got.ID, plan.DesiredSkills); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	// state = plan + computed id：我們送出的欄位都會被 server 原樣回存，無須讀回覆寫
	// （沿用 company/secret 骨架：只有 id 是 computed，其餘信任 plan，避免 apply 後不一致）。
	state := plan
	state.ID = types.StringValue(got.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *agentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state agentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.client.GetAgent(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsGone(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read agent failed", err.Error())
		return
	}
	model, diags := agentFromAPI(ctx, got, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *agentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state agentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := buildAgentUpdateInput(plan, state)

	// adapter 有變 → GET 現有 adapterConfig，疊上 plan 管理的 key（保留未管 key），
	// 並處理「從 config 移除某管理 key」的清除（見 buildAdapterConfigPatch）。
	if !plan.Adapter.Equal(state.Adapter) {
		current, err := r.client.GetAgent(ctx, state.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Update agent failed (reading current adapterConfig)", err.Error())
			return
		}
		planManaged, diags := buildManagedAdapterConfig(ctx, plan.Adapter)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		priorManaged, diags := buildManagedAdapterConfig(ctx, state.Adapter)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		cfg, replace := buildAdapterConfigPatch(current.AdapterConfig, planManaged, priorManaged)
		in.AdapterConfig = cfg
		in.ReplaceAdapterConfig = &replace
	}

	if !agentUpdateInputEmpty(in) {
		if _, err := r.client.UpdateAgent(ctx, state.ID.ValueString(), in); err != nil {
			resp.Diagnostics.AddError("Update agent failed", err.Error())
			return
		}
	}

	if !plan.DesiredSkills.Equal(state.DesiredSkills) {
		if diags := r.syncSkills(ctx, state.ID.ValueString(), plan.DesiredSkills); diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	}

	// state = plan + 既有 id（沿用骨架：信任 plan，避免 apply 後不一致）。
	newState := plan
	newState.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *agentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state agentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteAgent(ctx, state.ID.ValueString()); err != nil && !client.IsGone(err) {
		resp.Diagnostics.AddError("Delete agent failed", err.Error())
	}
}

// ImportState 用單純的 passthrough id：live 探測 GET /api/agents/{id} 可獨立運作
// （不像 secret 需要 company_id 複合鍵），company_id 由 Read 從 API 回填。
func (r *agentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// syncSkills applies desired_skills via skills/sync. A null list means "not
// managed here" → skip entirely (don't clobber whatever paperclip has); an
// explicit (possibly empty) list is applied as-is.
func (r *agentResource) syncSkills(ctx context.Context, id string, list types.List) diag.Diagnostics {
	var diags diag.Diagnostics
	if list.IsNull() || list.IsUnknown() {
		return diags
	}
	var skills []string
	diags.Append(list.ElementsAs(ctx, &skills, false)...)
	if diags.HasError() {
		return diags
	}
	if err := r.client.SyncAgentSkills(ctx, id, skills); err != nil {
		diags.AddError("Sync agent skills failed", err.Error())
	}
	return diags
}

// secretRef renders a TF `{ secret_id }` into the live secret_ref shape the API
// stores. Sending all five fields (not just secretId) keeps read-back drift-free.
func secretRef(id string) map[string]any {
	return map[string]any{
		"type":                   "secret_ref",
		"version":                "latest",
		"secretId":               id,
		"projectionClass":        "unclassified",
		"projectionAllowlistKey": nil,
	}
}

// buildManagedAdapterConfig turns the nested `adapter` object into the
// provider-managed subset of adapterConfig ({model,engine,chrome,env}). Only
// keys the user actually set are included; the rest of the bag is preserved by
// MergeAdapterConfig at PATCH time (Update) or injected by paperclip (Create).
func buildManagedAdapterConfig(ctx context.Context, adapterObj types.Object) (map[string]any, diag.Diagnostics) {
	var diags diag.Diagnostics
	managed := map[string]any{}
	if adapterObj.IsNull() || adapterObj.IsUnknown() {
		return managed, diags
	}
	var am adapterModel
	diags.Append(adapterObj.As(ctx, &am, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return managed, diags
	}
	if !am.Model.IsNull() {
		managed["model"] = am.Model.ValueString()
	}
	if !am.Engine.IsNull() {
		managed["engine"] = am.Engine.ValueString()
	}
	if !am.Chrome.IsNull() {
		managed["chrome"] = am.Chrome.ValueBool()
	}
	if !am.Env.IsNull() && !am.Env.IsUnknown() {
		var envMap map[string]envSecretRefModel
		diags.Append(am.Env.ElementsAs(ctx, &envMap, false)...)
		if diags.HasError() {
			return managed, diags
		}
		envOut := map[string]any{}
		for name, ref := range envMap {
			envOut[name] = secretRef(ref.SecretID.ValueString())
		}
		managed["env"] = envOut
	}
	return managed, diags
}

// buildAgentUpdateInput returns an update input carrying ONLY the changed scalar
// fields (spec §6.3). adapterConfig is handled separately in Update (it needs a
// live GET + MergeAdapterConfig), never here.
func buildAgentUpdateInput(plan, state agentResourceModel) client.AgentUpdateInput {
	var in client.AgentUpdateInput
	if !plan.Name.Equal(state.Name) {
		v := plan.Name.ValueString()
		in.Name = &v
	}
	if !plan.Role.Equal(state.Role) {
		v := plan.Role.ValueString()
		in.Role = &v
	}
	if !plan.Title.Equal(state.Title) {
		v := plan.Title.ValueString()
		in.Title = &v
	}
	if !plan.Icon.Equal(state.Icon) {
		v := plan.Icon.ValueString()
		in.Icon = &v
	}
	if !plan.Capabilities.Equal(state.Capabilities) {
		v := plan.Capabilities.ValueString()
		in.Capabilities = &v
	}
	// reports_to 三態（見 client.AgentUpdateInput.ReportsTo 說明）：
	//   未變 → 不送（保留現況）；改成值 → 送 JSON 字串；由值改回 null → 送 JSON null（回到根）。
	if !plan.ReportsTo.Equal(state.ReportsTo) {
		if plan.ReportsTo.IsNull() {
			in.ReportsTo = json.RawMessage("null")
		} else {
			b, _ := json.Marshal(plan.ReportsTo.ValueString())
			in.ReportsTo = json.RawMessage(b)
		}
	}
	return in
}

// buildAdapterConfigPatch computes the adapterConfig payload + replaceAdapterConfig
// flag for an Update where the `adapter` block changed. It answers two needs at once:
//
//   - ADD/CHANGE a managed key (model/engine/chrome/env): overlay the plan's managed
//     keys onto the live bag via MergeAdapterConfig (preserving every unmanaged key)
//     and PATCH with replaceAdapterConfig=false — the pre-existing, working path.
//   - CLEAR a managed key (present in PRIOR STATE's adapter, dropped from the plan):
//     REMOVE it from the computed bag and switch to replaceAdapterConfig=true.
//
// Why the split: live probe (2026-07-22) proved a partial merge (replaceAdapterConfig
// =false) with a `null` value clears SCALAR keys (chrome/engine) but is SILENTLY
// IGNORED for the object key `env` — the server deep-merges and treats a null RHS as
// "no change" for object values. A full-config replace (built from the live bag minus
// the cleared keys) removes ALL managed keys uniformly while preserving the unmanaged
// ones (paperclipSkillSync, instructions*, unknown keys), and is idempotent.
//
// `current` is the freshly-GET'd live bag; it is never mutated (MergeAdapterConfig
// returns a fresh copy and we only delete from that copy). Only keys the provider
// manages are ever removed — priorManaged/planManaged come from buildManagedAdapterConfig,
// which never surfaces a server-owned key.
func buildAdapterConfigPatch(current, planManaged, priorManaged map[string]any) (map[string]any, bool) {
	merged := client.MergeAdapterConfig(current, planManaged)
	replace := false
	for k := range priorManaged {
		if _, stillManaged := planManaged[k]; !stillManaged {
			delete(merged, k)
			replace = true
		}
	}
	return merged, replace
}

func agentUpdateInputEmpty(in client.AgentUpdateInput) bool {
	return in.Name == nil && in.Role == nil && in.Title == nil && in.Icon == nil &&
		in.Capabilities == nil && in.ReportsTo == nil && in.AdapterConfig == nil
}

// agentFromAPI maps a GET response onto resource state (Read + Import). It
// reflects server truth for every attribute so drift is detectable and import
// round-trips; desired_skills is reconciled against `base` so a server with no
// skills doesn't churn a null↔[] diff.
func agentFromAPI(ctx context.Context, got *client.Agent, base agentResourceModel) (agentResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	m := agentResourceModel{
		ID:        types.StringValue(got.ID),
		CompanyID: types.StringValue(got.CompanyID),
		Name:      types.StringValue(got.Name),
		Role:      optionalString(got.Role),
		Title:     optionalString(got.Title),
		Icon:      optionalString(got.Icon),
	}
	m.Capabilities = optionalString(got.Capabilities)
	m.ReportsTo = optionalString(got.ReportsTo)

	adapterObj, d := adapterFromConfig(got.AdapterConfig)
	diags.Append(d...)
	m.Adapter = adapterObj

	skills := extractDesiredSkills(got.AdapterConfig)
	desired, d := reconcileDesiredSkills(ctx, base.DesiredSkills, skills)
	diags.Append(d...)
	m.DesiredSkills = desired

	return m, diags
}

// optionalString maps an omitempty API string to null (when empty) or a value.
func optionalString(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// adapterFromConfig rebuilds the nested `adapter` object from adapterConfig,
// surfacing ONLY the four managed keys. If none are present the object is null
// (matching a config with no adapter block).
func adapterFromConfig(ac map[string]any) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics
	nullObj := types.ObjectNull(adapterAttrTypes)
	if ac == nil {
		return nullObj, diags
	}
	model, hasModel := ac["model"].(string)
	engine, hasEngine := ac["engine"].(string)
	chrome, hasChrome := ac["chrome"].(bool)
	rawEnv, hasEnv := ac["env"].(map[string]any)
	if !hasModel && !hasEngine && !hasChrome && !hasEnv {
		return nullObj, diags
	}

	attrs := map[string]attr.Value{
		"model":  types.StringNull(),
		"engine": types.StringNull(),
		"chrome": types.BoolNull(),
		"env":    types.MapNull(types.ObjectType{AttrTypes: envSecretRefAttrTypes}),
	}
	if hasModel {
		attrs["model"] = types.StringValue(model)
	}
	if hasEngine {
		attrs["engine"] = types.StringValue(engine)
	}
	if hasChrome {
		attrs["chrome"] = types.BoolValue(chrome)
	}
	if hasEnv {
		envAttrs := map[string]attr.Value{}
		for name, v := range rawEnv {
			ref, _ := v.(map[string]any)
			secretID, _ := ref["secretId"].(string)
			obj, d := types.ObjectValue(envSecretRefAttrTypes, map[string]attr.Value{
				"secret_id": types.StringValue(secretID),
			})
			diags.Append(d...)
			envAttrs[name] = obj
		}
		envMap, d := types.MapValue(types.ObjectType{AttrTypes: envSecretRefAttrTypes}, envAttrs)
		diags.Append(d...)
		attrs["env"] = envMap
	}

	obj, d := types.ObjectValue(adapterAttrTypes, attrs)
	diags.Append(d...)
	return obj, diags
}

// extractDesiredSkills pulls adapterConfig.paperclipSkillSync.desiredSkills.
func extractDesiredSkills(ac map[string]any) []string {
	if ac == nil {
		return nil
	}
	sync, ok := ac["paperclipSkillSync"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := sync["desiredSkills"].([]any)
	if !ok {
		return nil
	}
	skills := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			skills = append(skills, s)
		}
	}
	return skills
}

// reconcileDesiredSkills reflects server skills when present; when the server
// has none it preserves `base` so a null (unmanaged) and an empty list don't
// oscillate into a phantom diff.
func reconcileDesiredSkills(ctx context.Context, base types.List, serverSkills []string) (types.List, diag.Diagnostics) {
	if len(serverSkills) == 0 {
		return base, nil
	}
	return types.ListValueFrom(ctx, types.StringType, serverSkills)
}

var (
	_ resource.Resource                = &agentResource{}
	_ resource.ResourceWithConfigure   = &agentResource{}
	_ resource.ResourceWithImportState = &agentResource{}
)
