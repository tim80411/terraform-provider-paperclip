// internal/provider/company_resource_test.go
package provider

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func protoV6Factories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"paperclip": providerserver.NewProtocol6WithError(New()),
	}
}

func preCheck(t *testing.T) {
	if os.Getenv("PAPERCLIP_API_BASE") == "" || os.Getenv("PAPERCLIP_API_KEY") == "" {
		t.Skip("set PAPERCLIP_API_BASE and PAPERCLIP_API_KEY (instance-admin board token) to run acceptance tests")
	}
}

func TestAccCompanyResource_lifecycle(t *testing.T) {
	preCheck(t)
	name := fmt.Sprintf("tfacc-scratch-%d", time.Now().Unix()) // 唯一名，絕不撞 既有正式公司
	renamed := name + "-renamed"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		PreCheck:                 func() { preCheck(t) },
		Steps: []resource.TestStep{
			{ // create + read
				Config: fmt.Sprintf(`resource "paperclip_company" "s" { name = %q }`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("paperclip_company.s", "name", name),
					resource.TestCheckResourceAttrSet("paperclip_company.s", "id"),
				),
			},
			{ // update (rename) — 驗證只改 name
				Config: fmt.Sprintf(`resource "paperclip_company" "s" { name = %q }`, renamed),
				Check:  resource.TestCheckResourceAttr("paperclip_company.s", "name", renamed),
			},
			{ // import → plan no-op
				ResourceName:      "paperclip_company.s",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
