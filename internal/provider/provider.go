// internal/provider/provider.go
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

type paperclipProvider struct{}

func New() provider.Provider { return &paperclipProvider{} }

type providerModel struct {
	APIBase types.String `tfsdk:"api_base"`
	APIKey  types.String `tfsdk:"api_key"`
}

func (p *paperclipProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "paperclip"
}

func (p *paperclipProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"api_base": schema.StringAttribute{
				Optional:    true,
				Description: "paperclip API base URL. Defaults to env PAPERCLIP_API_BASE.",
			},
			"api_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Board API bearer token. Defaults to env PAPERCLIP_API_KEY.",
			},
		},
	}
}

func (p *paperclipProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	base := cfg.APIBase.ValueString()
	if base == "" {
		base = os.Getenv("PAPERCLIP_API_BASE")
	}
	key := cfg.APIKey.ValueString()
	if key == "" {
		key = os.Getenv("PAPERCLIP_API_KEY")
	}
	if base == "" {
		resp.Diagnostics.AddError("Missing api_base", "Set provider api_base or env PAPERCLIP_API_BASE.")
	}
	if key == "" {
		resp.Diagnostics.AddError("Missing api_key", "Set provider api_key or env PAPERCLIP_API_KEY.")
	}
	if resp.Diagnostics.HasError() {
		return
	}
	c := client.New(base, key)
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *paperclipProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewCompanyResource,
		NewSecretResource,
		NewAgentResource,
	}
}

func (p *paperclipProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}
