// internal/provider/project_resource.go
// paperclip_project resource: declarative CRUD + import over the
// /api/companies/{cid}/projects + /api/projects/{id} endpoints. This turns the
// idempotent project-etl.sh (repo → project) into a declarative resource.
//
// A project binds a primary git_repo workspace (inline at create) and optionally
// references a lead agent (lead_agent_id) and a set of goals (goal_ids). Two
// removal=clear behaviors are load-bearing (repo-owner mandated, live-proven
// 2026-07-22) and R5 (the initial data import) depends on them being exact:
//   - lead_agent_id removed from config → explicit JSON null clears the lead.
//   - goal_ids emptied/removed → explicit empty array [] clears ALL goal links
//     (the API mirrors goalIds[0] into the legacy read-only goalId; we WRITE
//     goalIds, never goalId).
//
// workspace is create-time only: the API silently ignores an inline workspace in
// a PATCH (live-proven), so workspace fields are RequiresReplace — changing the
// repo recreates the project (a project IS its repo, matching the ETL's identity).
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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

// workspaceAttrTypes describes the nested `workspace` object. Only the three
// managed fields are modeled; the API's many other workspace fields (cwd,
// repoRef, visibility, …) are left untouched.
var workspaceAttrTypes = map[string]attr.Type{
	"source_type": types.StringType,
	"repo_url":    types.StringType,
	"is_primary":  types.BoolType,
}

type projectResource struct {
	client *client.Client
}

func NewProjectResource() resource.Resource { return &projectResource{} }

