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

// checkAgentAdapterKeysAbsent GETs the agent via the raw API and asserts each of
// `absentKeys` is GONE from adapterConfig (removed, or at worst null — both mean
// "cleared" to the provider), WHILE the paperclip-owned keys (paperclipSkillSync +
// instructions*) and the Required model key are STILL present. This is the make-or-
// break for the clear-on-removal fix: it proves the computed-full-config replace path
// removes only the managed keys the user dropped and never the opaque bag paperclip owns.
func checkAgentAdapterKeysAbsent(resourceName string, absentKeys ...string) resource.TestCheckFunc {
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
		for _, k := range absentKeys {
			if v, present := ac[k]; present && v != nil {
				return fmt.Errorf("adapterConfig[%q] = %v, want CLEARED (absent/null) — non-converging remnant", k, v)
			}
		}
		if _, ok := ac["paperclipSkillSync"].(map[string]any); !ok {
			return fmt.Errorf("paperclipSkillSync MISSING after clear — the exact regression the fix guards against: %+v", ac)
		}
		if _, ok := ac["instructionsEntryFile"]; !ok {
			return fmt.Errorf("instructions* MISSING after clear: %+v", ac)
		}
		if ac["model"] == nil {
			return fmt.Errorf("model MISSING after clear (Required key): %+v", ac)
		}
		return nil
	}
}

// checkAgentModelClearedSkillsSurvive GETs the agent via the raw API and asserts
// `model` is GONE from adapterConfig after the WHOLE adapter block was removed (I-2),
// WHILE paperclip-owned keys (paperclipSkillSync + instructions*) survive. This is the
// opposite polarity of checkAgentAdapterKeysAbsent (which requires model to remain): here
// the user dropped the entire block, so model itself must clear. Live-proven 2026-07-22
// that a model-less replaceAdapterConfig=true PATCH converges (agent ends model-less).
func checkAgentModelClearedSkillsSurvive(resourceName string) resource.TestCheckFunc {
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
		if v, present := ac["model"]; present && v != nil {
			return fmt.Errorf("adapterConfig.model = %v, want CLEARED after whole-block removal", v)
		}
		if _, ok := ac["paperclipSkillSync"].(map[string]any); !ok {
			return fmt.Errorf("paperclipSkillSync MISSING after model clear — unmanaged bag wrongly wiped: %+v", ac)
		}
		if _, ok := ac["instructionsEntryFile"]; !ok {
			return fmt.Errorf("instructions* MISSING after model clear: %+v", ac)
		}
		return nil
	}
}

// checkAgentReportsToNull asserts (raw API) that the agent has been reset to root.
func checkAgentReportsToNull(resourceName string) resource.TestCheckFunc {
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
		if got.ReportsTo != "" {
			return fmt.Errorf("reportsTo = %q, want empty (root) after reset", got.ReportsTo)
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
	// 依賴圖保證建立順序（root 先於 sub，secret 先於 sub.env）。The three bools toggle the
	// managed keys under test so successive steps can DROP them and prove convergence.
	config := func(model string, subChrome, subEnv, subReportsTo bool) string {
		reportsToLine := ""
		if subReportsTo {
			reportsToLine = "  reports_to     = paperclip_agent.root.id\n"
		}
		chromeLine := ""
		if subChrome {
			chromeLine = "    chrome = true\n"
		}
		envBlock := ""
		if subEnv {
			envBlock = "    env = {\n      GH_TOKEN = { secret_id = paperclip_company_secret.gh.id }\n    }\n"
		}
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
%s  desired_skills = [%q]
  adapter = {
    model  = %q
%s%s  }
}
`, companyName, model, reportsToLine, boardSkill, model, chromeLine, envBlock)
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
				Config: config(modelInitial, true, true, true),
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
			{ // update adapter.model → working merge path: paperclipSkillSync + instructions* must survive
				Config: config(modelUpdated, true, true, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("paperclip_agent.sub", "adapter.model", modelUpdated),
					checkAgentAdapterSurvives("paperclip_agent.sub", modelUpdated),
				),
			},
			{ // CLEAR chrome (scalar): drop it from adapter. The built-in post-apply idempotency
				// check FAILS if the plan doesn't converge — the direct proof of the clear fix.
				Config: config(modelUpdated, false, true, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("paperclip_agent.sub", "adapter.chrome"),
					// env survives (still declared); chrome gone; unmanaged bag intact.
					resource.TestCheckResourceAttrPair("paperclip_agent.sub", "adapter.env.GH_TOKEN.secret_id", "paperclip_company_secret.gh", "id"),
					checkAgentAdapterKeysAbsent("paperclip_agent.sub", "chrome"),
				),
			},
			{ // CLEAR env (object): drop it. Proves the object-key case null-under-merge could NOT clear.
				Config: config(modelUpdated, false, false, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("paperclip_agent.sub", "adapter.env.GH_TOKEN.secret_id"),
					checkAgentAdapterKeysAbsent("paperclip_agent.sub", "chrome", "env"),
				),
			},
			{ // RESET reports_to: drop it → subordinate returns to root (explicit JSON null).
				Config: config(modelUpdated, false, false, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("paperclip_agent.sub", "reports_to"),
					checkAgentReportsToNull("paperclip_agent.sub"),
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

// TestAccAgentResource_removeAdapterBlock is the I-2 convergence pin. Removing the
// ENTIRE adapter block (adapter = null in the plan) drives buildManagedAdapterConfig
// to {} → buildAdapterConfigPatch puts `model` (a Required-within-block key) into the
// clear-set → a replaceAdapterConfig=true PATCH with NO model. Live probe (2026-07-22)
// proved the API accepts that and the agent converges to a model-less state (matching
// Reflection Coach) while paperclipSkillSync survives. This test locks that: the
// framework's built-in post-apply idempotency check FAILS if the second step doesn't
// converge, and the read-back proves model is actually gone live.
func TestAccAgentResource_removeAdapterBlock(t *testing.T) {
	ts := time.Now().Unix()
	companyName := fmt.Sprintf("tfacc-agentblk-%d", ts)

	// withAdapter toggles the ENTIRE adapter block. desired_skills stays constant so
	// paperclipSkillSync exists and we can prove it survives the model clear.
	config := func(withAdapter bool) string {
		adapterBlock := ""
		if withAdapter {
			adapterBlock = "  adapter = {\n    model  = \"claude-sonnet-4-6\"\n    chrome = true\n  }\n"
		}
		return fmt.Sprintf(`
resource "paperclip_company" "c" {
  name = %q
}

resource "paperclip_agent" "a" {
  company_id     = paperclip_company.c.id
  name           = "Blocky"
  role           = "engineer"
  desired_skills = [%q]
%s}
`, companyName, boardSkill, adapterBlock)
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
			{ // create WITH adapter block (model+chrome) + a skill → paperclipSkillSync exists
				Config: config(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("paperclip_agent.a", "adapter.model", "claude-sonnet-4-6"),
					resource.TestCheckResourceAttr("paperclip_agent.a", "adapter.chrome", "true"),
					checkAgentAdapterSurvives("paperclip_agent.a", "claude-sonnet-4-6"),
				),
			},
			{ // REMOVE the ENTIRE adapter block. model → clear-set → replaceAdapterConfig=true
			  // with no model. Built-in idempotency check FAILS if it doesn't converge; read-back
			  // proves model gone live and paperclipSkillSync intact.
				Config: config(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("paperclip_agent.a", "adapter.model"),
					checkAgentModelClearedSkillsSurvive("paperclip_agent.a"),
				),
			},
		},
	})
}
