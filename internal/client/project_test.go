package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateProject_SendsFieldsAndParsesResponse(t *testing.T) {
	// live 探測（2026-07-22）：POST 帶 inline workspace + goalIds(array)；回應含 primaryWorkspace
	// (object) 與 goalId(=goalIds[0] 的鏡射)。這裡驗證：送出 goalIds(array，不是 goalId)、送出
	// 巢狀 workspace、並正確解析回應的 primaryWorkspace + goalIds。
	var raw map[string]json.RawMessage
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &raw)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{
			"id":"p1","companyId":"co1","name":"Proj","description":"d","status":"planned",
			"leadAgentId":"a1","goalId":"g1","goalIds":["g1"],
			"primaryWorkspace":{"id":"w1","sourceType":"git_repo","repoUrl":"https://github.com/o/r","name":"r","isPrimary":true},
			"workspaces":[{"id":"w1","sourceType":"git_repo","repoUrl":"https://github.com/o/r","name":"r","isPrimary":true}]
		}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	got, err := c.CreateProject(context.Background(), "co1", ProjectCreateInput{
		Name:        "Proj",
		Description: "d",
		Status:      "planned",
		LeadAgentId: "a1",
		GoalIds:     []string{"g1"},
		Workspace: &WorkspaceCreateInput{
			SourceType: "git_repo",
			RepoUrl:    "https://github.com/o/r",
			IsPrimary:  true,
		},
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/api/companies/co1/projects" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}

	// goalIds MUST be an array; goalId (legacy mirror) MUST NOT be written.
	var goalIds []string
	if err := json.Unmarshal(raw["goalIds"], &goalIds); err != nil || len(goalIds) != 1 || goalIds[0] != "g1" {
		t.Errorf("goalIds body = %s (err=%v), want [\"g1\"]", string(raw["goalIds"]), err)
	}
	if _, ok := raw["goalId"]; ok {
		t.Errorf("body must NOT contain goalId (legacy mirror is read-only): %s", string(raw["goalId"]))
	}

	// inline workspace present with the three managed fields.
	var ws map[string]any
	if err := json.Unmarshal(raw["workspace"], &ws); err != nil {
		t.Fatalf("workspace body not an object: %s", string(raw["workspace"]))
	}
	if ws["sourceType"] != "git_repo" || ws["repoUrl"] != "https://github.com/o/r" || ws["isPrimary"] != true {
		t.Errorf("workspace body = %+v", ws)
	}

	if got.ID != "p1" || got.CompanyID != "co1" || got.Name != "Proj" || got.Status != "planned" ||
		got.LeadAgentId != "a1" {
		t.Errorf("got scalars = %+v", got)
	}
	if len(got.GoalIds) != 1 || got.GoalIds[0] != "g1" {
		t.Errorf("got.GoalIds = %+v", got.GoalIds)
	}
	if got.PrimaryWorkspace == nil || got.PrimaryWorkspace.RepoUrl != "https://github.com/o/r" ||
		got.PrimaryWorkspace.SourceType != "git_repo" || !got.PrimaryWorkspace.IsPrimary {
		t.Errorf("got.PrimaryWorkspace = %+v", got.PrimaryWorkspace)
	}
}

func TestCreateProject_OmitsOptionalFieldsWhenUnset(t *testing.T) {
	// name 是唯一必填；description/status/leadAgentId/goalIds/workspace 省略時不進 body。
	var raw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &raw)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":"p1","companyId":"co1","name":"Bare"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if _, err := c.CreateProject(context.Background(), "co1", ProjectCreateInput{Name: "Bare"}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	for _, k := range []string{"description", "status", "leadAgentId", "goalIds", "goalId", "workspace"} {
		if _, ok := raw[k]; ok {
			t.Errorf("body must NOT contain %q when unset: %s", k, string(raw[k]))
		}
	}
}

func TestGetProject_ParsesFields(t *testing.T) {
	// live 探測：GET /api/projects/{id} 獨立運作；回應含 goalIds(array)、leadAgentId、
	// primaryWorkspace(object)。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/projects/p1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{
			"id":"p1","companyId":"co1","name":"Proj","status":"in_progress",
			"leadAgentId":"a1","goalId":"g1","goalIds":["g1","g2"],
			"primaryWorkspace":{"id":"w1","sourceType":"git_repo","repoUrl":"https://github.com/o/r","isPrimary":true}
		}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	got, err := c.GetProject(context.Background(), "p1")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.Name != "Proj" || got.Status != "in_progress" || got.LeadAgentId != "a1" {
		t.Errorf("got = %+v", got)
	}
	if len(got.GoalIds) != 2 || got.GoalIds[0] != "g1" || got.GoalIds[1] != "g2" {
		t.Errorf("got.GoalIds = %+v", got.GoalIds)
	}
	if got.PrimaryWorkspace == nil || got.PrimaryWorkspace.RepoUrl != "https://github.com/o/r" {
		t.Errorf("got.PrimaryWorkspace = %+v", got.PrimaryWorkspace)
	}
}

