package client

import (
	"context"
	"encoding/json"
)

// Project is a paperclip project as returned by GET /api/projects/{id}.
//
// live 探測（2026-07-22，手動 CRUD 對 scratch company）：
//   - goalIds 是 array；goalId 是 goalIds[0] 的「唯讀鏡射」——PATCH goalIds:[g1,g2] 後
//     goalId 自動變成 g1，PATCH goalIds:[] 後 goalId 變 null。provider 一律「寫 goalIds」，
//     goalId 只讀不寫（見 ProjectCreateInput/ProjectUpdateInput 皆無 goalId 欄位）。
//   - leadAgentId 是 nullable uuid；PATCH 送明確 JSON null 可清空（read-back 確認），跟 goal 的
//     ownerAgentId/agent 的 reportsTo 同款三態。
//   - primaryWorkspace 是主要 workspace 物件（GET 同時回 .workspaces array 與 .primaryWorkspace
//     object；本 provider v1 只管 primary，用 primaryWorkspace 即可）。workspace 只能在 CREATE 時
//     inline 帶入；PATCH 內的 inline workspace 被 API 靜默忽略（實測 repoUrl 不變、無新 ws），
//     所以 resource 端 workspace 欄位設為 RequiresReplace（改 repo = 重建 project）。
//   - 已刪除的 project GET 回 404 "Project not found"（IsGone 涵蓋 404‖403）。
type Project struct {
	ID          string   `json:"id"`
	CompanyID   string   `json:"companyId"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Status      string   `json:"status,omitempty"`
	LeadAgentId string   `json:"leadAgentId,omitempty"`
	GoalIds     []string `json:"goalIds,omitempty"`
	// GoalId is the legacy singular mirror of GoalIds[0]. It is READ-ONLY: the
	// provider never writes it (Create/Update inputs carry only GoalIds). Kept
	// here only to document the API surface — the resource never maps it to state.
	GoalId           string     `json:"goalId,omitempty"`
	PrimaryWorkspace *Workspace `json:"primaryWorkspace,omitempty"`
}

// Workspace is a project workspace. The provider manages only the primary
// git_repo workspace and only these three fields; the API carries many more
// (cwd, repoRef, visibility, setupCommand, …) that are left untouched.
type Workspace struct {
	ID         string `json:"id,omitempty"`
	SourceType string `json:"sourceType,omitempty"`
	RepoUrl    string `json:"repoUrl,omitempty"`
	Name       string `json:"name,omitempty"`
	IsPrimary  bool   `json:"isPrimary,omitempty"`
}

// ProjectCreateInput is the POST /api/companies/{cid}/projects body. Plain
// values + omitempty; the inline Workspace is create-time only (see Project doc).
// GoalIds (array) is written; goalId (legacy mirror) is never sent.
type ProjectCreateInput struct {
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	Status      string                `json:"status,omitempty"`
	LeadAgentId string                `json:"leadAgentId,omitempty"`
	GoalIds     []string              `json:"goalIds,omitempty"`
	Workspace   *WorkspaceCreateInput `json:"workspace,omitempty"`
}

// WorkspaceCreateInput is the inline `workspace` object accepted only at create.
// Name is omitted by this provider (server derives it from repoUrl — live proven
// by project-etl.sh, which never sends a workspace name).
type WorkspaceCreateInput struct {
	SourceType string `json:"sourceType"`
	RepoUrl    string `json:"repoUrl"`
	Name       string `json:"name,omitempty"`
	IsPrimary  bool   `json:"isPrimary"`
}

// ProjectUpdateInput 指標/RawMessage + omitempty：nil 欄位不進 JSON body → partial-merge 保留
// 未管欄位（spec §6.3）。三個 ref-shaped 欄位的清空語意各自不同（都是 live 實證）：
//
//   - LeadAgentId 是 json.RawMessage（三態）：nil→不送(保留)；"null"→送 JSON null(清空)；
//     `"<uuid>"`→送字串(指定)。跟 goal.ownerAgentId / agent.reportsTo 同一手法。
//   - GoalIds 是 *[]string（不是 []string）：nil pointer→不送(保留)；&[]→送空 array [](清空
//     所有連結，live 實證 PATCH goalIds:[] → goalId=null,goalIds=[])；&[a,b]→送 array(設定)。
//     用指標才能區分「不送」與「送空 []」——`[]string + omitempty` 會把空 slice 也 omit 掉，
//     就無法送出 [] 來清空。
//
// 沒有 Workspace 欄位：workspace 是 RequiresReplace（inline PATCH 被 API 忽略），永不 PATCH。
type ProjectUpdateInput struct {
	Name        *string         `json:"name,omitempty"`
	Description *string         `json:"description,omitempty"`
	Status      *string         `json:"status,omitempty"`
	LeadAgentId json.RawMessage `json:"leadAgentId,omitempty"`
	GoalIds     *[]string       `json:"goalIds,omitempty"`
}

func (c *Client) CreateProject(ctx context.Context, companyID string, in ProjectCreateInput) (*Project, error) {
	var out Project
	if err := c.do(ctx, "POST", "/api/companies/"+companyID+"/projects", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetProject reads a single project. live 探測：GET /api/projects/{id} 可獨立運作（不需 company
// id），已刪除的 project 回 404 "Project not found"（用 IsGone 判定）。
func (c *Client) GetProject(ctx context.Context, id string) (*Project, error) {
	var out Project
	if err := c.do(ctx, "GET", "/api/projects/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateProject(ctx context.Context, id string, in ProjectUpdateInput) (*Project, error) {
	var out Project
	if err := c.do(ctx, "PATCH", "/api/projects/"+id, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteProject(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/projects/"+id, nil, nil)
}
