// internal/client/project_workspace.go
// Non-primary project workspaces over /api/projects/{id}/workspaces.
// API 面（server route 實證）：GET list / POST create / DELETE remove——沒有
// PATCH，workspace 建後不可改，resource 端全欄位 RequiresReplace。
package client

import "context"

// ProjectWorkspaceCreateInput is the POST /api/projects/{id}/workspaces body.
// 刻意沒有 isPrimary 欄位（型別上就不可能表達 primary 轉移）：server 端
// isPrimary=true 會降級既有 primary workspace，那是 paperclip_project inline
// workspace（WorkspaceCreateInput）的守備範圍。
type ProjectWorkspaceCreateInput struct {
	RepoUrl string `json:"repoUrl"`
	Name    string `json:"name,omitempty"`
}

func (c *Client) CreateProjectWorkspace(ctx context.Context, projectID string, in ProjectWorkspaceCreateInput) (*Workspace, error) {
	var out Workspace
	if err := c.do(ctx, "POST", "/api/projects/"+projectID+"/workspaces", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListProjectWorkspaces(ctx context.Context, projectID string) ([]Workspace, error) {
	var out []Workspace
	if err := c.do(ctx, "GET", "/api/projects/"+projectID+"/workspaces", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) DeleteProjectWorkspace(ctx context.Context, projectID, workspaceID string) error {
	return c.do(ctx, "DELETE", "/api/projects/"+projectID+"/workspaces/"+workspaceID, nil, nil)
}
