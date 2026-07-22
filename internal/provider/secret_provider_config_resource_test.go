// internal/provider/secret_provider_config_resource_test.go
package provider

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// local_encrypted provider（無外部依賴）驗證 create/rename/no-op（TFPC-8 情境 1）。
func TestAccSecretProviderConfigResource_lifecycle(t *testing.T) {
	name := fmt.Sprintf("tfacc-scratch-spc-%d", time.Now().Unix()) // 唯一名，絕不撞既有正式公司

	config := func(display string) string {
		return fmt.Sprintf(`
resource "paperclip_company" "s" { name = %q }

resource "paperclip_secret_provider_config" "v" {
  company_id   = paperclip_company.s.id
  provider     = "local_encrypted"
  display_name = %q
}
`, name, display)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		PreCheck:                 func() { preCheck(t) },
		Steps: []resource.TestStep{
			{
				Config: config("vault-a"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("paperclip_secret_provider_config.v", "id"),
					resource.TestCheckResourceAttr("paperclip_secret_provider_config.v", "display_name", "vault-a"),
				),
			},
			{ // 冪等
				Config:   config("vault-a"),
				PlanOnly: true,
			},
			{ // rename（PATCH partial）
				Config: config("vault-b"),
				Check:  resource.TestCheckResourceAttr("paperclip_secret_provider_config.v", "display_name", "vault-b"),
			},
		},
	})
}