type projectResourceModel struct {
	ID          types.String `tfsdk:"id"`
	CompanyID   types.String `tfsdk:"company_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Status      types.String `tfsdk:"status"`
	LeadAgentId types.String `tfsdk:"lead_agent_id"`
	GoalIds     types.Set    `tfsdk:"goal_ids"`
	Workspace   types.Object `tfsdk:"workspace"`
}

// workspaceModel decodes the nested `workspace` object.
type workspaceModel struct {
	SourceType types.String `tfsdk:"source_type"`
	RepoUrl    types.String `tfsdk:"repo_url"`
	IsPrimary  types.Bool   `tfsdk:"is_primary"`
}

func (r *projectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *projectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A paperclip project. Binds a primary git_repo workspace (inline at create) and " +
			"optionally references a lead agent (lead_agent_id) and goals (goal_ids). Removing lead_agent_id " +
			"clears the lead (explicit null); emptying goal_ids clears all goal links (explicit empty array) — " +
			"neither no-ops. Changing the workspace recreates the project (workspace is create-time only).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"company_id": schema.StringAttribute{
				Required:      true,
				Description:   "父 company 的 id。project 不能跨 company 搬動，改這個欄位會整個重建。",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{Required: true},
			"description": schema.StringAttribute{
				Optional: true,
			},
			"status": schema.StringAttribute{
				Optional: true,
				Computed: true,
				// UseStateForUnknown：Optional+Computed 若不加這個，任何一次 update 都會讓未在 config
				// 指定的 status 在 plan 變成 unknown，buildProjectUpdateInput 就會送出空字串 ""（live
				// 實證 API 回 400 invalid_enum：backlog/planned/in_progress/completed/cancelled）。加上
				// 它後，未指定時保留既有 state 值、不觸發假 update。
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Description: "專案狀態，enum：backlog/planned/in_progress/completed/cancelled。省略時由 API 指定" +
					"預設並落地成 state（Optional+Computed，避免 null↔server 值的假差異）。",
			},
			"lead_agent_id": schema.StringAttribute{
				Optional: true,
				Description: "帶領這個 project 的 agent id（參照另一個 paperclip_agent.id）。留空（null）表示" +
					"沒有指定 lead。把已設定的 lead_agent_id 從 config 移除會送出 JSON null，實際清空" +
					"（live 實證 2026-07-22），無須重建。",
			},
			"goal_ids": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "這個 project 連結的 goal id 集合（參照 paperclip_goal.id）。用 Set（無序）而非 List：" +
					"live 探測 API 不保證保留 goalIds 的輸入順序（它有自己的正規順序），用 List 會每次 apply 觸發" +
					"順序不一致錯誤。寫入 API 的 goalIds（array）；API 會把 goalIds[0] 鏡射成 legacy 的唯讀 goalId，" +
					"本 provider 不寫 goalId。把集合清空（[]）或整個移除會送出空 array，實際清空所有連結" +
					"（live 實證 2026-07-22），不是靜默 no-op。",
			},
			"workspace": schema.SingleNestedAttribute{
				Optional: true,
				Description: "主要 workspace（primary git_repo）。只能在建立時 inline 帶入——live 探測：PATCH 內的" +
					"inline workspace 被 API 靜默忽略，所以這裡任何變更都會觸發整個 project 重建（RequiresReplace）。" +
					"v1 只管 primary workspace，非-primary 不在範圍內。",
				PlanModifiers: []planmodifier.Object{objectplanmodifier.RequiresReplace()},
				Attributes: map[string]schema.Attribute{
					"source_type": schema.StringAttribute{
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString("git_repo"),
						Description: `workspace 來源類型。v1 固定 "git_repo"（省略時的預設）。`,
					},
					"repo_url": schema.StringAttribute{
						Required:    true,
						Description: "git repo URL，例如 https://github.com/org/repo。",
					},
					"is_primary": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(true),
						Description: "是否為 primary workspace。v1 固定 true（省略時的預設）。",
					},
				},
			},
		},
	}
}

func (r *projectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *projectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := client.ProjectCreateInput{Name: plan.Name.ValueString()}
	if !plan.Description.IsNull() {
		in.Description = plan.Description.ValueString()
	}
	if !plan.Status.IsNull() && !plan.Status.IsUnknown() {
		in.Status = plan.Status.ValueString()
	}
	if !plan.LeadAgentId.IsNull() {
		in.LeadAgentId = plan.LeadAgentId.ValueString()
	}
	if !plan.GoalIds.IsNull() && !plan.GoalIds.IsUnknown() {
		ids, d := goalIdsToSlice(plan.GoalIds)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		in.GoalIds = ids
	}
	ws, diags := buildWorkspaceCreateInput(ctx, plan.Workspace)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	in.Workspace = ws

	got, err := r.client.CreateProject(ctx, plan.CompanyID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Create project failed", err.Error())
		return
	}
	model, diags := projectFromAPI(ctx, got, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *projectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.client.GetProject(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsGone(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read project failed", err.Error())
		return
	}
	model, diags := projectFromAPI(ctx, got, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *projectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state projectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// 只送有變的欄位（scalar 指標、lead_agent_id 三態、goal_ids 清空送 []）→ 保留未管欄位。
	// workspace 是 RequiresReplace，永遠不會走到這裡（有變就重建），所以 update input 沒有它。
	in, diags := buildProjectUpdateInput(plan, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !projectUpdateInputEmpty(in) {
		if _, err := r.client.UpdateProject(ctx, state.ID.ValueString(), in); err != nil {
			resp.Diagnostics.AddError("Update project failed", err.Error())
			return
		}
	}

	// 讀回權威狀態：PATCH 回應是否含 primaryWorkspace 不保證，統一用 GET 取完整 project
	// （沿用整個 repo「改完讀回驗證」的信念）。
	got, err := r.client.GetProject(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Update project failed (reading back)", err.Error())
		return
	}
	model, diags := projectFromAPI(ctx, got, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *projectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteProject(ctx, state.ID.ValueString()); err != nil && !client.IsGone(err) {
		resp.Diagnostics.AddError("Delete project failed", err.Error())
	}
}

// ImportState 用單純的 passthrough id：live 探測 GET /api/projects/{id} 可獨立運作
// （不像 secret 需要 company_id 複合鍵），company_id/workspace/goal_ids 由 Read 從 API 回填。
func (r *projectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// goalIdsToSlice converts a types.Set of strings to []string. A null/unknown
// set yields an EMPTY (non-nil) slice so a cleared goal_ids sends [] (clearing
// all links) rather than being omitted. Set iteration order is not meaningful:
// the API reorders goalIds into its own canonical order anyway (which is why the
// attribute is a Set, not a List), so send-order does not matter.
//
// The non-String else-branch is defensive: goal_ids' ElementType is StringType,
// so the framework guarantees every element is a types.String and this branch is
// unreachable via the public Set API. It surfaces a diagnostic rather than
// silently dropping an id — a dropped id would mis-set or wrongly clear a link.
func goalIdsToSlice(set types.Set) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if set.IsNull() || set.IsUnknown() {
		return []string{}, diags
	}
	elems := set.Elements()
	out := make([]string, 0, len(elems))
	for _, e := range elems {
		s, ok := e.(types.String)
		if !ok {
			diags.AddError(
				"Unexpected goal_ids element type",
				fmt.Sprintf("goal_ids element has type %T, expected string; refusing to silently drop it.", e),
			)
			continue
		}
		out = append(out, s.ValueString())
	}
	return out, diags
}

// buildWorkspaceCreateInput turns the nested `workspace` object into the inline
// create body. Returns nil when no workspace block is declared (a project may
// have none). source_type/is_primary carry their schema defaults (git_repo/true)
// so they are always known here; workspace name is omitted (server derives it).
func buildWorkspaceCreateInput(ctx context.Context, obj types.Object) (*client.WorkspaceCreateInput, diag.Diagnostics) {
	var diags diag.Diagnostics
	if obj.IsNull() || obj.IsUnknown() {
		return nil, diags
	}
	var wm workspaceModel
	diags.Append(obj.As(ctx, &wm, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil, diags
	}
	sourceType := "git_repo"
	if !wm.SourceType.IsNull() && !wm.SourceType.IsUnknown() {
		sourceType = wm.SourceType.ValueString()
	}
	isPrimary := true
	if !wm.IsPrimary.IsNull() && !wm.IsPrimary.IsUnknown() {
		isPrimary = wm.IsPrimary.ValueBool()
	}
	return &client.WorkspaceCreateInput{
		SourceType: sourceType,
		RepoUrl:    wm.RepoUrl.ValueString(),
		IsPrimary:  isPrimary,
	}, diags
}

// buildProjectUpdateInput returns an update input carrying ONLY the changed
// fields (spec §6.3). lead_agent_id uses the tri-state RawMessage encoding (mirrors
// goal/agent refs) and goal_ids uses a *[]string so an emptied/removed list sends
// an explicit [] and actually clears the links, per the removal=clear policy.
func buildProjectUpdateInput(plan, state projectResourceModel) (client.ProjectUpdateInput, diag.Diagnostics) {
	var in client.ProjectUpdateInput
	var diags diag.Diagnostics
	if !plan.Name.Equal(state.Name) {
		v := plan.Name.ValueString()
		in.Name = &v
	}
	if !plan.Description.Equal(state.Description) {
		v := plan.Description.ValueString()
		in.Description = &v
	}
	if !plan.Status.Equal(state.Status) {
		v := plan.Status.ValueString()
		in.Status = &v
	}
	// lead_agent_id 三態：未變→不送(保留)；改成值→JSON 字串；由值改回 null→JSON null(清空)。
	if !plan.LeadAgentId.Equal(state.LeadAgentId) {
		if plan.LeadAgentId.IsNull() {
			in.LeadAgentId = json.RawMessage("null")
		} else {
			b, _ := json.Marshal(plan.LeadAgentId.ValueString())
			in.LeadAgentId = json.RawMessage(b)
		}
	}
	// goal_ids：有變就送（null/[]→[] 清空；[a,b]→[a,b]）；沒變→nil pointer(不送，保留)。
	if !plan.GoalIds.Equal(state.GoalIds) {
		ids, d := goalIdsToSlice(plan.GoalIds)
		diags.Append(d...)
		in.GoalIds = &ids
	}
	return in, diags
}

func projectUpdateInputEmpty(in client.ProjectUpdateInput) bool {
	return in.Name == nil && in.Description == nil && in.Status == nil &&
		in.LeadAgentId == nil && in.GoalIds == nil
}

// projectFromAPI maps a Create/Get/Update response onto resource state. goal_ids
// is reconciled against `base` so a server with no goals doesn't churn a null↔[]
// diff (mirrors agent's desired_skills). workspace reflects the primary workspace.
func projectFromAPI(ctx context.Context, got *client.Project, base projectResourceModel) (projectResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	m := projectResourceModel{
		ID:          types.StringValue(got.ID),
		CompanyID:   types.StringValue(got.CompanyID),
		Name:        types.StringValue(got.Name),
		Description: optionalString(got.Description),
		Status:      optionalString(got.Status),
		LeadAgentId: optionalString(got.LeadAgentId),
	}

	goalIds, d := reconcileGoalIds(ctx, base.GoalIds, got.GoalIds)
	diags.Append(d...)
	m.GoalIds = goalIds

	ws, d := workspaceFromAPI(got.PrimaryWorkspace)
	diags.Append(d...)
	m.Workspace = ws

	return m, diags
}

// reconcileGoalIds maps the server's goal ids onto state so drift is detectable
// without churning a phantom diff. The result is a Set, so the server's canonical
// ordering never shows up as a diff. When the server reports NO goals, `base`
// disambiguates two cases (base = plan on Create/Update, state on Read/Import):
//
//   - base null/unknown → return base. A null base means the attribute is absent
//     (config declares no goal_ids) or this is a fresh/import read; reflecting the
//     server's [] would oscillate against that null into a phantom null↔[] diff.
//   - base is a populated known set → return an EMPTY set. state had goals but the
//     server now has none: a full out-of-band clear. Returning base here would
//     hide it (the original blind spot); reflecting [] surfaces the drift so the
//     next plan restores the declared links.
//
// This does NOT reintroduce create/apply non-convergence: the API returns the
// just-written goalIds in its Create response and in the Update read-back GET
// (live-proven 2026-07-22), so serverIds is non-empty whenever goals were
// declared — the populated-base branch is only reached by a genuine Read-time
// clear. A base that is an EMPTY known set (explicit `goal_ids = []`) also lands
// in the empty-set branch, which matches it exactly (no churn).
func reconcileGoalIds(ctx context.Context, base types.Set, serverIds []string) (types.Set, diag.Diagnostics) {
	if len(serverIds) == 0 {
		if base.IsNull() || base.IsUnknown() {
			return base, nil
		}
		return types.SetValueFrom(ctx, types.StringType, []string{})
	}
	return types.SetValueFrom(ctx, types.StringType, serverIds)
}

// workspaceFromAPI rebuilds the nested `workspace` object from the primary
// workspace, surfacing ONLY the three managed fields. A project without a primary
// workspace maps to a null object (matching a config with no workspace block).
func workspaceFromAPI(ws *client.Workspace) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics
	if ws == nil {
		return types.ObjectNull(workspaceAttrTypes), diags
	}
	obj, d := types.ObjectValue(workspaceAttrTypes, map[string]attr.Value{
		"source_type": types.StringValue(ws.SourceType),
		"repo_url":    types.StringValue(ws.RepoUrl),
		"is_primary":  types.BoolValue(ws.IsPrimary),
	})
	diags.Append(d...)
	return obj, diags
}

var (
	_ resource.Resource                = &projectResource{}
	_ resource.ResourceWithConfigure   = &projectResource{}
	_ resource.ResourceWithImportState = &projectResource{}
)
