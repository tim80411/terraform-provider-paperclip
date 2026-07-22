// internal/provider/goal_resource_test.go
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

func TestAccGoalResource_lifecycle(t *testing.T) {
	ts := time.Now().Unix()
	companyName := fmt.Sprintf("tfacc-goal-%d", ts)

	// config renders company → owner agent → parent goal → child goal. The
	// dependency graph guarantees create order (parent before child) with NO
	// custom ordering code needed. `parented` toggles child.parent_id so a
	// later step can drop it and prove removal=clear converges.
	config := func(childStatus string, parented bool) string {
		parentLine := ""
		if parented {
			parentLine = "  parent_id  = paperclip_goal.parent.id\n"
		}
		return fmt.Sprintf(`
resource "paperclip_company" "c" {
  name = %q
}

resource "paperclip_agent" "owner" {
  company_id = paperclip_company.c.id
  name       = "Owner"
  role       = "engineer"
  adapter = {
    model = "claude-opus-4-8"
  }
}

resource "paperclip_goal" "parent" {
  company_id  = paperclip_company.c.id
  title       = "Parent Goal"
  description = "top level"
  level       = "company"
  status      = "active"
}

resource "paperclip_goal" "child" {
  company_id     = paperclip_company.c.id
  title          = "Child Goal"
  description    = "child"
  level          = "task"
  status         = %q
  owner_agent_id = paperclip_agent.owner.id
%s}
`, companyName, childStatus, parentLine)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		PreCheck:                 func() { preCheck(t) },
		CheckDestroy: func(s *terraform.State) error {
			c := client.New(os.Getenv("PAPERCLIP_API_BASE"), os.Getenv("PAPERCLIP_API_KEY"))
			for _, rs := range s.RootModule().Resources {
				if rs.Type != "paperclip_goal" {
					continue
				}
				_, err := c.GetGoal(context.Background(), rs.Primary.ID)
				if err == nil {
					return fmt.Errorf("goal %s still exists after destroy", rs.Primary.ID)
				}
				if !client.IsGone(err) {
					return fmt.Errorf("unexpected error checking destroy: %w", err)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{ // create: parent + child (child.parent_id = parent.id, child.owner_agent_id = agent.id)
				Config: config("planned", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("paperclip_goal.parent", "id"),
					resource.TestCheckResourceAttr("paperclip_goal.parent", "level", "company"),
					resource.TestCheckResourceAttr("paperclip_goal.parent", "status", "active"),
					resource.TestCheckNoResourceAttr("paperclip_goal.parent", "parent_id"),
					resource.TestCheckResourceAttr("paperclip_goal.child", "status", "planned"),
					resource.TestCheckResourceAttrPair("paperclip_goal.child", "parent_id", "paperclip_goal.parent", "id"),
					resource.TestCheckResourceAttrPair("paperclip_goal.child", "owner_agent_id", "paperclip_agent.owner", "id"),
				),
			},
			{ // change child status
				Config: config("active", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("paperclip_goal.child", "status", "active"),
					resource.TestCheckResourceAttrPair("paperclip_goal.child", "parent_id", "paperclip_goal.parent", "id"),
				),
			},
			{ // RESET parent_id to null: drop it from config. The framework's built-in
				// post-apply idempotency check FAILS if the plan doesn't converge — the
				// direct proof that removal=clear works for parent_id (self-referential ref).
				Config: config("active", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("paperclip_goal.child", "parent_id"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["paperclip_goal.child"]
						if !ok {
							return fmt.Errorf("paperclip_goal.child not in state")
						}
						c := client.New(os.Getenv("PAPERCLIP_API_BASE"), os.Getenv("PAPERCLIP_API_KEY"))
						got, err := c.GetGoal(context.Background(), rs.Primary.ID)
						if err != nil {
							return fmt.Errorf("GetGoal(%s): %w", rs.Primary.ID, err)
						}
						if got.ParentId != "" {
							return fmt.Errorf("parentId = %q, want cleared (empty) after reset", got.ParentId)
						}
						if got.OwnerAgentId == "" {
							return fmt.Errorf("ownerAgentId unexpectedly cleared — only parent_id was reset")
						}
						return nil
					},
				),
			},
			{ // import child by plain id (passthrough)
				ResourceName:      "paperclip_goal.child",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccGoalResource_omitEnumsThenUpdate is the I-1 regression pin. A goal that
// OMITS both level and status (relying on the API defaults task/planned, live-confirmed
// 2026-07-22), then an in-place update of ANOTHER field (description). Without
// UseStateForUnknown on level/status, the omitted Optional+Computed enums go UNKNOWN on
// the second plan → buildGoalUpdateInput serializes "level":""/"status":"" (the *string
// omitempty can't drop a pointer-to-"") → API 400 invalid_enum. With the plan modifier
// the enums are held from prior state, so the update carries only description and the
// framework's post-apply idempotency check passes. This is the same guard project.status
// already had; the goal resource lacked it.
func TestAccGoalResource_omitEnumsThenUpdate(t *testing.T) {
	ts := time.Now().Unix()
	companyName := fmt.Sprintf("tfacc-goalenum-%d", ts)

	config := func(description string) string {
		return fmt.Sprintf(`
resource "paperclip_company" "c" {
  name = %q
}

resource "paperclip_goal" "g" {
  company_id  = paperclip_company.c.id
  title       = "OKR draft"
  description = %q
}
`, companyName, description)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		PreCheck:                 func() { preCheck(t) },
		CheckDestroy: func(s *terraform.State) error {
			c := client.New(os.Getenv("PAPERCLIP_API_BASE"), os.Getenv("PAPERCLIP_API_KEY"))
			for _, rs := range s.RootModule().Resources {
				if rs.Type != "paperclip_goal" {
					continue
				}
				_, err := c.GetGoal(context.Background(), rs.Primary.ID)
				if err == nil {
					return fmt.Errorf("goal %s still exists after destroy", rs.Primary.ID)
				}
				if !client.IsGone(err) {
					return fmt.Errorf("unexpected error checking destroy: %w", err)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{ // create OMITTING level+status → API defaults land in state (task/planned)
				Config: config("first"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("paperclip_goal.g", "level", "task"),
					resource.TestCheckResourceAttr("paperclip_goal.g", "status", "planned"),
				),
			},
			{ // in-place update of description ONLY. Without the fix this 400s (level/status
			  // become unknown → sent as ""); with it, the enums hold from state and the plan
			  // converges (the built-in post-apply idempotency check is the regression proof).
				Config: config("second"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("paperclip_goal.g", "description", "second"),
					resource.TestCheckResourceAttr("paperclip_goal.g", "level", "task"),
					resource.TestCheckResourceAttr("paperclip_goal.g", "status", "planned"),
				),
			},
		},
	})
}