func TestGetProject_404IsGone(t *testing.T) {
	// live 探測（2026-07-22）：已刪除的 project GET 回 404 "Project not found"。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":"Project not found"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	_, err := c.GetProject(context.Background(), "gone")
	if err == nil {
		t.Fatal("expected error on 404")
	}
	if !IsGone(err) || !IsNotFound(err) {
		t.Errorf("expected IsGone && IsNotFound true for 404, err = %v", err)
	}
}

func TestUpdateProject_OmitsUnsetFields(t *testing.T) {
	var raw map[string]json.RawMessage
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &raw)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"p1","companyId":"co1","name":"Renamed"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	newName := "Renamed"
	if _, err := c.UpdateProject(context.Background(), "p1", ProjectUpdateInput{Name: &newName}); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if gotMethod != "PATCH" || gotPath != "/api/projects/p1" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if _, ok := raw["name"]; !ok {
		t.Error("body missing name")
	}
	// workspace is RequiresReplace → never in an update body (live: inline workspace PATCH is a no-op).
	for _, k := range []string{"description", "status", "leadAgentId", "goalIds", "goalId", "workspace"} {
		if _, ok := raw[k]; ok {
			t.Errorf("body must NOT contain %q when unset (would clobber): %s", k, string(raw[k]))
		}
	}
}

func TestUpdateProject_SendsGoalIdsArrayWhenSet(t *testing.T) {
	// goal_ids 有值 → 送出 array（寫 goalIds，永不寫 goalId）。
	var raw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &raw)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"p1","companyId":"co1","name":"P","goalId":"g1","goalIds":["g1","g2"]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	ids := []string{"g1", "g2"}
	if _, err := c.UpdateProject(context.Background(), "p1", ProjectUpdateInput{GoalIds: &ids}); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	var goalIds []string
	if err := json.Unmarshal(raw["goalIds"], &goalIds); err != nil || len(goalIds) != 2 {
		t.Errorf("goalIds body = %s, want [g1,g2]", string(raw["goalIds"]))
	}
	if _, ok := raw["goalId"]; ok {
		t.Errorf("body must NEVER contain goalId (legacy mirror): %s", string(raw["goalId"]))
	}
}

func TestUpdateProject_SendsGoalIdsEmptyArrayWhenCleared(t *testing.T) {
	// goal_ids 清空 → 送出明確的空 array []（live 實證 2026-07-22：goalIds:[] 清掉所有連結）。
	// 這是 removal=clear 政策的核心：清空必須真的送 []，不是靜默省略。
	var raw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &raw)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"p1","companyId":"co1","name":"P","goalId":null,"goalIds":[]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	empty := []string{}
	if _, err := c.UpdateProject(context.Background(), "p1", ProjectUpdateInput{GoalIds: &empty}); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	v, ok := raw["goalIds"]
	if !ok {
		t.Fatal("body must CONTAIN goalIds when clearing (explicit empty array)")
	}
	if string(v) != "[]" {
		t.Errorf("goalIds = %s, want []", string(v))
	}
}

func TestUpdateProject_OmitsGoalIdsWhenNil(t *testing.T) {
	// 沒動到 goal_ids → nil pointer → 不得出現在 body（保留現況）。
	var raw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &raw)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"p1","companyId":"co1","name":"Renamed"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	name := "Renamed"
	if _, err := c.UpdateProject(context.Background(), "p1", ProjectUpdateInput{Name: &name}); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if _, ok := raw["goalIds"]; ok {
		t.Errorf("body must NOT contain goalIds when nil: %s", string(raw["goalIds"]))
	}
}

func TestUpdateProject_SendsLeadAgentIdNullWhenCleared(t *testing.T) {
	// lead_agent_id 從 config 移除 → 送出明確 JSON null（live 實證：清空 leadAgentId 成功）。
	var raw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &raw)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"p1","companyId":"co1","name":"P","leadAgentId":null}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if _, err := c.UpdateProject(context.Background(), "p1", ProjectUpdateInput{LeadAgentId: json.RawMessage("null")}); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	v, ok := raw["leadAgentId"]
	if !ok {
		t.Fatal("body must CONTAIN leadAgentId when clearing (explicit null)")
	}
	if string(v) != "null" {
		t.Errorf("leadAgentId = %s, want null", string(v))
	}
}

func TestUpdateProject_SendsLeadAgentIdValueWhenSet(t *testing.T) {
	var raw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &raw)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"p1","companyId":"co1","name":"P","leadAgentId":"a2"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	lid, _ := json.Marshal("a2")
	if _, err := c.UpdateProject(context.Background(), "p1", ProjectUpdateInput{LeadAgentId: json.RawMessage(lid)}); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if string(raw["leadAgentId"]) != `"a2"` {
		t.Errorf("leadAgentId = %s, want \"a2\"", string(raw["leadAgentId"]))
	}
}

func TestDeleteProject(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if err := c.DeleteProject(context.Background(), "p1"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if gotMethod != "DELETE" || gotPath != "/api/projects/p1" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
}
