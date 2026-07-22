// internal/provider/company_data_source.go
// paperclip_company data source: resolve an existing company by exact name.
// 給「既有公司安全上車」用——import 前以 name 解析 live uuid，.tf 不必硬編 id。
package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

type companyDataSource struct {
	client *client.Client
}

func NewCompanyDataSource() datasource.DataSource { return &companyDataSource{} }

type companyDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func (d *companyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_company"
}

func (d *companyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up an existing company by exact name.",
		Attributes: map[string]schema.Attribute{
			"name":        schema.StringAttribute{Required: true, Description: "Exact company name to resolve."},
			"id":          schema.StringAttribute{Computed: true},
			"description": schema.StringAttribute{Computed: true},
		},
	}
}

func (d *companyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected ProviderData", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
		return
	}
	d.client = c
}

// findCompanyByName resolves an exact-name match out of the full list.
// name 不強制唯一（server 僅對 issuePrefix 做唯一性），所以 0/1/N 三態都要有明確結果：
// 0 → not-found（含查詢名）；N → 歧義錯誤（列出候選 id，提示改用 id）。
func findCompanyByName(companies []client.Company, name string) (*client.Company, error) {
	var matches []client.Company
	for _, co := range companies {
		if co.Name == name {
			matches = append(matches, co)
		}
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		return nil, fmt.Errorf("no company named %q found", name)
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		return nil, fmt.Errorf("multiple companies named %q (ids: %v); disambiguate by importing with an explicit id", name, ids)
	}
}

func (d *companyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg companyDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	companies, err := d.client.ListCompanies(ctx)
	if err != nil {
		resp.Diagnostics.AddError("List companies failed", err.Error())
		return
	}
	got, err := findCompanyByName(companies, cfg.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Company lookup failed", err.Error())
		return
	}
	state := companyDataSourceModel{
		ID:          types.StringValue(got.ID),
		Name:        types.StringValue(got.Name),
		Description: types.StringValue(got.Description),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
