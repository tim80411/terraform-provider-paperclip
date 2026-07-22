// internal/provider/project_resource_test.go
package provider

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

// checkProjectGoalSet GETs the project via the raw API and asserts that the SET
// of goalIds equals the resolved ids of goalResources (order-insensitive — the
// API has its own canonical order) AND that the legacy read-only goalId is a
// MEMBER of goalIds (empty when no goals). goalId mirrors the server's "primary"
// goal, which is NOT necessarily the first element of the returned array (the
// array order is canonical, not input order) — so membership, not position, is
// the real invariant. This is the make-or-break for R5: it proves the provider
// writes goalIds correctly and that emptying it clears ALL links.
func checkProjectGoalSet(projectResource string, goalResources ...string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		pr, ok := s.RootModule().Resources[projectResource]
		if !ok {
			return fmt.Errorf("%s not in state", projectResource)
		}
		want := make([]string, 0, len(goalResources))
		for _, gr := range goalResources {
			r, ok := s.RootModule().Resources[gr]
			if !ok {
				return fmt.Errorf("%s not in state", gr)
			}
			want = append(want, r.Primary.ID)
		}
		c := client.New(os.Getenv("PAPERCLIP_API_BASE"), os.Getenv("PAPERCLIP_API_KEY"))
		got, err := c.GetProject(context.Background(), pr.Primary.ID)
		if err != nil {
			return fmt.Errorf("GetProject(%s): %w", pr.Primary.ID, err)
		}
		gotSorted := append([]string(nil), got.GoalIds...)
		wantSorted := append([]string(nil), want...)
		sort.Strings(gotSorted)
		sort.Strings(wantSorted)
		if strings.Join(gotSorted, ",") != strings.Join(wantSorted, ",") {
			return fmt.Errorf("goalIds set = %v, want %v", got.GoalIds, want)
		}
		// goalId (legacy mirror) must be empty when cleared, else a MEMBER of goalIds
		// (server picks the primary goal; not necessarily the array's first element).
		if len(got.GoalIds) == 0 {
			if got.GoalId != "" {
				return fmt.Errorf("goalId = %q, want empty when no goals linked", got.GoalId)
			}
			return nil
		}
		member := false
		for _, id := range got.GoalIds {
			if id == got.GoalId {
				member = true
				break
			}
		}
		if got.GoalId == "" || !member {
			return fmt.Errorf("goalId (legacy mirror) = %q, want a member of goalIds %v", got.GoalId, got.GoalIds)
		}
		return nil
	}
}

// checkProjectLeadCleared asserts (raw API) that leadAgentId is empty.
func checkProjectLeadCleared(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("%s not in state", resourceName)
		}
		c := client.New(os.Getenv("PAPERCLIP_API_BASE"), os.Getenv("PAPERCLIP_API_KEY"))
		got, err := c.GetProject(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("GetProject(%s): %w", rs.Primary.ID, err)
		}
		if got.LeadAgentId != "" {
			return fmt.Errorf("leadAgentId = %q, want empty (cleared)", got.LeadAgentId)
		}
		return nil
	}
}

