package client

import (
	"context"
	"fmt"
	"net/http"
)

type Secret struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Key         string `json:"key"`
	ManagedMode string `json:"managedMode"`
}

type SecretCreateInput struct {
	Name        string `json:"name"`
	Key         string `json:"key,omitempty"`
	Value       string `json:"value,omitempty"`
	ManagedMode string `json:"managedMode,omitempty"`
}

// SecretUpdateInput 指標 + omitempty：nil 欄位不進 JSON body → partial-merge 保留（spec §6.3）。
// 沒有 Value 欄位——live 探測 PATCH /api/secrets/{id} 不接受 value，改走 RotateSecret。
type SecretUpdateInput struct {
	Name *string `json:"name,omitempty"`
	Key  *string `json:"key,omitempty"`
}

func (c *Client) CreateSecret(ctx context.Context, companyID string, in SecretCreateInput) (*Secret, error) {
	var out Secret
	if err := c.do(ctx, "POST", "/api/companies/"+companyID+"/secrets", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetSecret 沒有對應的單筆 GET：live 探測 GET /api/secrets/{id} 回 404 "API route not found"
// （openapi.json 也印證只有 PATCH/DELETE，沒有 GET）。改用 list-under-company 端點 + id 過濾。
// 找不到時合成一個 404 APIError，讓呼叫端可以照常用 IsNotFound/IsGone 判斷。
func (c *Client) GetSecret(ctx context.Context, companyID, id string) (*Secret, error) {
	path := "/api/companies/" + companyID + "/secrets"
	var out []Secret
	if err := c.do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].ID == id {
			return &out[i], nil
		}
	}
	return nil, &APIError{
		StatusCode: http.StatusNotFound,
		Method:     "GET",
		Path:       path,
		Body:       fmt.Sprintf("secret %q not found in company %q secret list", id, companyID),
	}
}

func (c *Client) UpdateSecret(ctx context.Context, id string, in SecretUpdateInput) (*Secret, error) {
	var out Secret
	if err := c.do(ctx, "PATCH", "/api/secrets/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteSecret(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/secrets/"+id, nil, nil)
}

type secretRotateInput struct {
	Value string `json:"value"`
}

func (c *Client) RotateSecret(ctx context.Context, id, value string) (*Secret, error) {
	var out Secret
	if err := c.do(ctx, "POST", "/api/secrets/"+id+"/rotate", secretRotateInput{Value: value}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
