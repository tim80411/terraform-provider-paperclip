// internal/provider/project_workspace_resource_internal_test.go
package provider

import (
	"testing"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

// Read 用 list-then-find：workspace 沒有單獨 GET 端點，漂移偵測靠
// 「list 後以 id 尋找」——找不到即視為 live 端已刪（state 移除、計畫重建）。
func TestFindWorkspaceByID_Found(t *testing.T) {
	list := []client.Workspace{
		{ID: "w1", IsPrimary: true},
		{ID: "w2", RepoUrl: "https://github.com/o/r2"},
	}

	got, ok := findWorkspaceByID(list, "w2")
	if !ok {
		t.Fatal("expected found")
	}
	if got.RepoUrl != "https://github.com/o/r2" {
		t.Errorf("got = %+v", got)
	}
}

func TestFindWorkspaceByID_NotFound(t *testing.T) {
	list := []client.Workspace{{ID: "w1"}}

	_, ok := findWorkspaceByID(list, "ghost")
	if ok {
		t.Fatal("expected not found for missing id")
	}
}
