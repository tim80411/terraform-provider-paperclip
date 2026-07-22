// internal/client/routine_test.go
package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateRoutine_PostsToCompanyPath(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"rt1","companyId":"c1","title":"daily sweep","status":"active"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	out, err := c.CreateRoutine(context.Background(), "c1", RoutineCreateInput{Title: "daily sweep"})
	if err != nil {
		t.Fatalf("CreateRoutine error: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/api/companies/c1/routines" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if gotBody["title"] != "daily sweep" {
		t.Errorf("body title = %v", gotBody["title"])
	}
	if _, has := gotBody["status"]; has {
		t.Error("unset status must be omitted (server defaults to active)")
	}
	if out.ID != "rt1" || out.Status != "active" {
		t.Errorf("out = %+v", out)
	}
}

func TestGetRoutine_Parses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/routines/rt1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"rt1","companyId":"c1","title":"t","status":"paused","assigneeAgentId":"a1","description":"d"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	out, err := c.GetRoutine(context.Background(), "rt1")
	if err != nil {
		t.Fatalf("GetRoutine error: %v", err)
	}
	if out.Status != "paused" || out.AssigneeAgentId != "a1" || out.Description != "d" {
		t.Errorf("out = %+v", out)
	}
}

// PATCH partial-merge（updateRoutineSchema = create.partial()）：未設欄位不進 body。
func TestUpdateRoutine_OmitsUnsetFields(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"rt1","title":"new"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	title := "new"
	if _, err := c.UpdateRoutine(context.Background(), "rt1", RoutineUpdateInput{Title: &title}); err != nil {
		t.Fatalf("UpdateRoutine error: %v", err)
	}
	if gotBody["title"] != "new" {
		t.Errorf("body title = %v", gotBody["title"])
	}
	for _, k := range []string{"description", "status", "assigneeAgentId"} {
		if _, has := gotBody[k]; has {
			t.Errorf("unset %s must be omitted from PATCH body", k)
		}
	}
}

// assigneeAgentId 三態（同 project.leadAgentId 手法）：nil→不送；JSON null→清空；uuid→指定。
func TestUpdateRoutine_AssigneeTriState(t *testing.T) {
	var gotRaw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotRaw)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"rt1"}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "tok")

	// 清空：送明確 JSON null
	if _, err := c.UpdateRoutine(context.Background(), "rt1", RoutineUpdateInput{AssigneeAgentId: json.RawMessage("null")}); err != nil {
		t.Fatal(err)
	}
	if string(gotRaw["assigneeAgentId"]) != "null" {
		t.Errorf("clear: assigneeAgentId = %s, want null", gotRaw["assigneeAgentId"])
	}

	// 指定：送字串 uuid
	if _, err := c.UpdateRoutine(context.Background(), "rt1", RoutineUpdateInput{AssigneeAgentId: json.RawMessage(`"a2"`)}); err != nil {
		t.Fatal(err)
	}
	if string(gotRaw["assigneeAgentId"]) != `"a2"` {
		t.Errorf("set: assigneeAgentId = %s", gotRaw["assigneeAgentId"])
	}
}

// routine 無 DELETE 端點：destroy = PATCH status=archived（ROUTINE_STATUSES 實證）。
func TestArchiveRoutine_PatchesStatusArchived(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"rt1","status":"archived"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if err := c.ArchiveRoutine(context.Background(), "rt1"); err != nil {
		t.Fatalf("ArchiveRoutine error: %v", err)
	}
	if gotMethod != "PATCH" || gotPath != "/api/routines/rt1" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if gotBody["status"] != "archived" {
		t.Errorf("body status = %v", gotBody["status"])
	}
	if len(gotBody) != 1 {
		t.Errorf("archive body must contain only status, got %v", gotBody)
	}
}
