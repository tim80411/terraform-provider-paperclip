# terraform-provider-paperclip

宣告式管理 paperclip 公司內容（見 `docs/superpowers/specs/2026-07-21-paperclip-terraform-provider-design.md`）。

## 開發安裝（dev_overrides）

```bash
make install   # build 並複製到 ~/go/bin/
```

`~/.terraformrc`：

```hcl
provider_installation {
  dev_overrides {
    "registry.terraform.io/tim80411/paperclip" = "/Users/<you>/go/bin"
  }
  direct {}
}
```

## 執行

```bash
export PAPERCLIP_API_BASE=https://paperclip.example.com   # 你的 Paperclip API base
export PAPERCLIP_API_KEY="$(cat ~/.config/paperclip.token)"  # instance-admin board token
cd examples/smoke && terraform plan
```

## 測試

- 單元：`make test`
- 驗收（會對 live 建/刪 scratch 公司，**不碰 既有正式公司**）：`make testacc`

## vendored spec

`openapi.json` 是 `GET /api/openapi.json` 的 pin 副本（OpenAPI 3.0.0，title "Paperclip API"）。
更新：重抓後 `git diff openapi.json` 檢視合約變動再升級 resource。
