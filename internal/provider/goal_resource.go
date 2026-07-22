// internal/provider/goal_resource.go
// paperclip_goal resource: declarative CRUD + import over the
// /api/companies/{cid}/goals + /api/goals/{id} endpoints.
//
// A goal is a node in the company's OKR tree: it can reference an owner agent
// (owner_agent_id) and a parent goal (parent_id, self-referential). Both refs
// are nullable and — per the repo-owner-mandated removal=clear policy already
// established for agent.reports_to — removing either from config must CLEAR
// the live value (explicit JSON null), not silently no-op into a perpetual
// plan diff. live 探測（2026-07-22）confirms explicit null cleanly clears both.
package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

type goalResource struct {
	client *client.Client
}

func NewGoalResource() resource.Resource { return &goalResource{} }

type goalResourceModel struct {
	ID           types.String `tfsdk:"id"`
	CompanyID    types.String `tfsdk:"company_id"`
	Title        types.String `tfsdk:"title"`
	Description  types.String `tfsdk:"description"`
	Level        types.String `tfsdk:"level"`
	Status       types.String `tfsdk:"status"`
	OwnerAgentId types.String `tfsdk:"owner_agent_id"`
	ParentId     types.String `tfsdk:"parent_id"`
}

func (r *goalResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_goal"
}

func (r *goalResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A node in a company's OKR tree. Optionally owned by an agent " +
			"(owner_agent_id) and optionally nested under another goal (parent_id, self-referential). " +
			"Removing either reference from config clears it live (explicit null), it does not no-op.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"company_id": schema.StringAttribute{
				Required:      true,
				Description:   "父 company 的 id。goal 不能跨 company 搬動，改這個欄位會整個重建。",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"title": schema.StringAttribute{Required: true},
			"description": schema.StringAttribute{
				Optional: true,
			},
			"level": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: `目標層級。live 探測 enum：company/team/agent/task。省略時 API 預設 "task"，` +
					"落地成 state 後即固定，之後只能靠明確改 config 值來變更（Terraform Optional+Computed 慣例）。",
			},
			"status": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: `目標狀態。live 探測 enum：planned/active/achieved/cancelled。省略時 API 預設 ` +
					`"planned"，落地成 state 後即固定。`,
			},
			"owner_agent_id": schema.StringAttribute{
				Optional: true,
				Description: "負責這個 goal 的 agent id（參照另一個 paperclip_agent.id）。留空（null）表示" +
					"沒有指定 owner。把已設定的 owner_agent_id 從 config 移除會送出 JSON null，實際清空" +
					"（live 實證 2026-07-22），無須重建。",
			},
			"parent_id": schema.StringAttribute{
				Optional: true,
				Description: "上層 goal 的 id（自我參照另一個 paperclip_goal.id）。Terraform 的依賴圖會" +
					"自動確保 parent 先建立，不需要額外的排序程式碼。把已設定的 parent_id 從 config 移除會" +
					"送出 JSON null，goal 回到頂層（live 實證 2026-07-22），無須重建。",
			},
		},
	}
}

func (r *goalResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *goalResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan goalResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := client.GoalCreateInput{Title: plan.Title.ValueString()}
	if !plan.Description.IsNull() {
		in.Description = plan.Description.ValueString()
	}
	if !plan.Level.IsNull() {
		in.Level = plan.Level.ValueString()
	}
	if !plan.Status.IsNull() {
		in.Status = plan.Status.ValueString()
	}
	if !plan.OwnerAgentId.IsNull() {
		in.OwnerAgentId = plan.OwnerAgentId.ValueString()
	}
	if !plan.ParentId.IsNull() {
		in.ParentId = plan.ParentId.ValueString()
	}

	got, err := r.client.CreateGoal(ctx, plan.CompanyID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Create goal failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, goalFromAPI(got))...)
}

func (r *goalResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state goalResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.client.GetGoal(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsGone(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read goal failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, goalFromAPI(got))...)
}

