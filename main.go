// main.go
package main

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/tim80411/terraform-provider-paperclip/internal/provider"
)

func main() {
	err := providerserver.Serve(context.Background(), provider.New, providerserver.ServeOpts{
		Address: "registry.terraform.io/tim80411/paperclip",
	})
	if err != nil {
		log.Fatal(err)
	}
}
