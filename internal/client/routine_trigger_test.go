// internal/client/routine_trigger_test.go
package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// v1 只做 schedule kind（webhook 含 signing secret、api kind 屬 runtime 面）。
func TestCreateRoutineTrigger_PostsScheduleKind(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(201)
		// live 實證：POST 回 {trigger, revision} 信封（PATCH 才回裸 trigger）
		_, _ = w.Write([]byte(`{"trigger":{"id":"tg1","kind":"schedule","cronExpression":"0 9 * * *","timezone":"Asia/Taipei","enabled":true},"revision":{"id":"rev1"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	out, err := c.CreateRoutineTrigger(context.Background(), "rt1", RoutineTriggerCreateInput{
		CronExpression: "0 9 * * *",
		Timezone:       "Asia/Taipei",
	})
	if err != nil {
		t.Fatalf("CreateRoutineTrigger error: %v", err)
	}
	if gotPath != "/api/routines/rt1/triggers" {
		t.Errorf("path = %s", gotPath)
	}
	if gotBody["kind"] != "schedule" {
		t.Errorf("kind = %v, want schedule (hardcoded)", gotBody["kind"])
	}
	if gotBody["cronExpression"] != "0 9 * * *" {
		t.Errorf("cronExpression = %v", gotBody["cronExpression"])
	}
	if out.ID != "tg1" || !out.Enabled || out.Timezone != "Asia/Taipei" {
		t.Errorf("out = %+v", out)
	}
}

func TestGetRoutine_ParsesTriggers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"rt1","title":"t","triggers":[{"id":"tg1","kind":"schedule","cronExpression":"0 9 * * *","enabled":false}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	out, err := c.GetRoutine(context.Background(), "rt1")
	if err != nil {
		t.Fatalf("GetRoutine error: %v", err)
	}
	if len(out.Triggers) != 1 || out.Triggers[0].ID != "tg1" || out.Triggers[0].Enabled {
		t.Errorf("triggers = %+v", out.Triggers)
	}
}

func TestUpdateRoutineTrigger_OmitsUnsetFields(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"tg1"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	enabled := false
	if _, err := c.UpdateRoutineTrigger(context.Background(), "tg1", RoutineTriggerUpdateInput{Enabled: &enabled}); err != nil {
		t.Fatalf("UpdateRoutineTrigger error: %v", err)
	}
	if gotMethod != "PATCH" || gotPath != "/api/routine-triggers/tg1" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if gotBody["enabled"] != false {
		t.Errorf("enabled = %v", gotBody["enabled"])
	}
	for _, k := range []string{"cronExpression", "timezone", "label", "kind"} {
		if _, has := gotBody[k]; has {
			t.Errorf("unset %s must be omitted", k)
		}
	}
}

func TestDeleteRoutineTrigger_SendsDelete(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"tg1"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if err := c.DeleteRoutineTrigger(context.Background(), "tg1"); err != nil {
		t.Fatalf("DeleteRoutineTrigger error: %v", err)
	}
	if gotMethod != "DELETE" || gotPath != "/api/routine-triggers/tg1" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
}
