// internal/client/secret_provider_config.go
// External secret provider configs: POST /api/companies/{cid}/secret-provider-configs
// + GET/PATCH/DELETE /api/secret-provider-configs/{id}（per-ID GET 存在，不需 list-then-find）。
//
// server 實證（2026-07-22）：
//   - provider enum：local_encrypted | aws_secrets_manager | gcp_secret_manager | vault
//     （gcp/vault 目前 coming_soon 鎖定）
//   - config 是 opaque record：server 端 rejectSensitiveProviderConfigKeys（config 內
//     禁放 secret 值）+ 逐 provider payload 驗證 → 無效參數 422，client 不重複驗證。
package client

import "context"

type SecretProviderConfig struct {
	ID          string         `json:"id"`
	CompanyID   string         `json:"companyId,omitempty"`
	Provider    string         `json:"provider"`
	DisplayName string         `json:"displayName"`
	Status      string         `json:"status,omitempty"`
	IsDefault   bool           `json:"isDefault"`
	Config      map[string]any `json:"config,omitempty"`
}

type SecretProviderConfigCreateInput struct {
	Provider    string         `json:"provider"`
	DisplayName string         `json:"displayName"`
	Status      string         `json:"status,omitempty"`
	IsDefault   *bool          `json:"isDefault,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
}

// SecretProviderConfigUpdateInput：指標 + omitempty → nil 不進 body（partial-merge）。
// Config 用 *map 區分「不送」與「送空 {}」。
type SecretProviderConfigUpdateInput struct {
	DisplayName *string         `json:"displayName,omitempty"`
	Status      *string         `json:"status,omitempty"`
	IsDefault   *bool           `json:"isDefault,omitempty"`
	Config      *map[string]any `json:"config,omitempty"`
}

func (c *Client) CreateSecretProviderConfig(ctx context.Context, companyID string, in SecretProviderConfigCreateInput) (*SecretProviderConfig, error) {
	var out SecretProviderConfig
	if err := c.do(ctx, "POST", "/api/companies/"+companyID+"/secret-provider-configs", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetSecretProviderConfig(ctx context.Context, id string) (*SecretProviderConfig, error) {
	var out SecretProviderConfig
	if err := c.do(ctx, "GET", "/api/secret-provider-configs/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateSecretProviderConfig(ctx context.Context, id string, in SecretProviderConfigUpdateInput) (*SecretProviderConfig, error) {
	var out SecretProviderConfig
	if err := c.do(ctx, "PATCH", "/api/secret-provider-configs/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteSecretProviderConfig(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/secret-provider-configs/"+id, nil, nil)
}