func (r *goalResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state goalResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// 只送有變的欄位（指標 + owner_agent_id/parent_id 的三態 RawMessage）→ 保留未管欄位（spec §6.3）。
	in := buildGoalUpdateInput(plan, state)
	if goalUpdateInputEmpty(in) {
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	}

	got, err := r.client.UpdateGoal(ctx, state.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Update goal failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, goalFromAPI(got))...)
}

func (r *goalResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state goalResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteGoal(ctx, state.ID.ValueString()); err != nil && !client.IsGone(err) {
		resp.Diagnostics.AddError("Delete goal failed", err.Error())
	}
}

// ImportState 用單純的 passthrough id：live 探測 GET /api/goals/{id} 可獨立運作
// （不像 secret 需要 company_id 複合鍵），company_id 由 Read 從 API 回填。
func (r *goalResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// buildGoalUpdateInput returns an update input carrying ONLY the changed
// fields (spec §6.3). owner_agent_id/parent_id use the tri-state RawMessage
// encoding (goalRefRawMessage) so a ref changed from set→unset sends an
// explicit JSON null and actually clears it live, per the removal=clear policy.
func buildGoalUpdateInput(plan, state goalResourceModel) client.GoalUpdateInput {
	var in client.GoalUpdateInput
	if !plan.Title.Equal(state.Title) {
		v := plan.Title.ValueString()
		in.Title = &v
	}
	if !plan.Description.Equal(state.Description) {
		v := plan.Description.ValueString()
		in.Description = &v
	}
	if !plan.Level.Equal(state.Level) {
		v := plan.Level.ValueString()
		in.Level = &v
	}
	if !plan.Status.Equal(state.Status) {
		v := plan.Status.ValueString()
		in.Status = &v
	}
	if !plan.OwnerAgentId.Equal(state.OwnerAgentId) {
		in.OwnerAgentId = goalRefRawMessage(plan.OwnerAgentId)
	}
	if !plan.ParentId.Equal(state.ParentId) {
		in.ParentId = goalRefRawMessage(plan.ParentId)
	}
	return in
}

// goalRefRawMessage renders the PATCH payload for a changed nullable ref
// (owner_agent_id/parent_id): explicit JSON null when the plan clears it,
// otherwise a JSON string of the new id. Mirrors agent's reports_to tri-state
// (see agent_resource.go buildAgentUpdateInput) — live-proven 2026-07-22 that
// an explicit null cleanly clears both goal refs (no env-style exception).
func goalRefRawMessage(planVal types.String) json.RawMessage {
	if planVal.IsNull() {
		return json.RawMessage("null")
	}
	b, _ := json.Marshal(planVal.ValueString())
	return json.RawMessage(b)
}

func goalUpdateInputEmpty(in client.GoalUpdateInput) bool {
	return in.Title == nil && in.Description == nil && in.Level == nil && in.Status == nil &&
		in.OwnerAgentId == nil && in.ParentId == nil
}

// goalFromAPI maps a GET/Create/Update response onto resource state. Every
// attribute is server truth: unlike agent's adapterConfig, the goal API never
// normalizes or hides a field the provider sends, so there is no need to
// preserve the caller's own copy of anything (contrast secret's key casing,
// agent's write-only value).
func goalFromAPI(got *client.Goal) goalResourceModel {
	return goalResourceModel{
		ID:           types.StringValue(got.ID),
		CompanyID:    types.StringValue(got.CompanyID),
		Title:        types.StringValue(got.Title),
		Description:  optionalString(got.Description),
		Level:        optionalString(got.Level),
		Status:       optionalString(got.Status),
		OwnerAgentId: optionalString(got.OwnerAgentId),
		ParentId:     optionalString(got.ParentId),
	}
}

var (
	_ resource.Resource                = &goalResource{}
	_ resource.ResourceWithConfigure   = &goalResource{}
	_ resource.ResourceWithImportState = &goalResource{}
)
