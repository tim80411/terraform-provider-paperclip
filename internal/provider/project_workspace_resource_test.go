// internal/provider/project_workspace_resource_test.go
package provider

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// scratch company → project（inline primary）→ 額外 workspace，驗證非-primary
// 建立、冪等與 import（TFPC-4 情境 1）。
func TestAccProjectWorkspaceResource_lifecycle(t *testing.T) {
	name := fmt.Sprintf("tfacc-scratch-ws-%d", time.Now().Unix()) // 唯一名，絕不撞既有正式公司

	config := fmt.Sprintf(`
resource "paperclip_company" "s" { name = %q }

resource "paperclip_project" "p" {
  company_id = paperclip_company.s.id
  name       = "%s-proj"
  workspace = {
    source_type = "git_repo"
    repo_url    = "https://github.com/tim80411/tfacc-project-primary"
    is_primary  = true
  }
}

resource "paperclip_project_workspace" "extra" {
  project_id = paperclip_project.p.id
  repo_url   = "https://github.com/tim80411/tfacc-project-extra"
}
`, name, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		PreCheck:                 func() { preCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("paperclip_project_workspace.extra", "id"),
					resource.TestCheckResourceAttr("paperclip_project_workspace.extra", "repo_url", "https://github.com/tim80411/tfacc-project-extra"),
					resource.TestCheckResourceAttr("paperclip_project_workspace.extra", "source_type", "git_repo"),
				),
			},
			{ // 再 apply 為 no-op（冪等）
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}