func TestAccProjectResource_lifecycle(t *testing.T) {
	ts := time.Now().Unix()
	companyName := fmt.Sprintf("tfacc-project-%d", ts)
	repoURL := fmt.Sprintf("https://github.com/tim80411/tfacc-project-%d", ts)

	// config renders company → lead agent → 2 goals → project. goalRefs is nil for
	// "attribute absent", or a (possibly empty) slice of goal refs; leadSet toggles
	// the lead_agent_id line. workspace.repo_url is constant (changing it would
	// RequiresReplace) so successive steps exercise ONLY the in-place update path.
	config := func(name string, goalRefs []string, leadSet bool) string {
		goalIdsLine := ""
		if goalRefs != nil {
			goalIdsLine = "  goal_ids = [" + strings.Join(goalRefs, ", ") + "]\n"
		}
		leadLine := ""
		if leadSet {
			leadLine = "  lead_agent_id = paperclip_agent.lead.id\n"
		}
		return fmt.Sprintf(`
resource "paperclip_company" "c" {
  name = %q
}

resource "paperclip_agent" "lead" {
  company_id = paperclip_company.c.id
  name       = "Lead"
  role       = "engineer"
  adapter = {
    model = "claude-opus-4-8"
  }
}

resource "paperclip_goal" "g1" {
  company_id = paperclip_company.c.id
  title      = "Goal One"
}

resource "paperclip_goal" "g2" {
  company_id = paperclip_company.c.id
  title      = "Goal Two"
}

resource "paperclip_project" "p" {
  company_id  = paperclip_company.c.id
  name        = %q
  description = "acc project"
%s%s  workspace = {
    repo_url = %q
  }
}
`, companyName, name, goalIdsLine, leadLine, repoURL)
	}

	g1 := "paperclip_goal.g1.id"
	g2 := "paperclip_goal.g2.id"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		PreCheck:                 func() { preCheck(t) },
		CheckDestroy: func(s *terraform.State) error {
			c := client.New(os.Getenv("PAPERCLIP_API_BASE"), os.Getenv("PAPERCLIP_API_KEY"))
			for _, rs := range s.RootModule().Resources {
				if rs.Type != "paperclip_project" {
					continue
				}
				_, err := c.GetProject(context.Background(), rs.Primary.ID)
				if err == nil {
					return fmt.Errorf("project %s still exists after destroy", rs.Primary.ID)
				}
				if !client.IsGone(err) {
					return fmt.Errorf("unexpected error checking destroy: %w", err)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{ // create: workspace bound, goal_ids=[g1], lead set
				Config: config("Proj", []string{g1}, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("paperclip_project.p", "id"),
					resource.TestCheckResourceAttr("paperclip_project.p", "name", "Proj"),
					resource.TestCheckResourceAttr("paperclip_project.p", "workspace.repo_url", repoURL),
					resource.TestCheckResourceAttr("paperclip_project.p", "workspace.source_type", "git_repo"),
					resource.TestCheckResourceAttr("paperclip_project.p", "workspace.is_primary", "true"),
					resource.TestCheckResourceAttr("paperclip_project.p", "goal_ids.#", "1"),
					resource.TestCheckTypeSetElemAttrPair("paperclip_project.p", "goal_ids.*", "paperclip_goal.g1", "id"),
					resource.TestCheckResourceAttrPair("paperclip_project.p", "lead_agent_id", "paperclip_agent.lead", "id"),
					checkProjectGoalSet("paperclip_project.p", "paperclip_goal.g1"),
				),
			},
			{ // rename (in-place scalar update) — goal_ids/lead unchanged
				Config: config("Proj-renamed", []string{g1}, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("paperclip_project.p", "name", "Proj-renamed"),
					resource.TestCheckResourceAttr("paperclip_project.p", "goal_ids.#", "1"),
				),
			},
			{ // add a goal: goal_ids=[g1,g2] → both linked (Set: order-insensitive)
				Config: config("Proj-renamed", []string{g1, g2}, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("paperclip_project.p", "goal_ids.#", "2"),
					resource.TestCheckTypeSetElemAttrPair("paperclip_project.p", "goal_ids.*", "paperclip_goal.g1", "id"),
					resource.TestCheckTypeSetElemAttrPair("paperclip_project.p", "goal_ids.*", "paperclip_goal.g2", "id"),
					checkProjectGoalSet("paperclip_project.p", "paperclip_goal.g1", "paperclip_goal.g2"),
				),
			},
			{ // remove a goal (selective): goal_ids=[g2] → only g2 linked
				Config: config("Proj-renamed", []string{g2}, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("paperclip_project.p", "goal_ids.#", "1"),
					resource.TestCheckTypeSetElemAttrPair("paperclip_project.p", "goal_ids.*", "paperclip_goal.g2", "id"),
					checkProjectGoalSet("paperclip_project.p", "paperclip_goal.g2"),
				),
			},
			{ // EMPTY goal_ids: goal_ids=[] → all links cleared (read-back proves it).
				// The built-in post-apply idempotency check FAILS if the plan doesn't
				// converge — direct proof that emptying sends [] and clears, not no-op.
				Config: config("Proj-renamed", []string{}, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("paperclip_project.p", "goal_ids.#", "0"),
					checkProjectGoalSet("paperclip_project.p"), // no goals → goalIds [] and goalId ""
				),
			},
			{ // RESET lead_agent_id: drop it → lead cleared (explicit JSON null).
				Config: config("Proj-renamed", []string{}, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("paperclip_project.p", "lead_agent_id"),
					checkProjectLeadCleared("paperclip_project.p"),
				),
			},
			{ // import by plain id (passthrough)
				ResourceName:      "paperclip_project.p",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
