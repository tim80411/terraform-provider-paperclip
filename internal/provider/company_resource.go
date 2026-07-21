// internal/provider/company_resource.go
// paperclip_company resource: declarative CRUD + import over the /api/companies endpoints.
package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

type companyResource struct {
	client *client.Client
}

func NewCompanyResource() resource.Resource { return &companyResource{} }

type companyResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func (r *companyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_company"
}

func (r *companyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true},
			"name":        schema.StringAttribute{Required: true},
			"description": schema.StringAttribute{Optional: true, Computed: true},
		},
	}
}

func (r *companyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *companyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan companyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := client.CompanyCreateInput{Name: plan.Name.ValueString()}
	if !plan.Description.IsNull() {
		in.Description = plan.Description.ValueString()
	}
	got, err := r.client.CreateCompany(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Create company failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, companyFromAPI(got))...)
}

func (r *companyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state companyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.client.GetCompany(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read company failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, companyFromAPI(got))...)
}

func (r *companyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state companyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// 只送有變的欄位（指標）→ 保留未管欄位（spec §6.3）
	in := buildCompanyUpdateInput(plan, state)
	got, err := r.client.UpdateCompany(ctx, state.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Update company failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, companyFromAPI(got))...)
}

func (r *companyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state companyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteCompany(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete company failed", err.Error())
	}
}

func (r *companyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// buildCompanyUpdateInput returns an update input containing ONLY the fields
// that changed between state and plan (spec §6.3: never send unchanged fields).
func buildCompanyUpdateInput(plan, state companyResourceModel) client.CompanyUpdateInput {
	var in client.CompanyUpdateInput
	if !plan.Name.Equal(state.Name) {
		v := plan.Name.ValueString()
		in.Name = &v
	}
	if !plan.Description.Equal(state.Description) && !plan.Description.IsNull() {
		v := plan.Description.ValueString()
		in.Description = &v
	}
	return in
}

func companyFromAPI(c *client.Company) companyResourceModel {
	return companyResourceModel{
		ID:          types.StringValue(c.ID),
		Name:        types.StringValue(c.Name),
		Description: types.StringValue(c.Description),
	}
}

var (
	_ resource.Resource                = &companyResource{}
	_ resource.ResourceWithConfigure   = &companyResource{}
	_ resource.ResourceWithImportState = &companyResource{}
)
