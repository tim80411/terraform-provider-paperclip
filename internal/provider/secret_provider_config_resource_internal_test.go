// internal/provider/secret_provider_config_resource_internal_test.go
package provider

import "testing"

// config_json 存 JSON 字串；read-back 的 key 排序/空白差異不得產生假漂移，
// 語意相等時保留 state 原字串。
func TestJSONSemanticallyEqual_IgnoresOrderAndWhitespace(t *testing.T) {
	a := `{"region":"ap-northeast-1","roleArn":"arn:aws:iam::1:role/x"}`
	b := "{\n  \"roleArn\": \"arn:aws:iam::1:role/x\",\n  \"region\": \"ap-northeast-1\"\n}"

	if !jsonSemanticallyEqual(a, b) {
		t.Error("semantically equal JSON must compare equal")
	}
}

func TestJSONSemanticallyEqual_DetectsValueChange(t *testing.T) {
	a := `{"region":"ap-northeast-1"}`
	b := `{"region":"us-east-1"}`

	if jsonSemanticallyEqual(a, b) {
		t.Error("different values must compare unequal")
	}
}

func TestJSONSemanticallyEqual_InvalidJSONFallsBackToStringCompare(t *testing.T) {
	if jsonSemanticallyEqual("not-json", "{}") {
		t.Error("invalid vs valid must be unequal")
	}
	if !jsonSemanticallyEqual("not-json", "not-json") {
		t.Error("identical strings equal even if not JSON")
	}
}
