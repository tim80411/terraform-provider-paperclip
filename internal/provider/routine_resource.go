// internal/provider/routine_resource.go
// paperclip_routine: declarative routine (scheduled agent work definition).
//
// 設計要點（server 原始碼實證，2026-07-22）：
//   - destroy = archive（無 DELETE 端點；archived 是 terminal state）。
//   - Read 時 status=archived 視同已刪（RemoveResource）——UI 手動 archive 會被
//     偵測為漂移並計畫重建。
//   - PATCH partial-merge：只送 schema 管的欄位；v1 不管 priority/policies/
//     variables/env/project 等欄位，它們被 partial PATCH 保留。
//   - trigger 是獨立 resource（paperclip_routine_trigger，TFPC-5 拆分線後補）。
package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

type routineResource struct {
	client *client.Client
}

func NewRoutineResource() resource.Resource { return &routineResource{} }

type routineResourceModel struct {
	ID              types.String `tfsdk:"id"`
	CompanyID       types.String `tfsdk:"company_id"`
	Title           types.String `tfsdk:"title"`
	Description     types.String `tfsdk:"description"`
	Status          types.String `tfsdk:"status"`
	AssigneeAgentID types.String `tfsdk:"assignee_agent_id"`
}

func (r *routineResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_routine"
}

func (r *routineResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Routine (scheduled agent work definition). Destroy archives the routine (the API has no hard delete). Triggers are managed separately.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"company_id": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"title":       schema.StringAttribute{Required: true},
			"description": schema.StringAttribute{Optional: true, Computed: true},
			"status": schema.StringAttribute{
				Optional:    true,
				Computed:    true, // 未設定時 server 預設 active；避免未知值攤成假 diff
				Description: "active | paused (archived is the destroy state, not configurable).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"assignee_agent_id": schema.StringAttribute{Optional: true},
		},
	}
}

func (r *routineResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func routineToModel(companyID string, rt *client.Routine) routineResourceModel {
	m := routineResourceModel{
		ID:          types.StringValue(rt.ID),
		CompanyID:   types.StringValue(companyID),
		Title:       types.StringValue(rt.Title),
		Description: types.StringValue(rt.Description),
		Status:      types.StringValue(rt.Status),
	}
	if rt.AssigneeAgentId != "" {
		m.AssigneeAgentID = types.StringValue(rt.AssigneeAgentId)
	} else {
		m.AssigneeAgentID = types.StringNull()
	}
	return m
}

// buildRoutineUpdateInput diffs plan vs state; 只有變更的欄位進 PATCH body。
// assignee 三態：未變→nil（不送）；清空→JSON null；指定→quoted uuid。
func buildRoutineUpdateInput(plan, state routineResourceModel) client.RoutineUpdateInput {
	var in client.RoutineUpdateInput
	if !plan.Title.Equal(state.Title) {
		v := plan.Title.ValueString()
		in.Title = &v
	}
	if !plan.Description.Equal(state.Description) {
		v := plan.Description.ValueString()
		in.Description = &v
	}
	if !plan.Status.Equal(state.Status) && !plan.Status.IsUnknown() {
		v := plan.Status.ValueString()
		in.Status = &v
	}
	if !plan.AssigneeAgentID.Equal(state.AssigneeAgentID) {
		if plan.AssigneeAgentID.IsNull() {
			in.AssigneeAgentId = json.RawMessage("null")
		} else {
			quoted, _ := json.Marshal(plan.AssigneeAgentID.ValueString())
			in.AssigneeAgentId = quoted
		}
	}
	return in
}

func (r *routineResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan routineResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := client.RoutineCreateInput{Title: plan.Title.ValueString()}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		in.Description = plan.Description.ValueString()
	}
	if !plan.Status.IsNull() && !plan.Status.IsUnknown() {
		in.Status = plan.Status.ValueString()
	}
	if !plan.AssigneeAgentID.IsNull() && !plan.AssigneeAgentID.IsUnknown() {
		in.AssigneeAgentId = plan.AssigneeAgentID.ValueString()
	}
	got, err := r.client.CreateRoutine(ctx, plan.CompanyID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Create routine failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, routineToModel(plan.CompanyID.ValueString(), got))...)
}

func (r *routineResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state routineResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.client.GetRoutine(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsGone(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read routine failed", err.Error())
		return
	}
	if got.Status == "archived" { // archive 即刪除 → 呈現漂移、計畫重建
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, routineToModel(state.CompanyID.ValueString(), got))...)
}

func (r *routineResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state routineResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.client.UpdateRoutine(ctx, state.ID.ValueString(), buildRoutineUpdateInput(plan, state))
	if err != nil {
		resp.Diagnostics.AddError("Update routine failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, routineToModel(state.CompanyID.ValueString(), got))...)
}

func (r *routineResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state routineResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.ArchiveRoutine(ctx, state.ID.ValueString()); err != nil && !client.IsGone(err) {
		resp.Diagnostics.AddError("Archive routine failed", err.Error())
	}
}
