package mygomcp

import (
	"context"
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxAccessElements = 500

type accessInput struct {
	SessionID string `json:"session_id" jsonschema:"Registered headless session ID"`
	Role      string `json:"role,omitempty" jsonschema:"Only elements with this role, as returned in role (for example button, text_field, heading)"`
	Name      string `json:"name,omitempty" jsonschema:"Only elements whose accessible name is exactly this"`
}

func (in accessInput) validate() error {
	if in.Role != "" {
		if _, ok := roleByName[in.Role]; !ok {
			return fmt.Errorf("unknown role %q", in.Role)
		}
	}
	return nil
}

type accessElement struct {
	Role            string       `json:"role"`
	Name            string       `json:"name,omitempty"`
	Value           string       `json:"value,omitempty"`
	Description     string       `json:"description,omitempty"`
	Placeholder     string       `json:"placeholder,omitempty"`
	RoleDescription string       `json:"role_description,omitempty"`
	Bounds          targetBounds `json:"bounds"`
	States          []string     `json:"states,omitempty"`
	Level           int          `json:"level,omitempty"`
	Min             *float64     `json:"min,omitempty"`
	Max             *float64     `json:"max,omitempty"`
	Now             *float64     `json:"now,omitempty"`
}

type accessInfo struct {
	status
	Matches   int             `json:"matches"`
	Truncated bool            `json:"truncated,omitempty"`
	Elements  []accessElement `json:"elements"`
}

// roleNames names every ui.Role, indexed by its value; ui.Role has no String.
var roleNames = [...]string{
	ui.RoleAuto: "auto", ui.RoleNone: "none", ui.RoleGroup: "group", ui.RoleText: "text",
	ui.RoleButton: "button", ui.RoleLink: "link", ui.RoleCheckBox: "check_box", ui.RoleRadio: "radio",
	ui.RoleSwitch: "switch", ui.RoleSlider: "slider", ui.RoleProgress: "progress", ui.RoleTextField: "text_field",
	ui.RoleImage: "image", ui.RoleList: "list", ui.RoleScroll: "scroll", ui.RoleDialog: "dialog",
	ui.RolePopup: "popup", ui.RoleTooltip: "tooltip", ui.RolePopUpButton: "pop_up_button", ui.RoleTabList: "tab_list",
	ui.RoleTab: "tab", ui.RoleSplitter: "splitter", ui.RoleStatus: "status", ui.RoleTable: "table",
	ui.RoleRow: "row", ui.RoleCell: "cell", ui.RoleColumnHeader: "column_header", ui.RoleTree: "tree",
	ui.RoleTreeItem: "tree_item", ui.RoleListItem: "list_item", ui.RoleMenuButton: "menu_button", ui.RoleToolbar: "toolbar",
	ui.RoleRadioGroup: "radio_group", ui.RoleToggleButton: "toggle_button", ui.RoleComboBox: "combo_box", ui.RoleDisclosure: "disclosure",
	ui.RoleMeter: "meter", ui.RoleStepper: "stepper", ui.RoleColorWell: "color_well", ui.RoleAlertDialog: "alert_dialog",
	ui.RoleMenu: "menu", ui.RoleMenuBar: "menu_bar", ui.RoleMenuItem: "menu_item", ui.RoleMenuItemCheckBox: "menu_item_check_box",
	ui.RoleMenuItemRadio: "menu_item_radio", ui.RoleHeading: "heading", ui.RoleRegion: "region", ui.RoleLog: "log",
	ui.RoleTimer: "timer", ui.RoleAlert: "alert", ui.RoleSeparator: "separator", ui.RoleRowHeader: "row_header",
	ui.RoleSearch: "search", ui.RoleArticle: "article", ui.RoleNavigation: "navigation", ui.RoleGrid: "grid",
	ui.RoleTreeGrid: "tree_grid", ui.RoleListBox: "list_box", ui.RoleOption: "option", ui.RoleDeletion: "deletion",
	ui.RoleInsertion: "insertion", ui.RoleTabPanel: "tab_panel", ui.RoleMain: "main", ui.RoleBanner: "banner",
	ui.RoleContentInfo: "content_info", ui.RoleComplementary: "complementary",
}

var roleByName = func() map[string]ui.Role {
	m := make(map[string]ui.Role, len(roleNames))
	for r, n := range roleNames {
		m[n] = ui.Role(r)
	}
	return m
}()

func roleName(r ui.Role) string {
	if int(r) < len(roleNames) && roleNames[r] != "" {
		return roleNames[r]
	}
	return fmt.Sprintf("role_%d", r)
}

func accessStates(a ui.AccessInfo) []string {
	var out []string
	for _, f := range []struct {
		on   bool
		name string
	}{
		{a.Checked, "checked"},
		{a.Mixed, "mixed"},
		{a.Expanded, "expanded"},
		{a.Expandable, "expandable"},
		{a.Selected, "selected"},
		{a.Selectable, "selectable"},
		{a.Multiselectable, "multiselectable"},
		{a.Busy, "busy"},
		{a.Disabled, "disabled"},
		{a.Invalid, "invalid"},
		{a.Focusable, "focusable"},
		{a.Focused, "focused"},
		{a.ReadOnly, "read_only"},
		{a.Multiline, "multiline"},
		{a.Password, "password"},
		{a.Search, "search"},
		{a.Offscreen, "offscreen"},
	} {
		if f.on {
			out = append(out, f.name)
		}
	}
	return out
}

func accessElementOf(a ui.AccessInfo) accessElement {
	e := accessElement{
		Role: roleName(a.Role), Name: a.Name, Value: a.Value, Description: a.Description,
		Placeholder: a.Placeholder, RoleDescription: a.RoleDescription,
		Bounds: targetBounds{X: a.Bounds.X, Y: a.Bounds.Y, Width: a.Bounds.W, Height: a.Bounds.H},
		States: accessStates(a), Level: a.Level,
	}
	if a.Max > a.Min {
		e.Min, e.Max, e.Now = &a.Min, &a.Max, &a.Now
	}
	return e
}

func (s *service) registerAccessTools(server *mcp.Server) {
	addTool(server, "mygo_access", "Read the accessibility tree of the last frame, as a screen reader would, without changing the view or the current observation: role, accessible name, value, states (checked, disabled, focused, ...) and bounds in DIPs for each element in reading order. Optional exact role and name filters; at most 500 elements.", func(ctx context.Context, in accessInput) (payload, error) {
		return s.operation(ctx, in.SessionID, in.validate, false, func(x *session) (payload, error) {
			x.tester.Frame()
			out := accessInfo{status: currentStatus(x, false), Elements: []accessElement{}}
			for _, a := range x.tester.AccessAll() {
				if in.Role != "" && a.Role != roleByName[in.Role] || in.Name != "" && a.Name != in.Name {
					continue
				}
				out.Matches++
				if len(out.Elements) == maxAccessElements {
					out.Truncated = true
					continue
				}
				out.Elements = append(out.Elements, accessElementOf(a))
			}
			return payload{metadata: out}, nil
		})
	})
}
