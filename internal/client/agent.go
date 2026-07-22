package client

import (
	"context"
	"encoding/json"
)

// Agent is the paperclip agent as returned by GET /api/agents/{id}.
//
// live 探測（2026-07-21）：
//   - capabilities 是「字串」不是陣列（CEO=null，engineer 是一段中文說明）。
//   - reportsTo 是 nullable uuid（根 agent 為 null）。
//   - adapterConfig 是 opaque bag：provider 只管 model/engine/chrome/env，其餘 key
//     （paperclipSkillSync、instructions*、graceSec、dangerouslySkipPermissions、未知 key…）
//     由 paperclip 自己維護，Update 時必須原封保留 → 用 map[string]any 承接，靠
//     MergeAdapterConfig 只疊管理的 key。
type Agent struct {
	ID            string         `json:"id"`
	CompanyID     string         `json:"companyId"`
	Name          string         `json:"name"`
	Role          string         `json:"role,omitempty"`
	Title         string         `json:"title,omitempty"`
	Icon          string         `json:"icon,omitempty"`
	Capabilities  string         `json:"capabilities,omitempty"`
	ReportsTo     string         `json:"reportsTo,omitempty"`
	AdapterType   string         `json:"adapterType,omitempty"`
	Status        string         `json:"status,omitempty"`
	AdapterConfig map[string]any `json:"adapterConfig,omitempty"`
}

// AgentCreateInput is the POST /api/companies/{cid}/agents body.
// ReportsTo is a pointer so a root agent (nil) omits the field entirely rather
// than sending an empty string. DesiredSkills is accepted at create-time by the
// API (validated against company skills) — but this provider syncs skills via
// SyncAgentSkills after create for a uniform create/update path, so the resource
// leaves it nil; the field stays here to mirror the API surface (see agent_test.go).
type AgentCreateInput struct {
	Name          string         `json:"name"`
	Role          string         `json:"role,omitempty"`
	Title         string         `json:"title,omitempty"`
	Icon          string         `json:"icon,omitempty"`
	Capabilities  string         `json:"capabilities,omitempty"`
	ReportsTo     *string        `json:"reportsTo,omitempty"`
	DesiredSkills []string       `json:"desiredSkills,omitempty"`
	AdapterType   string         `json:"adapterType,omitempty"`
	AdapterConfig map[string]any `json:"adapterConfig,omitempty"`
}

// AgentUpdateInput 指標 + omitempty：nil 欄位不進 JSON body → API partial-merge 保留
// 未管欄位（spec §6.3）。
//
// ReplaceAdapterConfig 是 *bool（不是 bool）——因為 false 是我們要「明確送出」的值
// （告訴 server 做 shallow-merge 而非整包替換），若用 `bool + omitempty`，false 會被
// omitempty 吃掉、變成「不送」，server 就會退回預設行為。用指標才能區分「沒設」與「設 false」。
//
// ReportsTo 是 json.RawMessage（不是 *string）——因為它需要「三態」：
//   - nil（omitempty 吃掉）→ 不送 → 保留現況
//   - json.RawMessage("null") → 送出 JSON null → agent 回到根（live 實證 2026-07-22）
//   - json.RawMessage(`"<uuid>"`) → 送出字串 → 指定上級
//
// `*string + omitempty` 只能表達「省略」與「值」兩態，無法送出 JSON null，所以無法把
// 已設定的上級清成根。改用 RawMessage 才能明確送 null。
type AgentUpdateInput struct {
	Name                 *string         `json:"name,omitempty"`
	Role                 *string         `json:"role,omitempty"`
	Title                *string         `json:"title,omitempty"`
	Icon                 *string         `json:"icon,omitempty"`
	Capabilities         *string         `json:"capabilities,omitempty"`
	ReportsTo            json.RawMessage `json:"reportsTo,omitempty"`
	AdapterConfig        map[string]any  `json:"adapterConfig,omitempty"`
	ReplaceAdapterConfig *bool           `json:"replaceAdapterConfig,omitempty"`
}

// managedAdapterConfigKeys are the ONLY adapterConfig keys this provider owns.
// MergeAdapterConfig overlays strictly these — never paperclipSkillSync,
// instructions*, or any server-owned/unknown key.
var managedAdapterConfigKeys = []string{"model", "engine", "chrome", "env"}

// MergeAdapterConfig overlays ONLY the provider-managed keys from `managed`
// onto a fresh copy of `current`, preserving every other key that paperclip
// owns (paperclipSkillSync, instructions*, dangerouslySkipPermissions, and any
// unknown future key). It never mutates its inputs.
//
// This is the crux of the resource's Update path: the API's adapterConfig is an
// opaque bag; a naive PATCH that replaced the whole bag would silently wipe the
// skill-sync and instruction pointers paperclip injects. The caller GETs the
// current bag, calls this to overlay just its four keys, then PATCHes the result
// with replaceAdapterConfig=false. Restricting the overlay to the fixed managed
// set (rather than blindly copying every key in `managed`) means a stray key can
// never clobber a server-owned one.
func MergeAdapterConfig(current, managed map[string]any) map[string]any {
	out := make(map[string]any, len(current)+len(managedAdapterConfigKeys))
	for k, v := range current {
		out[k] = v
	}
	for _, k := range managedAdapterConfigKeys {
		if v, ok := managed[k]; ok {
			out[k] = v
		}
	}
	return out
}

func (c *Client) CreateAgent(ctx context.Context, companyID string, in AgentCreateInput) (*Agent, error) {
	var out Agent
	if err := c.do(ctx, "POST", "/api/companies/"+companyID+"/agents", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetAgent reads a single agent. live 探測：GET /api/agents/{id} 可獨立運作（不需 company id），
// 已刪除的 agent 回 404 "Agent not found"（用 IsGone 判定）。
func (c *Client) GetAgent(ctx context.Context, id string) (*Agent, error) {
	var out Agent
	if err := c.do(ctx, "GET", "/api/agents/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateAgent(ctx context.Context, id string, in AgentUpdateInput) (*Agent, error) {
	var out Agent
	if err := c.do(ctx, "PATCH", "/api/agents/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteAgent(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/agents/"+id, nil, nil)
}

type agentSkillsSyncInput struct {
	// 一定要初始化成 []string{}（非 nil），否則空清單會序列化成 null 而非 []。
	DesiredSkills []string `json:"desiredSkills"`
}

// SyncAgentSkills sets an agent's desired skills via POST /api/agents/{id}/skills/sync.
// live 探測：寫入 adapterConfig.paperclipSkillSync.desiredSkills；skill 參照必須存在於 company
// （未知參照回 422）；空清單合法（清空）。
func (c *Client) SyncAgentSkills(ctx context.Context, id string, skills []string) error {
	if skills == nil {
		skills = []string{}
	}
	return c.do(ctx, "POST", "/api/agents/"+id+"/skills/sync", agentSkillsSyncInput{DesiredSkills: skills}, nil)
}
