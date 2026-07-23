// internal/provider/routine_trigger_resource.go
// paperclip_routine_trigger: schedule-kind trigger on a routine.
//
// 設計要點（server 原始碼實證，2026-07-22）：
//   - kind 固定 schedule（client 型別硬編）；webhook/api kind 不做（runtime 面）。
//   - trigger 無單獨 GET → Read 走 GET routine detail 的 triggers 陣列 list-then-find。
//   - PATCH /routine-triggers/{id} 是 partial → 只送變更欄位。
//   - 無效 cron 格式由 server 422 拒絕 → apply 明確報錯、不留半套 state
//     （TFPC-5 情境 3 的落點）。
package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

type routineTriggerResource struct {
	client *client.Client
}

func NewRoutineTriggerResource() resource.Resource { return &routineTriggerResource{} }

type routineTriggerResourceModel struct {
	ID             types.String `tfsdk:"id"`
	RoutineID      types.String `tfsdk:"routine_id"`
	CronExpression types.String `tfsdk:"cron_expression"`
	Timezone       types.String `tfsdk:"timezone"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	Label          types.String `tfsdk:"label"`
}

func (r *routineTriggerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_routine_trigger"
}

func (r *routineTriggerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Schedule trigger on a routine (kind is always \"schedule\"; webhook/api triggers are runtime-facing and not managed here).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"routine_id": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"cron_expression": schema.StringAttribute{Required: true},
			"timezone": schema.StringAttribute{
				Optional:      true,
				Computed:      true, // 未設定時 server 預設 UTC
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Optional:      true,
				Computed:      true, // 未設定時 server 預設 true
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"label": schema.StringAttribute{Optional: true},
		},
	}
}

func (r *routineTriggerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func findTriggerByID(list []client.RoutineTrigger, id string) (*client.RoutineTrigger, bool) {
	for i := range list {
		if list[i].ID == id {
			return &list[i], true
		}
	}
	return nil, false
}

func triggerToModel(routineID string, tg *client.RoutineTrigger) routineTriggerResourceModel {
	m := routineTriggerResourceModel{
		ID:             types.StringValue(tg.ID),
		RoutineID:      types.StringValue(routineID),
		CronExpression: types.StringValue(tg.CronExpression),
		Timezone:       types.StringValue(tg.Timezone),
		Enabled:        types.BoolValue(tg.Enabled),
	}
	if tg.Label != "" {
		m.Label = types.StringValue(tg.Label)
	} else {
		m.Label = types.StringNull()
	}
	return m
}

// buildTriggerUpdateInput diffs plan vs state; 只有變更欄位進 PATCH body。
func buildTriggerUpdateInput(plan, state routineTriggerResourceModel) client.RoutineTriggerUpdateInput {
	var in client.RoutineTriggerUpdateInput
	if !plan.CronExpression.Equal(state.CronExpression) {
		v := plan.CronExpression.ValueString()
		in.CronExpression = &v
	}
	if !plan.Timezone.Equal(state.Timezone) && !plan.Timezone.IsUnknown() {
		v := plan.Timezone.ValueString()
		in.Timezone = &v
	}
	if !plan.Enabled.Equal(state.Enabled) && !plan.Enabled.IsUnknown() {
		v := plan.Enabled.ValueBool()
		in.Enabled = &v
	}
	if !plan.Label.Equal(state.Label) {
		v := plan.Label.ValueString() // label 清空→送空字串（server trim 後存 null 等價）
		in.Label = &v
	}
	return in
}

func (r *routineTriggerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan routineTriggerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := client.RoutineTriggerCreateInput{CronExpression: plan.CronExpression.ValueString()}
	if !plan.Timezone.IsNull() && !plan.Timezone.IsUnknown() {
		in.Timezone = plan.Timezone.ValueString()
	}
	if !plan.Enabled.IsNull() && !plan.Enabled.IsUnknown() {
		v := plan.Enabled.ValueBool()
		in.Enabled = &v
	}
	if !plan.Label.IsNull() && !plan.Label.IsUnknown() {
		in.Label = plan.Label.ValueString()
	}
	got, err := r.client.CreateRoutineTrigger(ctx, plan.RoutineID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Create routine trigger failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, triggerToModel(plan.RoutineID.ValueString(), got))...)
}

func (r *routineTriggerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state routineTriggerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rt, err := r.client.GetRoutine(ctx, state.RoutineID.ValueString())
	if err != nil {
		if client.IsGone(err) { // routine 已消失 → trigger 一併回收
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read routine (for trigger) failed", err.Error())
		return
	}
	tg, found := findTriggerByID(rt.Triggers, state.ID.ValueString())
	if !found { // live 端已刪 → 漂移、計畫重建
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, triggerToModel(state.RoutineID.ValueString(), tg))...)
}

func (r *routineTriggerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state routineTriggerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.client.UpdateRoutineTrigger(ctx, state.ID.ValueString(), buildTriggerUpdateInput(plan, state))
	if err != nil {
		resp.Diagnostics.AddError("Update routine trigger failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, triggerToModel(state.RoutineID.ValueString(), got))...)
}

func (r *routineTriggerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state routineTriggerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteRoutineTrigger(ctx, state.ID.ValueString()); err != nil && !client.IsGone(err) {
		resp.Diagnostics.AddError("Delete routine trigger failed", err.Error())
	}
}

// ImportState: trigger 無單獨 GET（Read 走 parent routine 的 triggers envelope），
// 所以 import ID 必須是 "routine_id/trigger_id" 複合鍵（同 secret 的 company_id/secret_id 手法）。
func (r *routineTriggerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf(`Expected import ID in the form "routine_id/trigger_id", got: %q`, req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("routine_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
