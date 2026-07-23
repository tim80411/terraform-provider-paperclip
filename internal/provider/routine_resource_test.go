// internal/provider/routine_resource_test.go
package provider

import (
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func testAccModel() string {
	if m := os.Getenv("PAPERCLIP_TEST_MODEL"); m != "" {
		return m
	}
	return "claude-sonnet-4-6" // 與 agent acceptance 同款既存慣例值
}

// scratch company + agent → routine（assignee, active）+ schedule trigger，
// 驗證建立、冪等、暫停（TFPC-5 情境 1/2）。
// live 規則：active 需要 assignee（無 assignee 會被靜默降級 paused）。
func TestAccRoutineResource_lifecycle(t *testing.T) {
	name := fmt.Sprintf("tfacc-scratch-rt-%d", time.Now().Unix()) // 唯一名，絕不撞既有正式公司

	config := func(status string) string {
		return fmt.Sprintf(`
resource "paperclip_company" "s" { name = %q }

resource "paperclip_agent" "a" {
  company_id = paperclip_company.s.id
  name       = "Routine Runner"
  role       = "ceo"
  icon       = "crown"
  adapter = {
    model = %q
  }
}

resource "paperclip_routine" "r" {
  company_id        = paperclip_company.s.id
  title             = "%s-routine"
  status            = %q
  assignee_agent_id = paperclip_agent.a.id
}

resource "paperclip_routine_trigger" "t" {
  routine_id      = paperclip_routine.r.id
  cron_expression = "0 9 * * 1-5"
  timezone        = "Asia/Taipei"
}
`, name, testAccModel(), name, status)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		PreCheck:                 func() { preCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: config("active"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("paperclip_routine.r", "status", "active"),
					resource.TestCheckResourceAttr("paperclip_routine_trigger.t", "cron_expression", "0 9 * * 1-5"),
					resource.TestCheckResourceAttr("paperclip_routine_trigger.t", "enabled", "true"),
				),
			},
			{ // 再 apply 為 no-op（冪等）
				Config:   config("active"),
				PlanOnly: true,
			},
			{ // import routine：純 uuid 直通，company_id 由 Read 從 GET detail 回填
				ResourceName:      "paperclip_routine.r",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // import trigger：無單獨 GET → 複合 "routine_id/trigger_id"
				ResourceName:      "paperclip_routine_trigger.t",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["paperclip_routine_trigger.t"]
					if !ok {
						return "", fmt.Errorf("paperclip_routine_trigger.t not found in state")
					}
					return rs.Primary.Attributes["routine_id"] + "/" + rs.Primary.ID, nil
				},
			},
			{ // 更新 status → paused
				Config: config("paused"),
				Check:  resource.TestCheckResourceAttr("paperclip_routine.r", "status", "paused"),
			},
		},
	})
}

// 無 assignee 宣告 active → plan 期就被 ValidateConfig 擋下（不打 API）。
func TestAccRoutine_activeWithoutAssigneeRejected(t *testing.T) {
	name := fmt.Sprintf("tfacc-scratch-rtna-%d", time.Now().Unix())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		PreCheck:                 func() { preCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "paperclip_company" "s" { name = %q }

resource "paperclip_routine" "r" {
  company_id = paperclip_company.s.id
  title      = "%s-routine"
  status     = "active"
}
`, name, name),
				ExpectError: regexp.MustCompile("assignee_agent_id"),
			},
		},
	})
}

// 無效 cron 格式 → server 422，apply 明確報錯（TFPC-5 情境 3）。
func TestAccRoutineTrigger_invalidCronRejected(t *testing.T) {
	name := fmt.Sprintf("tfacc-scratch-rtbad-%d", time.Now().Unix())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		PreCheck:                 func() { preCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "paperclip_company" "s" { name = %q }

resource "paperclip_routine" "r" {
  company_id = paperclip_company.s.id
  title      = "%s-routine"
}

resource "paperclip_routine_trigger" "t" {
  routine_id      = paperclip_routine.r.id
  cron_expression = "not-a-cron"
}
`, name, name),
				ExpectError: regexp.MustCompile("(?i)cron|invalid|422"),
			},
		},
	})
}
