// internal/client/budget.go
// Company budget: PATCH /api/companies/{cid}/budgets {budgetMonthlyCents}.
// server 實證（costs.ts）：寫入 companies.budgetMonthlyCents + upsert company-scope
// policy，回傳 company JSON。DB default = 0 → destroy 語意 = PATCH 0（還原預設）。
// agent 層也有同款端點（PATCH /agents/{aid}/budgets），未納入 v1。
package client

import "context"

type budgetUpdateInput struct {
	BudgetMonthlyCents int64 `json:"budgetMonthlyCents"`
}

func (c *Client) UpdateCompanyBudget(ctx context.Context, companyID string, cents int64) (*Company, error) {
	var out Company
	if err := c.do(ctx, "PATCH", "/api/companies/"+companyID+"/budgets", budgetUpdateInput{BudgetMonthlyCents: cents}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
