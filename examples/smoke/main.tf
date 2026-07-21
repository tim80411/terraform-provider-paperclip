# examples/smoke/main.tf
terraform {
  required_providers {
    paperclip = {
      source = "registry.terraform.io/tim80411/paperclip"
    }
  }
}

provider "paperclip" {
  # api_base / api_key 由 env PAPERCLIP_API_BASE / PAPERCLIP_API_KEY 提供
}

resource "paperclip_company" "scratch" {
  name = "tfacc-smoke"

  lifecycle {
    prevent_destroy = true # spec §7 安全網示範
  }
}
