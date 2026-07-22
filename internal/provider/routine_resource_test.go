// internal/provider/routine_resource_test.go
package provider

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// scratch company → routine + schedule trigger，驗證建立、冪等、暫停（TFPC-5 情境 1/2）。
func TestAccRoutineResource_lifecycle(t *testing.T) {
	name := fmt.Sprintf("tfacc-scratch-rt-%d", time.Now().Unix()) // 唯一名，絕不撞既有正式公司

	config := func(status string) string {
		return fmt.Sprintf(`
resource "paperclip_company" "s" { name = %q }

resource "paperclip_routine" "r" {
  company_id = paperclip_company.s.id
  title      = "%s-routine"
  status     = %q
}

resource "paperclip_routine_trigger" "t" {
  routine_id      = paperclip_routine.r.id
  cron_expression = "0 9 * * 1-5"
  timezone        = "Asia/Taipei"
}
`, name, name, status)
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
			{ // 更新 status → paused
				Config: config("paused"),
				Check:  resource.TestCheckResourceAttr("paperclip_routine.r", "status", "paused"),
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
