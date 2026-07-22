package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestCreateAgent_SendsFieldsAndParsesAdapterConfig(t *testing.T) {
	var gotBody map[string]any
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(201)
		// live 探測：server 建立後會 auto-inject instructions*，並回 adapterConfig 全貌。
		_, _ = w.Write([]byte(`{
			"id":"a1","companyId":"co1","name":"Chief","role":"ceo","adapterType":"claude_local",
			"adapterConfig":{"model":"claude-opus-4-8","chrome":true,
			  "instructionsEntryFile":"AGENTS.md","instructionsBundleMode":"managed"}
		}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	got, err := c.CreateAgent(context.Background(), "co1", AgentCreateInput{
		Name:          "Chief",
		Role:          "ceo",
		AdapterType:   "claude_local",
		AdapterConfig: map[string]any{"model": "claude-opus-4-8", "chrome": true},
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/api/companies/co1/agents" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if gotBody["name"] != "Chief" || gotBody["adapterType"] != "claude_local" {
		t.Errorf("body = %+v", gotBody)
	}
	ac, _ := gotBody["adapterConfig"].(map[string]any)
	if ac["model"] != "claude-opus-4-8" {
		t.Errorf("body.adapterConfig = %+v", ac)
	}
	if got.ID != "a1" || got.CompanyID != "co1" || got.Role != "ceo" {
		t.Errorf("got = %+v", got)
	}
	if got.AdapterConfig["model"] != "claude-opus-4-8" {
		t.Errorf("got.AdapterConfig = %+v", got.AdapterConfig)
	}
}

func TestCreateAgent_OmitsReportsToWhenNil(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":"a1","companyId":"co1","name":"Chief","reportsTo":null}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	_, err := c.CreateAgent(context.Background(), "co1", AgentCreateInput{Name: "Chief"})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	// 根 agent：reportsTo 為 nil → 不得出現在 body（否則會送 null / 空字串）。
	if _, ok := gotBody["reportsTo"]; ok {
		t.Errorf("body must NOT contain reportsTo when nil: %+v", gotBody)
	}
}

func TestCreateAgent_SendsReportsToWhenSet(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":"a2","companyId":"co1","name":"Sub","reportsTo":"a1"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	boss := "a1"
	_, err := c.CreateAgent(context.Background(), "co1", AgentCreateInput{Name: "Sub", ReportsTo: &boss})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if gotBody["reportsTo"] != "a1" {
		t.Errorf("body.reportsTo = %v, want a1", gotBody["reportsTo"])
	}
}

func TestGetAgent_ParsesAdapterConfigAndReportsTo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agents/a1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{
			"id":"a1","companyId":"co1","name":"Scout","role":"engineer","title":"Data Eng","icon":"database",
			"capabilities":"does data things","reportsTo":"boss1","adapterType":"claude_local","status":"idle",
			"adapterConfig":{"model":"claude-sonnet-4-6","chrome":true,
			  "paperclipSkillSync":{"desiredSkills":["paperclipai/paperclip/paperclip-board"]},
			  "instructionsEntryFile":"AGENTS.md"}
		}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	got, err := c.GetAgent(context.Background(), "a1")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if got.Name != "Scout" || got.Role != "engineer" || got.Title != "Data Eng" || got.Icon != "database" {
		t.Errorf("got = %+v", got)
	}
	if got.Capabilities != "does data things" || got.ReportsTo != "boss1" {
		t.Errorf("got capabilities/reportsTo = %q / %q", got.Capabilities, got.ReportsTo)
	}
	if got.AdapterConfig["model"] != "claude-sonnet-4-6" {
		t.Errorf("got.AdapterConfig.model = %v", got.AdapterConfig["model"])
	}
	ps, ok := got.AdapterConfig["paperclipSkillSync"].(map[string]any)
	if !ok {
		t.Fatalf("paperclipSkillSync missing: %+v", got.AdapterConfig)
	}
	if _, ok := ps["desiredSkills"]; !ok {
		t.Errorf("paperclipSkillSync.desiredSkills missing: %+v", ps)
	}
}

func TestGetAgent_404IsGone(t *testing.T) {
	// live 探測：已刪除的 agent GET 回 404 "Agent not found"（company 是 403，agent 是 404）。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":"Agent not found"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	_, err := c.GetAgent(context.Background(), "gone")
	if err == nil {
		t.Fatal("expected error on 404")
	}
	if !IsGone(err) || !IsNotFound(err) {
		t.Errorf("expected IsGone && IsNotFound true for 404, err = %v", err)
	}
}

func TestUpdateAgent_OmitsUnsetFieldsSendsAdapterConfig(t *testing.T) {
	var gotBody map[string]any
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"a1","companyId":"co1","name":"Chief"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	f := false
	_, err := c.UpdateAgent(context.Background(), "a1", AgentUpdateInput{
		AdapterConfig:        map[string]any{"model": "claude-opus-4-8"},
		ReplaceAdapterConfig: &f,
	})
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if gotMethod != "PATCH" || gotPath != "/api/agents/a1" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	// 未設的欄位（name/role/...）不得進 body（保留未管欄位，spec §6.3）。
	if _, ok := gotBody["name"]; ok {
		t.Error("body must NOT contain name when unset")
	}
	if _, ok := gotBody["role"]; ok {
		t.Error("body must NOT contain role when unset")
	}
	// replaceAdapterConfig=false 必須明確送出（bool 指標，false 不能被 omitempty 吃掉）。
	if v, ok := gotBody["replaceAdapterConfig"]; !ok || v != false {
		t.Errorf("body.replaceAdapterConfig = %v (ok=%v), want false present", v, ok)
	}
	if ac, _ := gotBody["adapterConfig"].(map[string]any); ac["model"] != "claude-opus-4-8" {
		t.Errorf("body.adapterConfig = %+v", gotBody["adapterConfig"])
	}
}

func TestUpdateAgent_SendsReportsToNullWhenCleared(t *testing.T) {
	// reports_to reset → root：body 必須含 "reportsTo": null（不是省略、也不是空字串）。
	// live 探測（2026-07-22）：PATCH {"reportsTo":null} 讓 agent 回到根。
	var raw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &raw)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"a1","companyId":"co1","name":"Sub","reportsTo":null}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if _, err := c.UpdateAgent(context.Background(), "a1", AgentUpdateInput{ReportsTo: json.RawMessage("null")}); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	v, ok := raw["reportsTo"]
	if !ok {
		t.Fatal("body must CONTAIN reportsTo when clearing (explicit null)")
	}
	if string(v) != "null" {
		t.Errorf("reportsTo = %s, want null", string(v))
	}
}

func TestUpdateAgent_SendsReportsToValueWhenSet(t *testing.T) {
	var raw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &raw)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"a1","companyId":"co1","name":"Sub","reportsTo":"boss-2"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	rt, _ := json.Marshal("boss-2")
	if _, err := c.UpdateAgent(context.Background(), "a1", AgentUpdateInput{ReportsTo: json.RawMessage(rt)}); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if string(raw["reportsTo"]) != `"boss-2"` {
		t.Errorf("reportsTo = %s, want \"boss-2\"", string(raw["reportsTo"]))
	}
}

func TestUpdateAgent_OmitsReportsToWhenNil(t *testing.T) {
	// 沒動到 reports_to → nil RawMessage → 不得出現在 body（保留現況，不誤送 null）。
	var raw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &raw)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"a1","companyId":"co1","name":"Renamed"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	name := "Renamed"
	if _, err := c.UpdateAgent(context.Background(), "a1", AgentUpdateInput{Name: &name}); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if _, ok := raw["reportsTo"]; ok {
		t.Errorf("body must NOT contain reportsTo when nil: %+v", raw)
	}
}

func TestDeleteAgent(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if err := c.DeleteAgent(context.Background(), "a1"); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	if gotMethod != "DELETE" || gotPath != "/api/agents/a1" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
}

func TestSyncAgentSkills_PostsDesiredSkills(t *testing.T) {
	var gotBody map[string]any
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"desiredSkills":["paperclipai/paperclip/paperclip-board"]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	err := c.SyncAgentSkills(context.Background(), "a1", []string{"paperclipai/paperclip/paperclip-board"})
	if err != nil {
		t.Fatalf("SyncAgentSkills: %v", err)
	}
	if gotMethod != "POST" || gotPath != "/api/agents/a1/skills/sync" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	ds, ok := gotBody["desiredSkills"].([]any)
	if !ok || len(ds) != 1 || ds[0] != "paperclipai/paperclip/paperclip-board" {
		t.Errorf("body.desiredSkills = %+v", gotBody["desiredSkills"])
	}
}

func TestSyncAgentSkills_EmptyListSendsEmptyArray(t *testing.T) {
	// 清空 skills：body 必須是 [] 而不是 null（live 探測空陣列 200 OK）。
	var raw map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &raw)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"desiredSkills":[]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if err := c.SyncAgentSkills(context.Background(), "a1", []string{}); err != nil {
		t.Fatalf("SyncAgentSkills: %v", err)
	}
	if string(raw["desiredSkills"]) != "[]" {
		t.Errorf("desiredSkills = %s, want []", string(raw["desiredSkills"]))
	}
}

// --- MergeAdapterConfig: the load-bearing test of this whole task ---

func TestMergeAdapterConfig_PreservesUnmanagedKeys(t *testing.T) {
	// 這是全 task 最關鍵的測試：送 model 不得抹掉 paperclipSkillSync / instructions*。
	current := map[string]any{
		"model":  "claude-sonnet-4-6",
		"chrome": true,
		"paperclipSkillSync": map[string]any{
			"desiredSkills": []any{"paperclipai/paperclip/paperclip-board"},
		},
		"instructionsFilePath":       "/x/AGENTS.md",
		"instructionsRootPath":       "/x",
		"instructionsEntryFile":      "AGENTS.md",
		"instructionsBundleMode":     "managed",
		"graceSec":                   15,
		"dangerouslySkipPermissions": true,
		"someUnknownFutureKey":       "keepme",
	}
	managed := map[string]any{"model": "claude-opus-4-8"}

	out := MergeAdapterConfig(current, managed)

	if out["model"] != "claude-opus-4-8" {
		t.Errorf("model = %v, want overlaid claude-opus-4-8", out["model"])
	}
	// 每一個未管理的 key 都必須原封不動保留。
	if _, ok := out["paperclipSkillSync"]; !ok {
		t.Error("paperclipSkillSync was DROPPED — this is the exact regression the merge guards against")
	}
	for _, k := range []string{
		"instructionsFilePath", "instructionsRootPath", "instructionsEntryFile",
		"instructionsBundleMode", "graceSec", "dangerouslySkipPermissions", "someUnknownFutureKey",
	} {
		if _, ok := out[k]; !ok {
			t.Errorf("unmanaged key %q was dropped", k)
		}
	}
	// chrome 未在 managed 出現 → 保留 current 的值。
	if out["chrome"] != true {
		t.Errorf("chrome = %v, want preserved true", out["chrome"])
	}
}

func TestMergeAdapterConfig_OverlaysAllFourManagedKeys(t *testing.T) {
	current := map[string]any{"model": "old", "keepme": "yes"}
	managed := map[string]any{
		"model":  "new",
		"engine": "some-engine",
		"chrome": false,
		"env":    map[string]any{"GH_TOKEN": map[string]any{"type": "secret_ref", "secretId": "s1"}},
	}
	out := MergeAdapterConfig(current, managed)
	if out["model"] != "new" || out["engine"] != "some-engine" || out["chrome"] != false {
		t.Errorf("managed keys not overlaid: %+v", out)
	}
	if _, ok := out["env"]; !ok {
		t.Error("env not overlaid")
	}
	if out["keepme"] != "yes" {
		t.Error("unmanaged keepme dropped")
	}
}

func TestMergeAdapterConfig_IgnoresNonManagedKeysInManaged(t *testing.T) {
	// 安全性：即使 managed 混進非管理 key，也不得覆蓋到 current（只疊 model/engine/chrome/env）。
	current := map[string]any{"paperclipSkillSync": "sacred"}
	managed := map[string]any{"model": "m", "paperclipSkillSync": "ATTACKER"}
	out := MergeAdapterConfig(current, managed)
	if out["paperclipSkillSync"] != "sacred" {
		t.Errorf("paperclipSkillSync = %v, want untouched 'sacred' (managed must not overlay non-managed keys)", out["paperclipSkillSync"])
	}
	if out["model"] != "m" {
		t.Errorf("model = %v, want m", out["model"])
	}
}

func TestMergeAdapterConfig_DoesNotMutateInputs(t *testing.T) {
	current := map[string]any{"model": "old", "paperclipSkillSync": "keep"}
	managed := map[string]any{"model": "new"}
	currentCopy := map[string]any{"model": "old", "paperclipSkillSync": "keep"}
	managedCopy := map[string]any{"model": "new"}

	_ = MergeAdapterConfig(current, managed)

	if !reflect.DeepEqual(current, currentCopy) {
		t.Errorf("current was mutated: %+v", current)
	}
	if !reflect.DeepEqual(managed, managedCopy) {
		t.Errorf("managed was mutated: %+v", managed)
	}
}

func TestMergeAdapterConfig_NilCurrent(t *testing.T) {
	out := MergeAdapterConfig(nil, map[string]any{"model": "m"})
	if out["model"] != "m" {
		t.Errorf("out = %+v, want model=m even with nil current", out)
	}
}
