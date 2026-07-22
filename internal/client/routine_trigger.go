// internal/client/routine_trigger.go
// Routine triggers: POST /api/routines/{rid}/triggers + PATCH/DELETE
// /api/routine-triggers/{tid}. trigger 無單獨 GET——GET /api/routines/{id}
// 的 detail 內含 triggers 陣列（service getDetail 實證），Read 走 list-then-find。
//
// v1 只做 kind=schedule：webhook kind 含 signing secret/publicId（runtime 面）、
// api kind 是手動觸發口，都不屬宣告式設定。
package client

import "context"

type RoutineTrigger struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Label          string `json:"label,omitempty"`
	Enabled        bool   `json:"enabled"`
	CronExpression string `json:"cronExpression,omitempty"`
	Timezone       string `json:"timezone,omitempty"`
}

// RoutineTriggerCreateInput：kind 由 client 硬編 "schedule"（型別上不可能建其他 kind）。
type RoutineTriggerCreateInput struct {
	Label          string `json:"label,omitempty"`
	Enabled        *bool  `json:"enabled,omitempty"` // 未設→server default true
	CronExpression string `json:"cronExpression"`
	Timezone       string `json:"timezone,omitempty"` // 未設→server default UTC
}

type routineTriggerCreateBody struct {
	Kind string `json:"kind"`
	RoutineTriggerCreateInput
}

// RoutineTriggerUpdateInput：指標 + omitempty → nil 不進 body（partial-merge）。
type RoutineTriggerUpdateInput struct {
	Label          *string `json:"label,omitempty"`
	Enabled        *bool   `json:"enabled,omitempty"`
	CronExpression *string `json:"cronExpression,omitempty"`
	Timezone       *string `json:"timezone,omitempty"`
}

func (c *Client) CreateRoutineTrigger(ctx context.Context, routineID string, in RoutineTriggerCreateInput) (*RoutineTrigger, error) {
	var out RoutineTrigger
	body := routineTriggerCreateBody{Kind: "schedule", RoutineTriggerCreateInput: in}
	if err := c.do(ctx, "POST", "/api/routines/"+routineID+"/triggers", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateRoutineTrigger(ctx context.Context, triggerID string, in RoutineTriggerUpdateInput) (*RoutineTrigger, error) {
	var out RoutineTrigger
	if err := c.do(ctx, "PATCH", "/api/routine-triggers/"+triggerID, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteRoutineTrigger(ctx context.Context, triggerID string) error {
	return c.do(ctx, "DELETE", "/api/routine-triggers/"+triggerID, nil, nil)
}
