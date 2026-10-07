package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	"github.com/charmbracelet/x/ansi/kitty"
)

// Run the example's real host routing with asynchronous frames and capture
// terminal output without creating a terminal session or probing one.
func pump(m *model, cmd tea.Cmd) (ids []int, raw string) {
	var visit func(tea.Cmd)
	visit = func(c tea.Cmd) {
		if c == nil {
			return
		}
		msg := c()
		if msg == nil {
			return
		}
		if r, ok := msg.(tea.RawMsg); ok {
			raw += fmt.Sprint(r.Msg)
			return
		}
		v := reflect.ValueOf(msg)
		if v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeOf(tea.Cmd(nil)) {
			for i := 0; i < v.Len(); i++ {
				visit(v.Index(i).Interface().(tea.Cmd))
			}
			return
		}
		if f, ok := msg.(picture.KittyFrameMsg); ok {
			ids = append(ids, f.ID)
		}
		if _, ok := msg.(tea.QuitMsg); ok {
			return
		}
		_, next := m.Update(msg)
		visit(next)
	}
	visit(cmd)
	return
}

func TestTwoComponentsThroughPublicAPI(t *testing.T) {
	picture.ForceKittyCapability(picture.KittyCapabilitySupported)
	picture.SetTmuxPassthrough(false)
	t.Cleanup(func() { picture.ForceKittyCapability(picture.KittyCapabilityUnknown) })
	m := newModel()
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 110, Height: 32})
	ids, _ := pump(m, cmd)
	if len(ids) != 2 || ids[0] == ids[1] {
		t.Fatalf("image IDs: %v", ids)
	}
	for _, qr := range m.codes {
		if qr.Err() != nil || !strings.ContainsRune(qr.View(), kitty.Placeholder) {
			t.Fatal("missing component")
		}
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: '2'})
	_, raw := pump(m, cmd)
	if m.codes[1] != nil || !strings.Contains(raw, fmt.Sprintf("a=d,d=I,i=%d", ids[1])) {
		t.Fatal("removal did not clean the second image")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: '2'})
	replacement, _ := pump(m, cmd)
	for _, id := range replacement {
		if id == ids[0] || id == ids[1] {
			t.Fatal("restoration reused an image ID")
		}
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'n'})
	pump(m, cmd)
	if m.selected != 1 || m.codes[0].Err() != nil {
		t.Fatal("content update failed")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'g'})
	_, raw = pump(m, cmd)
	if !strings.Contains(raw, "a=d,d=I,i=") || !strings.Contains(m.View().Content, "▀") {
		t.Fatal("glyph transition failed")
	}
	_, cmd = m.Update(tea.WindowSizeMsg{Width: 20, Height: 10})
	pump(m, cmd)
	if m.codes[0].Err() == nil || strings.Contains(m.View().Content, "▀") {
		t.Fatal("small layout cropped the QR")
	}
	_, cmd = m.Update(tea.WindowSizeMsg{Width: 110, Height: 32})
	pump(m, cmd)
	if m.codes[0].Err() != nil {
		t.Fatal("resize did not recover")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'q'})
	pump(m, cmd)
	for _, qr := range m.codes {
		if qr.View() != "" {
			t.Fatal("quit did not close QR")
		}
	}
}

// A QR that qr.SetSize accepted must survive the host's pane layout unwrapped.
// lipgloss v2 widths include the border, so the pane must hand the QR the width
// that remains inside it. Sweep widths so every code meets its exact limit.
func TestPaneLayoutNeverWrapsAFittedQR(t *testing.T) {
	for _, solid := range []string{"0", "1"} {
		t.Run("solid="+solid, func(t *testing.T) {
			t.Setenv("TERM_PROGRAM", "")
			t.Setenv("NTCHARTS_QRCODE_SOLID", solid)
			m := newModel()
			fitted := 0
			for width := 20; width <= 220; width++ {
				_, cmd := m.Update(tea.WindowSizeMsg{Width: width, Height: 90})
				pump(m, cmd)
				content := m.View().Content
				for i, qr := range m.codes {
					if qr == nil || qr.Err() != nil {
						continue
					}
					fitted++
					for _, row := range strings.Split(qr.View(), "\n") {
						if !strings.Contains(content, row) {
							t.Fatalf("width %d: QR %d row wrapped or altered by the pane layout", width, i+1)
						}
					}
				}
			}
			if fitted == 0 {
				t.Fatal("sweep never fitted a QR")
			}
		})
	}
}
