// internal/provider/company_budget_resource.go
// paperclip_company_budget: monthly budget cap on a company.
//
// 設計要點（server 原始碼實證，2026-07-22）：
//   - 值存在 company 物件（budgetMonthlyCents，DB default 0）；PATCH
//     /companies/{cid}/budgets 同時 upsert company-scope budget policy。
//   - destroy = PATCH 0（還原 DB 預設）——不是猜測，是 schema default 實證。
//   - 一間公司一個預算 → resource id = company_id（singleton per company）。
package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

type companyBudgetResource struct {
	client *client.Client
}

func NewCompanyBudgetResource() resource.Resource { return &companyBudgetResource{} }

type companyBudgetResourceModel struct {
	ID           types.String `tfsdk:"id"`
	CompanyID    types.String `tfsdk:"company_id"`
	MonthlyCents types.Int64  `tfsdk:"monthly_cents"`
}

func (r *companyBudgetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_company_budget"
}

func (r *companyBudgetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Monthly budget cap (in cents) on a company. One budget per company. Destroy resets to 0 (the server default, meaning no cap).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"company_id": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"monthly_cents": schema.Int64Attribute{
				Required:    true,
				Description: "Monthly budget in cents (non-negative). 0 = no cap (server default).",
			},
		},
	}
}

func (r *companyBudgetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *companyBudgetResource) apply(ctx context.Context, companyID string, cents int64) (*client.Company, error) {
	return r.client.UpdateCompanyBudget(ctx, companyID, cents)
}

func (r *companyBudgetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan companyBudgetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.apply(ctx, plan.CompanyID.ValueString(), plan.MonthlyCents.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Set company budget failed", err.Error())
		return
	}
	state := companyBudgetResourceModel{
		ID:           types.StringValue(plan.CompanyID.ValueString()), // singleton per company
		CompanyID:    plan.CompanyID,
		MonthlyCents: types.Int64Value(got.BudgetMonthlyCents),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *companyBudgetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state companyBudgetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.client.GetCompany(ctx, state.CompanyID.ValueString())
	if err != nil {
		if client.IsGone(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read company (for budget) failed", err.Error())
		return
	}
	state.MonthlyCents = types.Int64Value(got.BudgetMonthlyCents)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *companyBudgetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan companyBudgetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.apply(ctx, plan.CompanyID.ValueString(), plan.MonthlyCents.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Update company budget failed", err.Error())
		return
	}
	plan.ID = types.StringValue(plan.CompanyID.ValueString())
	plan.MonthlyCents = types.Int64Value(got.BudgetMonthlyCents)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *companyBudgetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state companyBudgetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// destroy = 還原 DB 預設 0（無上限）
	if _, err := r.apply(ctx, state.CompanyID.ValueString(), 0); err != nil && !client.IsGone(err) {
		resp.Diagnostics.AddError("Reset company budget failed", err.Error())
	}
}

// ImportState: singleton per company（resource id = company_id）→ import ID
// 就是 company_id，同時回填 id 與 company_id；monthly_cents 由 Read 從
// GET company 的 budgetMonthlyCents 回填。
func (r *companyBudgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("company_id"), req.ID)...)
}
