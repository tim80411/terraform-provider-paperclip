package client

import "context"

type Company struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Slug        string `json:"slug,omitempty"`
}

type CompanyCreateInput struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Slug        string `json:"slug,omitempty"`
}

// 指標 + omitempty：nil 欄位不進 JSON body → API partial-merge 保留（spec §6.3）
type CompanyUpdateInput struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Slug        *string `json:"slug,omitempty"`
}

func (c *Client) CreateCompany(ctx context.Context, in CompanyCreateInput) (*Company, error) {
	var out Company
	if err := c.do(ctx, "POST", "/api/companies", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetCompany(ctx context.Context, id string) (*Company, error) {
	var out Company
	if err := c.do(ctx, "GET", "/api/companies/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateCompany(ctx context.Context, id string, in CompanyUpdateInput) (*Company, error) {
	var out Company
	if err := c.do(ctx, "PATCH", "/api/companies/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteCompany(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/companies/"+id, nil, nil)
}
