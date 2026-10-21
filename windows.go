package mygomcp

import (
	"crypto/rand"
	"errors"

	"github.com/egoist/mygo"
)

type window struct {
	w      *mygo.Window
	driver *mygo.InputDriver
	shot   *screenshot
}

// WindowInfo describes a supported native content window; dimensions are DIPs.
type WindowInfo struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Ready      bool     `json:"ready"`
	Focused    bool     `json:"focused"`
	Width      float64  `json:"width"`
	Height     float64  `json:"height"`
	Scale      float64  `json:"scale"`
	Operations []string `json:"operations"`
}

func newID() string { return rand.Text() }

// refresh runs on the serialized worker. Polling the public live-window
// snapshot avoids attaching lifecycle listeners that outlive a server.
func (s *Server) refresh() []WindowInfo {
	infos := []WindowInfo{}
	mygo.RunOnMain(func() {
		live := mygo.Windows()
		for id, entry := range s.windows {
			found := false
			for _, w := range live {
				if entry.w == w {
					found = true
					break
				}
			}
			if !found {
				delete(s.windows, id)
			}
		}
		for _, w := range live {
			id := ""
			var entry *window
			for k, v := range s.windows {
				if v.w == w {
					id, entry = k, v
					break
				}
			}
			if entry == nil {
				d, err := mygo.NewInputDriver(w)
				if err != nil {
					continue
				}
				id = newID()
				entry = &window{w: w, driver: d}
				s.windows[id] = entry
			}
			g, err := entry.driver.Geometry()
			if err != nil {
				continue
			}
			infos = append(infos, WindowInfo{ID: id, Title: w.Title(), Ready: g.Width > 0 && g.Height > 0, Focused: w.IsFocused(), Width: g.Width, Height: g.Height, Scale: g.Scale, Operations: []string{"screenshot", "click", "type", "key"}})
		}
	})
	return infos
}

func (s *Server) target(id string) (*window, error) {
	if s.windows[id] == nil {
		return nil, errors.New("mygo-mcp: unknown or closed window_id; list windows first")
	}
	s.refresh()
	w := s.windows[id]
	if w == nil {
		return nil, errors.New("mygo-mcp: unknown or closed window_id")
	}
	return w, nil
}
