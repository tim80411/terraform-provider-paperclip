// internal/provider/project_workspace_resource.go
// paperclip_project_workspace: declarative non-primary git workspace on a project.
//
// API 面（server 原始碼實證，2026-07-22）：
//   - 只有 GET list / POST create / DELETE——無 PATCH → 全欄位 RequiresReplace。
//   - 本 resource 不暴露 is_primary：server 端 isPrimary=true 會把既有 primary
//     降級（primary 轉移），那會跟 paperclip_project 的 inline workspace 打架；
//     primary 歸 project resource 管，這裡只管額外（非-primary）workspace。
//   - workspace 無單獨 GET → Read 用 list-then-find 做漂移偵測。
package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

type projectWorkspaceResource struct {
	client *client.Client
}

func NewProjectWorkspaceResource() resource.Resource { return &projectWorkspaceResource{} }

type projectWorkspaceResourceModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	RepoUrl    types.String `tfsdk:"repo_url"`
	Name       types.String `tfsdk:"name"`
	SourceType types.String `tfsdk:"source_type"`
}

func (r *projectWorkspaceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_workspace"
}

func (r *projectWorkspaceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Additional (non-primary) git workspace on a project. Workspaces are immutable: any change forces replacement. Primary workspace is managed inline on paperclip_project.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"project_id": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"repo_url": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Optional: true,
				Computed: true, // 未指定時 server 由 repoUrl 派生
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"source_type": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *projectWorkspaceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func findWorkspaceByID(list []client.Workspace, id string) (*client.Workspace, bool) {
	for i := range list {
		if list[i].ID == id {
			return &list[i], true
		}
	}
	return nil, false
}

func workspaceToModel(projectID string, ws *client.Workspace) projectWorkspaceResourceModel {
	return projectWorkspaceResourceModel{
		ID:         types.StringValue(ws.ID),
		ProjectID:  types.StringValue(projectID),
		RepoUrl:    types.StringValue(ws.RepoUrl),
		Name:       types.StringValue(ws.Name),
		SourceType: types.StringValue(ws.SourceType),
	}
}

func (r *projectWorkspaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectWorkspaceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := client.ProjectWorkspaceCreateInput{RepoUrl: plan.RepoUrl.ValueString()}
	if !plan.Name.IsNull() && !plan.Name.IsUnknown() {
		in.Name = plan.Name.ValueString()
	}
	got, err := r.client.CreateProjectWorkspace(ctx, plan.ProjectID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Create project workspace failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, workspaceToModel(plan.ProjectID.ValueString(), got))...)
}

func (r *projectWorkspaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectWorkspaceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	list, err := r.client.ListProjectWorkspaces(ctx, state.ProjectID.ValueString())
	if err != nil {
		if client.IsGone(err) { // project 本身已消失 → workspace 一併回收
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("List project workspaces failed", err.Error())
		return
	}
	ws, found := findWorkspaceByID(list, state.ID.ValueString())
	if !found { // live 端已刪 → 呈現漂移、計畫重建（TFPC-4 情境 2）
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, workspaceToModel(state.ProjectID.ValueString(), ws))...)
}

// 全欄位 RequiresReplace，Update 永遠不會被呼叫；保留守衛以防 schema 演進時遺漏。
func (r *projectWorkspaceResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update not supported", "project workspaces are immutable; all attributes require replacement")
}

func (r *projectWorkspaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectWorkspaceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteProjectWorkspace(ctx, state.ProjectID.ValueString(), state.ID.ValueString())
	if err != nil && !client.IsGone(err) {
		resp.Diagnostics.AddError("Delete project workspace failed", err.Error())
	}
}
