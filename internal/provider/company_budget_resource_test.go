// internal/provider/company_budget_resource_test.go
package provider

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// scratch company 設定預算 → 讀回一致 → no-op → 調整（TFPC-9 情境 1/2）。
func TestAccCompanyBudgetResource_lifecycle(t *testing.T) {
	name := fmt.Sprintf("tfacc-scratch-bud-%d", time.Now().Unix()) // 唯一名，絕不撞既有正式公司

	config := func(cents int) string {
		return fmt.Sprintf(`
resource "paperclip_company" "s" { name = %q }

resource "paperclip_company_budget" "b" {
  company_id    = paperclip_company.s.id
  monthly_cents = %d
}
`, name, cents)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		PreCheck:                 func() { preCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: config(50000),
				Check:  resource.TestCheckResourceAttr("paperclip_company_budget.b", "monthly_cents", "50000"),
			},
			{ // 冪等
				Config:   config(50000),
				PlanOnly: true,
			},
			{ // import：singleton per company → import ID 就是 company_id（= 資源 id）
				ResourceName:      "paperclip_company_budget.b",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // 調整上限
				Config: config(120000),
				Check:  resource.TestCheckResourceAttr("paperclip_company_budget.b", "monthly_cents", "120000"),
			},
		},
	})
}
