package client

import (
	"context"
	"encoding/json"
)

// Goal is a paperclip goal (an OKR-tree node) as returned by GET /api/goals/{id}.
//
// live 探測（2026-07-22，openapi.json + 手動 CRUD 對 scratch company）：
//   - level enum：company/team/agent/task；Create 省略時 API 預設 "task"。
//   - status enum：planned/active/achieved/cancelled；Create 省略時 API 預設 "planned"。
//   - parentId/ownerAgentId 皆為 nullable uuid；PATCH 送明確 JSON null 可清空兩者（GET 回讀
//     確認），跟 agent 的 reportsTo 同款——且不像 agent 的 adapterConfig.env，這裡沒有「null
//     對物件欄位無效」的例外，parentId/ownerAgentId 都乾淨地清空。
//   - GET /api/goals/{id} 可獨立運作（不需 company id），跟 agent 同款，不像 secret 得靠
//     company 底下的 list 端點；已刪除的 goal 回 404 "Goal not found"（不像 company 是 403,
//     但 IsGone 本就涵蓋兩者）。
type Goal struct {
	ID           string `json:"id"`
	CompanyID    string `json:"companyId"`
	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	Level        string `json:"level,omitempty"`
	Status       string `json:"status,omitempty"`
	OwnerAgentId string `json:"ownerAgentId,omitempty"`
	ParentId     string `json:"parentId,omitempty"`
}

// GoalCreateInput is the POST /api/companies/{cid}/goals body. Plain values +
// omitempty（沒有 tri-state 需求——一個還不存在的 goal 沒有「清空」可言，只有「省略」或
// 「設定」，跟 omitempty 的語意天然吻合）。live 探測：title 是唯一必填欄位（minLength 1），
// 其餘省略時 API 自己給預設。
type GoalCreateInput struct {
	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	Level        string `json:"level,omitempty"`
	Status       string `json:"status,omitempty"`
	OwnerAgentId string `json:"ownerAgentId,omitempty"`
	ParentId     string `json:"parentId,omitempty"`
}

// GoalUpdateInput 指標 + omitempty：nil 欄位不進 JSON body → partial-merge 保留（spec §6.3）。
//
// OwnerAgentId/ParentId 是 json.RawMessage（不是 *string）——跟 agent 的 reportsTo 同一手法，
// 三態：
//   - nil（omitempty 吃掉）→ 不送 → 保留現況
//   - json.RawMessage("null") → 送出 JSON null → 清空（live 實證 2026-07-22）
//   - json.RawMessage(`"<uuid>"`) → 送出字串 → 指定 parent/owner
//
// `*string + omitempty` 只能表達「省略」與「值」兩態，無法送出 JSON null，所以無法把已設定的
// 參照清成 null。改用 RawMessage 才能明確送 null（這正是 removal=clear 政策要求的機制）。
type GoalUpdateInput struct {
	Title        *string         `json:"title,omitempty"`
	Description  *string         `json:"description,omitempty"`
	Level        *string         `json:"level,omitempty"`
	Status       *string         `json:"status,omitempty"`
	OwnerAgentId json.RawMessage `json:"ownerAgentId,omitempty"`
	ParentId     json.RawMessage `json:"parentId,omitempty"`
}

func (c *Client) CreateGoal(ctx context.Context, companyID string, in GoalCreateInput) (*Goal, error) {
	var out Goal
	if err := c.do(ctx, "POST", "/api/companies/"+companyID+"/goals", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetGoal reads a single goal. live 探測：GET /api/goals/{id} 可獨立運作（不需 company id）；
// 已刪除的 goal 回 404 "Goal not found"（用 IsGone 判定）。
func (c *Client) GetGoal(ctx context.Context, id string) (*Goal, error) {
	var out Goal
	if err := c.do(ctx, "GET", "/api/goals/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateGoal(ctx context.Context, id string, in GoalUpdateInput) (*Goal, error) {
	var out Goal
	if err := c.do(ctx, "PATCH", "/api/goals/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteGoal(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/goals/"+id, nil, nil)
}
