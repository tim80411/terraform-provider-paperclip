// internal/provider/secret_provider_config_resource.go
// paperclip_secret_provider_config: external secret provider vault config.
//
// 設計要點（server 原始碼實證，2026-07-22）：
//   - config 是 opaque record（值不得含 secret——server 端強制拒絕），TF 端以
//     config_json 字串承載；Read 用 JSON 語意比較避免 key 排序假漂移。
//   - provider 換類型 = config schema 全變 → provider 欄位 RequiresReplace。
//   - 無效參數由 server 422 拒絕 → apply 明確報錯、不留半套 state（卡上情境 2）。
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

type secretProviderConfigResource struct {
	client *client.Client
}

func NewSecretProviderConfigResource() resource.Resource { return &secretProviderConfigResource{} }

type secretProviderConfigResourceModel struct {
	ID          types.String `tfsdk:"id"`
	CompanyID   types.String `tfsdk:"company_id"`
	Provider    types.String `tfsdk:"provider"`
	DisplayName types.String `tfsdk:"display_name"`
	Status      types.String `tfsdk:"status"`
	IsDefault   types.Bool   `tfsdk:"is_default"`
	ConfigJSON  types.String `tfsdk:"config_json"`
}

func (r *secretProviderConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret_provider_config"
}

func (r *secretProviderConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "External secret provider vault config. config_json must NOT contain secret values (the API rejects sensitive keys); it carries connection parameters only.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"company_id": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"provider": schema.StringAttribute{
				Required:      true,
				Description:   "local_encrypted | aws_secrets_manager | gcp_secret_manager | vault (gcp/vault are locked coming_soon server-side).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"display_name": schema.StringAttribute{Required: true},
			"status": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"is_default": schema.BoolAttribute{
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"config_json": schema.StringAttribute{
				Optional:      true,
				Computed:      true, // 未設定 → server 預設 {}
				Description:   "JSON object of provider connection parameters (no secret values).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *secretProviderConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// jsonSemanticallyEqual：兩段 JSON 依解析後值比較；任一非法時退回字串比較。
func jsonSemanticallyEqual(a, b string) bool {
	var av, bv any
	if json.Unmarshal([]byte(a), &av) != nil || json.Unmarshal([]byte(b), &bv) != nil {
		return a == b
	}
	return reflect.DeepEqual(av, bv)
}

// parseConfigJSON: config_json 字串 → map；空字串視為 {}。
func parseConfigJSON(s string) (map[string]any, error) {
	if s == "" {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	return m, nil
}

// spcToModel maps API → TF state。prior 的 config_json 若與 read-back 語意相等則
// 保留原字串（避免 key 排序假漂移）。
func spcToModel(companyID string, got *client.SecretProviderConfig, priorConfigJSON types.String) secretProviderConfigResourceModel {
	cfgBytes, _ := json.Marshal(got.Config)
	cfgStr := string(cfgBytes)
	if got.Config == nil {
		cfgStr = "{}"
	}
	if !priorConfigJSON.IsNull() && !priorConfigJSON.IsUnknown() && jsonSemanticallyEqual(priorConfigJSON.ValueString(), cfgStr) {
		cfgStr = priorConfigJSON.ValueString()
	}
	return secretProviderConfigResourceModel{
		ID:          types.StringValue(got.ID),
		CompanyID:   types.StringValue(companyID),
		Provider:    types.StringValue(got.Provider),
		DisplayName: types.StringValue(got.DisplayName),
		Status:      types.StringValue(got.Status),
		IsDefault:   types.BoolValue(got.IsDefault),
		ConfigJSON:  types.StringValue(cfgStr),
	}
}

func (r *secretProviderConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan secretProviderConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := client.SecretProviderConfigCreateInput{
		Provider:    plan.Provider.ValueString(),
		DisplayName: plan.DisplayName.ValueString(),
	}
	if !plan.Status.IsNull() && !plan.Status.IsUnknown() {
		in.Status = plan.Status.ValueString()
	}
	if !plan.IsDefault.IsNull() && !plan.IsDefault.IsUnknown() {
		v := plan.IsDefault.ValueBool()
		in.IsDefault = &v
	}
	if !plan.ConfigJSON.IsNull() && !plan.ConfigJSON.IsUnknown() {
		cfg, err := parseConfigJSON(plan.ConfigJSON.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid config_json", err.Error())
			return
		}
		in.Config = cfg
	}
	got, err := r.client.CreateSecretProviderConfig(ctx, plan.CompanyID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Create secret provider config failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, spcToModel(plan.CompanyID.ValueString(), got, plan.ConfigJSON))...)
}

func (r *secretProviderConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state secretProviderConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.client.GetSecretProviderConfig(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsGone(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read secret provider config failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, spcToModel(state.CompanyID.ValueString(), got, state.ConfigJSON))...)
}

func (r *secretProviderConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state secretProviderConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var in client.SecretProviderConfigUpdateInput
	if !plan.DisplayName.Equal(state.DisplayName) {
		v := plan.DisplayName.ValueString()
		in.DisplayName = &v
	}
	if !plan.Status.Equal(state.Status) && !plan.Status.IsUnknown() {
		v := plan.Status.ValueString()
		in.Status = &v
	}
	if !plan.IsDefault.Equal(state.IsDefault) && !plan.IsDefault.IsUnknown() {
		v := plan.IsDefault.ValueBool()
		in.IsDefault = &v
	}
	if !plan.ConfigJSON.IsUnknown() && !plan.ConfigJSON.IsNull() &&
		!jsonSemanticallyEqual(plan.ConfigJSON.ValueString(), state.ConfigJSON.ValueString()) {
		cfg, err := parseConfigJSON(plan.ConfigJSON.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid config_json", err.Error())
			return
		}
		in.Config = &cfg
	}
	got, err := r.client.UpdateSecretProviderConfig(ctx, state.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Update secret provider config failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, spcToModel(state.CompanyID.ValueString(), got, plan.ConfigJSON))...)
}

func (r *secretProviderConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state secretProviderConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteSecretProviderConfig(ctx, state.ID.ValueString()); err != nil && !client.IsGone(err) {
		resp.Diagnostics.AddError("Delete secret provider config failed", err.Error())
	}
}
