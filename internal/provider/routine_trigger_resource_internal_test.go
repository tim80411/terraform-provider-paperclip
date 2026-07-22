// internal/provider/routine_trigger_resource_internal_test.go
package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

func TestFindTriggerByID_Found(t *testing.T) {
	list := []client.RoutineTrigger{
		{ID: "tg1", CronExpression: "0 9 * * *"},
		{ID: "tg2", CronExpression: "0 18 * * *"},
	}

	got, ok := findTriggerByID(list, "tg2")
	if !ok || got.CronExpression != "0 18 * * *" {
		t.Fatalf("got = %+v ok=%v", got, ok)
	}
}

func TestFindTriggerByID_NotFound(t *testing.T) {
	if _, ok := findTriggerByID([]client.RoutineTrigger{{ID: "tg1"}}, "ghost"); ok {
		t.Fatal("expected not found")
	}
}

func TestBuildTriggerUpdateInput_NothingChanged(t *testing.T) {
	state := routineTriggerResourceModel{
		ID:             types.StringValue("tg1"),
		RoutineID:      types.StringValue("rt1"),
		CronExpression: types.StringValue("0 9 * * *"),
		Timezone:       types.StringValue("UTC"),
		Enabled:        types.BoolValue(true),
	}
	plan := state

	in := buildTriggerUpdateInput(plan, state)

	if in.CronExpression != nil || in.Timezone != nil || in.Enabled != nil {
		t.Errorf("no-op diff must be empty, got %+v", in)
	}
}

func TestBuildTriggerUpdateInput_CronAndEnabledChanged(t *testing.T) {
	state := routineTriggerResourceModel{
		ID:             types.StringValue("tg1"),
		CronExpression: types.StringValue("0 9 * * *"),
		Enabled:        types.BoolValue(true),
	}
	plan := state
	plan.CronExpression = types.StringValue("30 8 * * 1-5")
	plan.Enabled = types.BoolValue(false)

	in := buildTriggerUpdateInput(plan, state)

	if in.CronExpression == nil || *in.CronExpression != "30 8 * * 1-5" {
		t.Errorf("CronExpression = %v", in.CronExpression)
	}
	if in.Enabled == nil || *in.Enabled != false {
		t.Errorf("Enabled = %v", in.Enabled)
	}
	if in.Timezone != nil {
		t.Errorf("unchanged Timezone must stay nil")
	}
}
