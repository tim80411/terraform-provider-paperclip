// internal/provider/agent_resource_test.go
package provider

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

// boardSkill is an always-available company-managed skill (live 探測：每間 company 都有，
// 不需匯入)。用它當 desired_skill 才能在 scratch company 上驗證 paperclipSkillSync 的存活。
const boardSkill = "paperclipai/paperclip/paperclip-board"

// checkAgentAdapterSurvives GETs the agent via the raw API and asserts the model
// changed to wantModel while the paperclip-owned adapterConfig keys
// (paperclipSkillSync + instructions*) are STILL present. This is the make-or-break
// acceptance check for the whole task: proof that the GET-merge-PATCH Update path
// does not clobber the opaque bag.
func checkAgentAdapterSurvives(resourceName, wantModel string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("%s not in state", resourceName)
		}
		c := client.New(os.Getenv("PAPERCLIP_API_BASE"), os.Getenv("PAPERCLIP_API_KEY"))
		got, err := c.GetAgent(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("GetAgent(%s): %w", rs.Primary.ID, err)
		}
		ac := got.AdapterConfig
		if ac["model"] != wantModel {
			return fmt.Errorf("adapterConfig.model = %v, want %q", ac["model"], wantModel)
		}
		sync, ok := ac["paperclipSkillSync"].(map[string]any)
		if !ok {
			return fmt.Errorf("paperclipSkillSync MISSING after model change — the exact regression MergeAdapterConfig guards against: %+v", ac)
		}
		if ds, _ := sync["desiredSkills"].([]any); len(ds) == 0 {
			return fmt.Errorf("paperclipSkillSync.desiredSkills empty after model change: %+v", sync)
		}
		if _, ok := ac["instructionsEntryFile"]; !ok {
			return fmt.Errorf("instructions* MISSING after model change: %+v", ac)
		}
		return nil
	}
}

func TestAccAgentResource_lifecycle(t *testing.T) {
	ts := time.Now().Unix()
	companyName := fmt.Sprintf("tfacc-agent-%d", ts)
	const modelInitial = "claude-sonnet-4-6"
	const modelUpdated = "claude-opus-4-8"

	// config renders the whole graph: company → secret → root agent → subordinate agent.
	// 依賴圖保證建立順序（root 先於 sub，secret 先於 sub.env）。
	config := func(model string) string {
		return fmt.Sprintf(`
resource "paperclip_company" "c" {
  name = %q
}

resource "paperclip_company_secret" "gh" {
  company_id    = paperclip_company.c.id
  name          = "gh-token"
  key           = "gh_token"
  value         = "ghp_probe_value"
  value_version = "1"
}

resource "paperclip_agent" "root" {
  company_id = paperclip_company.c.id
  name       = "Chief"
  role       = "ceo"
  icon       = "crown"
  adapter = {
    model = %q
  }
}

resource "paperclip_agent" "sub" {
  company_id     = paperclip_company.c.id
  name           = "Scout"
  role           = "engineer"
  title          = "Data Engineer"
  icon           = "database"
  capabilities   = "does data things"
  reports_to     = paperclip_agent.root.id
  desired_skills = [%q]
  adapter = {
    model  = %q
    chrome = true
    env = {
      GH_TOKEN = { secret_id = paperclip_company_secret.gh.id }
    }
  }
}
`, companyName, model, boardSkill, model)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		PreCheck:                 func() { preCheck(t) },
		CheckDestroy: func(s *terraform.State) error {
			c := client.New(os.Getenv("PAPERCLIP_API_BASE"), os.Getenv("PAPERCLIP_API_KEY"))
			for _, rs := range s.RootModule().Resources {
				if rs.Type != "paperclip_agent" {
					continue
				}
				_, err := c.GetAgent(context.Background(), rs.Primary.ID)
				if err == nil {
					return fmt.Errorf("agent %s still exists after destroy", rs.Primary.ID)
				}
				if !client.IsGone(err) {
					return fmt.Errorf("unexpected error checking destroy: %w", err)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{ // create: chain + skills attached at create (so paperclipSkillSync exists)
				Config: config(modelInitial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("paperclip_agent.root", "id"),
					resource.TestCheckNoResourceAttr("paperclip_agent.root", "reports_to"),
					resource.TestCheckResourceAttrPair("paperclip_agent.sub", "reports_to", "paperclip_agent.root", "id"),
					resource.TestCheckResourceAttr("paperclip_agent.sub", "adapter.model", modelInitial),
					resource.TestCheckResourceAttr("paperclip_agent.sub", "adapter.chrome", "true"),
					resource.TestCheckResourceAttr("paperclip_agent.sub", "desired_skills.0", boardSkill),
					resource.TestCheckResourceAttrPair("paperclip_agent.sub", "adapter.env.GH_TOKEN.secret_id", "paperclip_company_secret.gh", "id"),
					// sanity: even at create, the two paperclip-owned keys coexist with our managed ones.
					checkAgentAdapterSurvives("paperclip_agent.sub", modelInitial),
				),
			},
			{ // update adapter.model → THE make-or-break: paperclipSkillSync + instructions* must survive
				Config: config(modelUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("paperclip_agent.sub", "adapter.model", modelUpdated),
					checkAgentAdapterSurvives("paperclip_agent.sub", modelUpdated),
				),
			},
			{ // import subordinate by plain id (passthrough)
				ResourceName:      "paperclip_agent.sub",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
