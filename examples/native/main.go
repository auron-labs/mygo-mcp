// Native demonstrates the same ordinary ui controls with and without MCP.
package main

import (
	"fmt"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var startAutomation = func() func() { return func() {} }

type form struct {
	editing      bool
	text, result string
}

func (s *form) submit() { s.result = s.text; fmt.Printf("Submitted: %s\n", s.result) }

func (s *form) view(c *ui.Context) {
	if c.Shortcut(ui.Cmd, ui.KeyEnter) {
		s.submit()
	}
	ui.Column(c).Fill().Padding(24).Gap(16).Children(func() {
		ui.Text(c, "Native MCP example").Height(24)
		if !s.editing {
			if ui.Button(c, "Open form").Width(240).Height(40).Clicked() {
				s.editing = true
			}
			return
		}
		if ui.TextInput(c, &s.text).Label("Message").Placeholder("Message").Width(400).Height(40).Submitted() {
			s.submit()
		}
		ui.Button(c, "Disabled").Disabled(true).Width(240).Height(40)
		if ui.PrimaryButton(c, "Submit").Width(240).Height(40).Clicked() {
			s.submit()
		}
		ui.Text(c, "Result: "+s.result).Height(40)
	})
}

func main() {
	stop := func() {}
	mygo.App.WhenReady(func() {
		for _, title := range []string{"MCP form", "MCP second window"} {
			s := &form{}
			mygo.NewWindow(mygo.WindowOptions{Title: title, Width: 640, Height: 480, Content: ui.View(s.view)})
		}
		stop = startAutomation()
	})
	err := mygo.App.Run()
	stop()
	if err != nil {
		log.Fatal(err)
	}
}
