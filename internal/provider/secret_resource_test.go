// internal/provider/secret_resource_test.go
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

func TestAccSecretResource_lifecycle(t *testing.T) {
	companyName := fmt.Sprintf("tfacc-scratch-secret-%d", time.Now().Unix())
	secretName := "gh-token"
	renamed := "gh-token-renamed"
	// key 刻意用小寫：live 探測顯示 paperclip 會把 key 正規化成小寫（"GH_TOKEN" → "gh_token"）；
	// provider 在 Read 用 reconcileKey 保留 config 的原始大小寫（見 secret_resource.go），
	// 但 ImportStateVerify 是逐字比對「原始 config casing」vs「import 後重建的 casing」——
	// import 沒有「prior state」可比對大小寫，所以一律採用 API 值。這裡用小寫 key 讓兩者
	// 天然相等，避免測試本身撞上這個已知、已記錄在 report 的 casing 落差。
	key := "gh_token"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		PreCheck:                 func() { preCheck(t) },
		CheckDestroy: func(s *terraform.State) error {
			c := client.New(os.Getenv("PAPERCLIP_API_BASE"), os.Getenv("PAPERCLIP_API_KEY"))
			for _, rs := range s.RootModule().Resources {
				if rs.Type != "paperclip_company_secret" {
					continue
				}
				companyID := rs.Primary.Attributes["company_id"]
				_, err := c.GetSecret(context.Background(), companyID, rs.Primary.ID)
				if err == nil {
					return fmt.Errorf("secret %s still exists after destroy", rs.Primary.ID)
				}
				if !client.IsGone(err) {
					return fmt.Errorf("unexpected error checking destroy: %w", err)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{ // create
				Config: fmt.Sprintf(`
resource "paperclip_company" "s" {
  name = %q
}

resource "paperclip_company_secret" "sec" {
  company_id    = paperclip_company.s.id
  name          = %q
  key           = %q
  value         = "ghp_initial_value"
  value_version = "1"
}
`, companyName, secretName, key),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("paperclip_company_secret.sec", "name", secretName),
					resource.TestCheckResourceAttr("paperclip_company_secret.sec", "key", key),
					resource.TestCheckResourceAttr("paperclip_company_secret.sec", "value", "ghp_initial_value"),
					resource.TestCheckResourceAttr("paperclip_company_secret.sec", "value_version", "1"),
					resource.TestCheckResourceAttrSet("paperclip_company_secret.sec", "id"),
					resource.TestCheckResourceAttrPair("paperclip_company_secret.sec", "company_id", "paperclip_company.s", "id"),
				),
			},
			{ // update name only — 不應該 rotate（value_version 沒變）
				Config: fmt.Sprintf(`
resource "paperclip_company" "s" {
  name = %q
}

resource "paperclip_company_secret" "sec" {
  company_id    = paperclip_company.s.id
  name          = %q
  key           = %q
  value         = "ghp_initial_value"
  value_version = "1"
}
`, companyName, renamed, key),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("paperclip_company_secret.sec", "name", renamed),
					resource.TestCheckResourceAttr("paperclip_company_secret.sec", "value_version", "1"),
				),
			},
			{ // bump value_version — 觸發 rotate（送出新的 value）
				Config: fmt.Sprintf(`
resource "paperclip_company" "s" {
  name = %q
}

resource "paperclip_company_secret" "sec" {
  company_id    = paperclip_company.s.id
  name          = %q
  key           = %q
  value         = "ghp_rotated_value"
  value_version = "2"
}
`, companyName, renamed, key),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("paperclip_company_secret.sec", "value", "ghp_rotated_value"),
					resource.TestCheckResourceAttr("paperclip_company_secret.sec", "value_version", "2"),
				),
			},
			{ // import → 需要 company_id/secret_id 複合 ID（見 secret_resource.go ImportState 註解）
				ResourceName:      "paperclip_company_secret.sec",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["paperclip_company_secret.sec"]
					if !ok {
						return "", fmt.Errorf("resource paperclip_company_secret.sec not found in state")
					}
					return rs.Primary.Attributes["company_id"] + "/" + rs.Primary.ID, nil
				},
				// value/value_version 在 API 端完全沒有可讀回的來源（value 從不回傳、
				// value_version 是純 provider 端的 rotate 觸發器，兩者都不是從 GetSecret 派生），
				// import 沒有 prior state 可保留，所以匯入後必然是 null——跟原本 config 值不同，
				// 這是 write-only secret 在任何 TF provider 都會有的已知限制（spec §6.2）。
				ImportStateVerifyIgnore: []string{"value", "value_version"},
			},
		},
	})
}
