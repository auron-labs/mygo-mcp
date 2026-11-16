package mygomcp

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRoleNamesComplete(t *testing.T) {
	for r := ui.RoleAuto; r <= ui.RoleComplementary; r++ {
		if roleNames[r] == "" {
			t.Errorf("role %d has no name", r)
		}
	}
	if len(roleNames) != int(ui.RoleComplementary)+1 {
		t.Error("roleNames does not end at the last ui role")
	}
}

func TestAccessTool(t *testing.T) {
	s := testService(t, demoFixture("one", 1))
	cs := testClient(t, s)
	id := s.sessions[0].id
	obs := inspect(t, cs, id)
	var all accessInfo
	callTool(t, cs, "mygo_access", accessInput{SessionID: id}, &all)
	if all.SessionID != id || all.Matches == 0 || all.Matches != len(all.Elements) || all.Truncated {
		t.Fatal("expected a nonempty accessibility tree", all)
	}
	var buttons accessInfo
	callTool(t, cs, "mygo_access", accessInput{SessionID: id, Role: "button", Name: "Edit"}, &buttons)
	if buttons.Matches == 0 {
		t.Fatal("Edit button not found", all)
	}
	for _, e := range buttons.Elements {
		if e.Role != "button" || e.Name != "Edit" || e.Bounds.Width <= 0 || e.Bounds.Height <= 0 {
			t.Fatal("unexpected filtered element", e)
		}
	}
	var none accessInfo
	callTool(t, cs, "mygo_access", accessInput{SessionID: id, Name: "does not exist"}, &none)
	if none.Matches != 0 || none.Elements == nil || len(none.Elements) != 0 {
		t.Fatal("expected no matches", none)
	}
	toolError(t, cs, "mygo_access", accessInput{SessionID: id, Role: "nope"})
	toolError(t, cs, "mygo_access", accessInput{})
	// Reading the tree must not supersede the observation.
	callTool(t, cs, "mygo_click_label", clickLabelInput{id, obs.ObservationID, "Edit"}, nil)
}
