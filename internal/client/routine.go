// internal/client/routine.go
// Routines over POST /api/companies/{cid}/routines + GET/PATCH /api/routines/{id}.
//
// server 原始碼實證（2026-07-22）：
//   - 無 DELETE 端點；ROUTINE_STATUSES = active|paused|archived → destroy = PATCH
//     status=archived（archive 即刪除，terminal state）。
//   - updateRoutineSchema = createRoutineSchema.partial() → PATCH 是 partial-merge，
//     provider 只送自己管的欄位（spec §6.3 通則）。
//   - assigneeAgentId nullable → 三態 RawMessage（同 project.leadAgentId 手法）。
//   - v1 不管的欄位：priority/concurrencyPolicy/catchUpPolicy/variables/env/
//     projectId/goalId/folderId/parentIssueId——partial PATCH 保留它們。
package client

import (
	"context"
	"encoding/json"
)

type Routine struct {
	ID              string `json:"id"`
	CompanyID       string `json:"companyId"`
	Title           string `json:"title"`
	Description     string `json:"description,omitempty"`
	Status          string `json:"status,omitempty"`
	AssigneeAgentId string `json:"assigneeAgentId,omitempty"`
}

type RoutineCreateInput struct {
	Title           string `json:"title"`
	Description     string `json:"description,omitempty"`
	Status          string `json:"status,omitempty"`
	AssigneeAgentId string `json:"assigneeAgentId,omitempty"`
}

// RoutineUpdateInput：指標/RawMessage + omitempty → nil 不進 body（partial-merge 保留）。
type RoutineUpdateInput struct {
	Title           *string         `json:"title,omitempty"`
	Description     *string         `json:"description,omitempty"`
	Status          *string         `json:"status,omitempty"`
	AssigneeAgentId json.RawMessage `json:"assigneeAgentId,omitempty"`
}

func (c *Client) CreateRoutine(ctx context.Context, companyID string, in RoutineCreateInput) (*Routine, error) {
	var out Routine
	if err := c.do(ctx, "POST", "/api/companies/"+companyID+"/routines", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetRoutine(ctx context.Context, id string) (*Routine, error) {
	var out Routine
	if err := c.do(ctx, "GET", "/api/routines/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateRoutine(ctx context.Context, id string, in RoutineUpdateInput) (*Routine, error) {
	var out Routine
	if err := c.do(ctx, "PATCH", "/api/routines/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ArchiveRoutine is the destroy path: body 只帶 status（partial-merge 不動其他欄位）。
func (c *Client) ArchiveRoutine(ctx context.Context, id string) error {
	archived := "archived"
	_, err := c.UpdateRoutine(ctx, id, RoutineUpdateInput{Status: &archived})
	return err
}
