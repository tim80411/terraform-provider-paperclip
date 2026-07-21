package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestBuildSecretUpdateInput_NothingChanged(t *testing.T) {
	state := secretResourceModel{
		ID:           types.StringValue("s1"),
		CompanyID:    types.StringValue("co1"),
		Name:         types.StringValue("gh-token"),
		Key:          types.StringValue("gh_token"),
		Value:        types.StringValue("v1"),
		ValueVersion: types.StringValue("1"),
	}
	plan := state

	in := buildSecretUpdateInput(plan, state)

	if in.Name != nil {
		t.Errorf("Name = %v, want nil", *in.Name)
	}
	if in.Key != nil {
		t.Errorf("Key = %v, want nil", *in.Key)
	}
}

func TestBuildSecretUpdateInput_OnlyNameChanged(t *testing.T) {
	state := secretResourceModel{
		ID: types.StringValue("s1"), CompanyID: types.StringValue("co1"),
		Name: types.StringValue("gh-token"), Key: types.StringValue("gh_token"),
		Value: types.StringValue("v1"), ValueVersion: types.StringValue("1"),
	}
	plan := state
	plan.Name = types.StringValue("gh-token-renamed")

	in := buildSecretUpdateInput(plan, state)

	if in.Name == nil || *in.Name != "gh-token-renamed" {
		t.Fatalf("Name = %v, want pointer to \"gh-token-renamed\"", in.Name)
	}
	if in.Key != nil {
		t.Errorf("Key = %v, want nil", *in.Key)
	}
}

func TestBuildSecretUpdateInput_OnlyKeyChanged(t *testing.T) {
	state := secretResourceModel{
		ID: types.StringValue("s1"), CompanyID: types.StringValue("co1"),
		Name: types.StringValue("gh-token"), Key: types.StringValue("gh_token"),
		Value: types.StringValue("v1"), ValueVersion: types.StringValue("1"),
	}
	plan := state
	plan.Key = types.StringValue("gh_token_2")

	in := buildSecretUpdateInput(plan, state)

	if in.Key == nil || *in.Key != "gh_token_2" {
		t.Fatalf("Key = %v, want pointer to \"gh_token_2\"", in.Key)
	}
	if in.Name != nil {
		t.Errorf("Name = %v, want nil", *in.Name)
	}
}

func TestBuildSecretUpdateInput_ValueVersionAloneNeverBuildsPatchFields(t *testing.T) {
	// value/value_version 從不透過 PATCH 送——這是 rotate 的職責，build-update-input 不該碰它們。
	state := secretResourceModel{
		ID: types.StringValue("s1"), CompanyID: types.StringValue("co1"),
		Name: types.StringValue("gh-token"), Key: types.StringValue("gh_token"),
		Value: types.StringValue("v1"), ValueVersion: types.StringValue("1"),
	}
	plan := state
	plan.Value = types.StringValue("v2")
	plan.ValueVersion = types.StringValue("2")

	in := buildSecretUpdateInput(plan, state)

	if in.Name != nil || in.Key != nil {
		t.Errorf("in = %+v, want both nil (value/value_version changes must not leak into PATCH)", in)
	}
}

func TestShouldRotateSecret(t *testing.T) {
	base := secretResourceModel{ValueVersion: types.StringValue("1")}

	same := base
	if shouldRotateSecret(same, base) {
		t.Error("shouldRotateSecret = true, want false when value_version unchanged")
	}

	bumped := base
	bumped.ValueVersion = types.StringValue("2")
	if !shouldRotateSecret(bumped, base) {
		t.Error("shouldRotateSecret = false, want true when value_version changed")
	}

	// null → null 不算變更（都沒設過 value_version）。
	nullState := secretResourceModel{}
	nullPlan := secretResourceModel{}
	if shouldRotateSecret(nullPlan, nullState) {
		t.Error("shouldRotateSecret = true, want false when both null")
	}
}

func TestReconcileKey_SameCaseInsensitive_KeepsPrior(t *testing.T) {
	// live 探測：paperclip 把 key 正規化成小寫（"GH_TOKEN" → "gh_token"）。
	// 若只是大小寫不同，保留呼叫端（config/prior state）原本的大小寫，避免每次 Read 都出現假 diff。
	prior := types.StringValue("GH_TOKEN")
	got := reconcileKey(prior, "gh_token")
	if !got.Equal(prior) {
		t.Errorf("reconcileKey = %v, want unchanged prior %v", got, prior)
	}
}

func TestReconcileKey_GenuineDrift_UsesAPIValue(t *testing.T) {
	prior := types.StringValue("gh_token")
	got := reconcileKey(prior, "totally_different_key")
	if got.ValueString() != "totally_different_key" {
		t.Errorf("reconcileKey = %v, want API value totally_different_key", got)
	}
}

func TestReconcileKey_NullPrior_UsesAPIValue(t *testing.T) {
	// import 情境：ImportState 只灌 company_id/id，Key 一開始是 null。
	got := reconcileKey(types.StringNull(), "gh_token")
	if got.ValueString() != "gh_token" {
		t.Errorf("reconcileKey = %v, want API value gh_token", got)
	}
}
