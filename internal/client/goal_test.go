package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateGoal_SendsFieldsAndParsesResponse(t *testing.T) {
	var gotBody map[string]any
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{
			"id":"g1","companyId":"co1","title":"Child Goal","description":"child",
			"level":"task","status":"planned","parentId":"p1","ownerAgentId":"a1"
		}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	got, err := c.CreateGoal(context.Background(), "co1", GoalCreateInput{
		Title:        "Child Goal",
		Description:  "child",
		Level:        "task",
		Status:       "planned",
		OwnerAgentId: "a1",
		ParentId:     "p1",
	})
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/api/companies/co1/goals" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if gotBody["title"] != "Child Goal" || gotBody["description"] != "child" ||
		gotBody["level"] != "task" || gotBody["status"] != "planned" ||
		gotBody["parentId"] != "p1" || gotBody["ownerAgentId"] != "a1" {
		t.Errorf("body = %+v", gotBody)
	}
	if got.ID != "g1" || got.CompanyID != "co1" || got.Title != "Child Goal" ||
		got.Level != "task" || got.Status != "planned" ||
		got.ParentId != "p1" || got.OwnerAgentId != "a1" {
		t.Errorf("got = %+v", got)
	}
}

func TestCreateGoal_OmitsOptionalFieldsWhenUnset(t *testing.T) {
	// live 探測：title 是唯一必填欄位；其餘省略時 API 自己給預設（level="task", status="planned"）。
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":"g1","companyId":"co1","title":"Bare Goal","level":"task","status":"planned"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	_, err := c.CreateGoal(context.Background(), "co1", GoalCreateInput{Title: "Bare Goal"})
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	for _, k := range []string{"description", "level", "status", "parentId", "ownerAgentId"} {
		if _, ok := gotBody[k]; ok {
			t.Errorf("body must NOT contain %q when unset: %+v", k, gotBody)
		}
	}
}

func TestGetGoal_ParsesFields(t *testing.T) {
	// live 探測：GET /api/goals/{id} 可獨立運作（不需 company id），跟 agent 同款，不像 secret
	// 得靠 company 底下的 list 端點。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/goals/g1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{
			"id":"g1","companyId":"co1","title":"Child Goal","description":"child",
			"level":"task","status":"active","parentId":"p1","ownerAgentId":"a1"
		}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	got, err := c.GetGoal(context.Background(), "g1")
	if err != nil {
		t.Fatalf("GetGoal: %v", err)
	}
	if got.Title != "Child Goal" || got.Description != "child" || got.Level != "task" ||
		got.Status != "active" || got.ParentId != "p1" || got.OwnerAgentId != "a1" {
		t.Errorf("got = %+v", got)
	}
}

func TestGetGoal_404IsGone(t *testing.T) {
	// live 探測（2026-07-22）：已刪除的 goal GET 回 404 "Goal not found"（跟 agent 同款，
	// 不像 company 是 403）。IsGone 兩者都涵蓋，無論如何都能正確判斷。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":"Goal not found"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	_, err := c.GetGoal(context.Background(), "gone")
	if err == nil {
		t.Fatal("expected error on 404")
	}
	if !IsGone(err) || !IsNotFound(err) {
		t.Errorf("expected IsGone && IsNotFound true for 404, err = %v", err)
	}
}

func TestUpdateGoal_OmitsUnsetFields(t *testing.T) {
	var gotBody map[string]any
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"g1","companyId":"co1","title":"Renamed"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	newTitle := "Renamed"
	_, err := c.UpdateGoal(context.Background(), "g1", GoalUpdateInput{Title: &newTitle})
	if err != nil {
		t.Fatalf("UpdateGoal: %v", err)
	}
	if gotMethod != "PATCH" || gotPath != "/api/goals/g1" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if _, ok := gotBody["title"]; !ok {
		t.Error("body missing title")
	}
	for _, k := range []string{"description", "level", "status", "parentId", "ownerAgentId"} {
		if _, ok := gotBody[k]; ok {
			t.Errorf("body must NOT contain %q when unset (would clobber): %+v", k, gotBody)
		}
	}
}

func TestUpdateGoal_SendsParentIdNullWhenCleared(t *testing.T) {
	// parent_id 從 config 移除 → 送出明確 JSON null（live 實證 2026-07-22：清空 parentId 成功）。
	var raw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &raw)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"g1","companyId":"co1","title":"Child","parentId":null}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if _, err := c.UpdateGoal(context.Background(), "g1", GoalUpdateInput{ParentId: json.RawMessage("null")}); err != nil {
		t.Fatalf("UpdateGoal: %v", err)
	}
	v, ok := raw["parentId"]
	if !ok {
		t.Fatal("body must CONTAIN parentId when clearing (explicit null)")
	}
	if string(v) != "null" {
		t.Errorf("parentId = %s, want null", string(v))
	}
}

func TestUpdateGoal_SendsParentIdValueWhenSet(t *testing.T) {
	var raw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &raw)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"g1","companyId":"co1","title":"Child","parentId":"p2"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	pid, _ := json.Marshal("p2")
	if _, err := c.UpdateGoal(context.Background(), "g1", GoalUpdateInput{ParentId: json.RawMessage(pid)}); err != nil {
		t.Fatalf("UpdateGoal: %v", err)
	}
	if string(raw["parentId"]) != `"p2"` {
		t.Errorf("parentId = %s, want \"p2\"", string(raw["parentId"]))
	}
}

func TestUpdateGoal_SendsOwnerAgentIdNullWhenCleared(t *testing.T) {
	// owner_agent_id 從 config 移除 → 送出明確 JSON null（live 實證：清空 ownerAgentId 成功）。
	var raw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &raw)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"g1","companyId":"co1","title":"Child","ownerAgentId":null}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if _, err := c.UpdateGoal(context.Background(), "g1", GoalUpdateInput{OwnerAgentId: json.RawMessage("null")}); err != nil {
		t.Fatalf("UpdateGoal: %v", err)
	}
	v, ok := raw["ownerAgentId"]
	if !ok {
		t.Fatal("body must CONTAIN ownerAgentId when clearing (explicit null)")
	}
	if string(v) != "null" {
		t.Errorf("ownerAgentId = %s, want null", string(v))
	}
}

func TestUpdateGoal_OmitsParentIdAndOwnerAgentIdWhenNil(t *testing.T) {
	// 沒動到 parent_id/owner_agent_id → nil RawMessage → 不得出現在 body（保留現況）。
	var raw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &raw)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"g1","companyId":"co1","title":"Renamed"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	title := "Renamed"
	if _, err := c.UpdateGoal(context.Background(), "g1", GoalUpdateInput{Title: &title}); err != nil {
		t.Fatalf("UpdateGoal: %v", err)
	}
	if _, ok := raw["parentId"]; ok {
		t.Errorf("body must NOT contain parentId when nil: %+v", raw)
	}
	if _, ok := raw["ownerAgentId"]; ok {
		t.Errorf("body must NOT contain ownerAgentId when nil: %+v", raw)
	}
}

func TestDeleteGoal(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if err := c.DeleteGoal(context.Background(), "g1"); err != nil {
		t.Fatalf("DeleteGoal: %v", err)
	}
	if gotMethod != "DELETE" || gotPath != "/api/goals/g1" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
}
