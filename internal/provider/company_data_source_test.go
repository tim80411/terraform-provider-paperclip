// internal/provider/company_data_source_test.go
package provider

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// 建 scratch 公司後用 data source 以 name 反查，驗證解析到同一個 id（TFPC-1 情境 1）。
func TestAccCompanyDataSource_byName(t *testing.T) {
	name := fmt.Sprintf("tfacc-scratch-ds-%d", time.Now().Unix()) // 唯一名，絕不撞既有正式公司

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		PreCheck:                 func() { preCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "paperclip_company" "s" { name = %q }

data "paperclip_company" "by_name" {
  name       = paperclip_company.s.name
  depends_on = [paperclip_company.s]
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.paperclip_company.by_name", "id", "paperclip_company.s", "id"),
					resource.TestCheckResourceAttr("data.paperclip_company.by_name", "name", name),
				),
			},
		},
	})
}

// 查不存在的名字必須明確報錯且錯誤含查詢名（TFPC-1 情境 2）。
func TestAccCompanyDataSource_notFound(t *testing.T) {
	ghost := fmt.Sprintf("tfacc-ghost-%d", time.Now().Unix())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		PreCheck:                 func() { preCheck(t) },
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(`data "paperclip_company" "ghost" { name = %q }`, ghost),
				ExpectError: regexp.MustCompile(regexp.QuoteMeta(ghost)),
			},
		},
	})
}
