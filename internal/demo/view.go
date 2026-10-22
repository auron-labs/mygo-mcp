// Package demo contains a small application view shared by the harness and tests.
package demo

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// State is owned by the caller's UI execution path.
type State struct {
	Editing   bool
	Text      string
	Submitted string
	Ready     bool
	due       time.Time
}

// New creates independent application state; View never recreates it.
func New() *State { return &State{} }

// View uses ordinary MyGo controls and handlers, without automation-specific routing.
func (s *State) View(c *ui.Context) {
	ui.Column(c).Children(func() {
		if !s.Editing {
			if ui.Button(c, "Edit").Size(200, 40).Clicked() {
				s.Editing = true
			}
			return
		}
		ui.TextInput(c, &s.Text).Label("Message").Size(200, 40)
		submit := ui.Button(c, "Submit").Size(200, 40).Clicked()
		if c.Shortcut(ui.Cmd, ui.KeyEnter) || submit {
			s.Submitted = s.Text
			s.Ready = false
			s.due = c.Now().Add(100 * time.Millisecond)
		}
		if !s.due.IsZero() && !s.Ready {
			if !c.Now().Before(s.due) {
				s.Ready = true
			} else {
				c.After(s.due.Sub(c.Now()))
			}
		}
		ui.Text(c, "Submitted: "+s.Submitted)
		if s.Ready {
			ui.Text(c, "Ready")
		}
	})
}
