// internal/client/project_workspace_test.go
package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// POST /api/projects/{id}/workspaces：v1 只建非-primary workspace，body 絕不帶
// isPrimary（server 端 isPrimary=true 會「轉移 primary」——降級既有 primary，
// 這不是本 resource 的守備範圍）。
func TestCreateProjectWorkspace_PostsBodyWithoutIsPrimary(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"w1","sourceType":"git_repo","repoUrl":"https://github.com/o/r","name":"r","isPrimary":false}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	out, err := c.CreateProjectWorkspace(context.Background(), "p1", ProjectWorkspaceCreateInput{
		RepoUrl: "https://github.com/o/r",
		Name:    "r",
	})
	if err != nil {
		t.Fatalf("CreateProjectWorkspace error: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/api/projects/p1/workspaces" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if _, has := gotBody["isPrimary"]; has {
		t.Error("body must not contain isPrimary (primary transfer is out of scope)")
	}
	if gotBody["repoUrl"] != "https://github.com/o/r" {
		t.Errorf("body repoUrl = %v", gotBody["repoUrl"])
	}
	if out.ID != "w1" || out.SourceType != "git_repo" || out.IsPrimary {
		t.Errorf("out = %+v", out)
	}
}

func TestListProjectWorkspaces_ParsesArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/projects/p1/workspaces" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`[{"id":"w1","isPrimary":true},{"id":"w2","repoUrl":"https://github.com/o/r2"}]`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	out, err := c.ListProjectWorkspaces(context.Background(), "p1")
	if err != nil {
		t.Fatalf("ListProjectWorkspaces error: %v", err)
	}
	if len(out) != 2 || out[0].ID != "w1" || !out[0].IsPrimary || out[1].RepoUrl != "https://github.com/o/r2" {
		t.Errorf("out = %+v", out)
	}
}

func TestDeleteProjectWorkspace_SendsDelete(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"w2"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if err := c.DeleteProjectWorkspace(context.Background(), "p1", "w2"); err != nil {
		t.Fatalf("DeleteProjectWorkspace error: %v", err)
	}
	if gotMethod != "DELETE" || gotPath != "/api/projects/p1/workspaces/w2" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
}
