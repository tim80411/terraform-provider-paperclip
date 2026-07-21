// internal/provider/company_resource.go （最小空殼，Task 5 會覆寫）
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

type companyResource struct{}

func NewCompanyResource() resource.Resource { return &companyResource{} }

func (r *companyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_company"
}
func (r *companyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{}
}
func (r *companyResource) Create(_ context.Context, _ resource.CreateRequest, _ *resource.CreateResponse) {}
func (r *companyResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse)       {}
func (r *companyResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {}
func (r *companyResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {}
