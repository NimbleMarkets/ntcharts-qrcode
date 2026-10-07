// A host-owned terminal session containing two independent QR components.
package main

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	booba "github.com/NimbleMarkets/go-booba"
	"github.com/NimbleMarkets/ntcharts-qrcode/qrcode"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	"github.com/charmbracelet/x/ansi"
)

var addresses = []string{
	"https://nimble.markets/",
	"https://github.com/NimbleMarkets/ntcharts-qrcode",
	"https://example.org/東京?q=☕",
}

// solidCells reports whether the host should avoid half-block glyphs. Apple
// Terminal draws U+2580 from the font, misplacing it within the cell. The host
// owns this choice: tmux hides TERM_PROGRAM, so NTCHARTS_QRCODE_SOLID=1 forces it.
func solidCells() bool {
	return os.Getenv("TERM_PROGRAM") == "Apple_Terminal" || os.Getenv("NTCHARTS_QRCODE_SOLID") == "1"
}

type model struct {
	terminal                    picture.Model
	codes                       [2]*qrcode.Model
	width, height, id, selected int
	preferKitty, solid          bool
}

func newModel() *model {
	m := &model{terminal: picture.NewWithConfig(picture.Config{KittyID: 1}), preferKitty: true, solid: solidCells()}
	m.codes[0], m.codes[1] = m.newQR(addresses[0]), m.newQR(addresses[1])
	return m
}

func (m *model) nextID() int {
	m.id++
	return 100 + m.id*1000 // shared across components, separate from the host picture
}

func (m *model) newQR(text string) *qrcode.Model {
	code, err := qrcode.Encode(text, qrcode.Options{})
	if err != nil {
		panic(err)
	} // fixed example payloads
	qr, err := qrcode.New(code, qrcode.Config{NextID: m.nextID, SolidCells: m.solid})
	if err != nil {
		panic(err)
	}
	return qr
}

// Only the host initiates asynchronous terminal detection.
func (m *model) Init() tea.Cmd { return m.terminal.Init() }

func (m *model) paneWidth() int {
	if m.codes[1] == nil {
		return m.width
	}
	return max(0, (m.width-1)/2)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.terminal.Update(msg)}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			for _, qr := range m.codes {
				if qr != nil {
					cmds = append(cmds, qr.Close())
				}
			}
			return m, tea.Sequence(tea.Batch(cmds...), tea.Quit)
		case "g":
			m.preferKitty = !m.preferKitty
		case "n", "space":
			m.selected = (m.selected + 1) % len(addresses)
			code, _ := qrcode.Encode(addresses[m.selected], qrcode.Options{})
			cmds = append(cmds, m.codes[0].SetCode(code))
		case "2":
			if m.codes[1] == nil {
				m.codes[1] = m.newQR(addresses[1])
			} else {
				cmds = append(cmds, m.codes[1].Close())
				m.codes[1] = nil
			}
		}
	}
	cw, ch := m.terminal.CellPixelSize()
	for _, qr := range m.codes {
		if qr == nil {
			continue
		}
		cmds = append(cmds, qr.SetTerminal(m.preferKitty, cw, ch))
		cmds = append(cmds, qr.SetSize(max(0, m.paneWidth()-2), max(0, m.height-6)))
		// Broadcast; each component filters its own asynchronous frames.
		cmds = append(cmds, qr.Update(msg))
	}
	return m, tea.Batch(cmds...)
}

func (m *model) View() tea.View {
	if m.width < 12 || m.height < 8 {
		v := tea.NewView("Enlarge terminal")
		v.AltScreen = true
		return v
	}
	inner := m.paneWidth() - 2
	var panes []string
	for i, qr := range m.codes {
		if qr == nil {
			continue
		}
		content := qr.View()
		if qr.Err() != nil {
			content = ansi.Truncate(content, inner, "…")
		}
		address := addresses[1]
		if i == 0 {
			address = addresses[m.selected]
		}
		lines := fmt.Sprintf("QR %d\n%s\n%s", i+1, content, ansi.Truncate(address, inner, "…"))
		panes = append(panes, lipgloss.NewStyle().Width(m.paneWidth()).Height(m.height-4).
			Border(lipgloss.RoundedBorder()).Render(lines))
	}
	body := panes[0]
	if len(panes) == 2 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, panes[0], " ", panes[1])
	}
	mode := "glyph"
	if m.solid {
		mode = "solid cells"
	}
	if m.preferKitty && picture.KittySupported() == picture.KittyCapabilitySupported {
		mode = "Kitty"
	}
	heading := ansi.Truncate("ntcharts-qrcode · "+mode, m.width, "…")
	hint := ansi.Truncate("n change URL · 2 show/hide second · g graphics · q quit", m.width, "…")
	v := tea.NewView(strings.Join([]string{heading, body, hint}, "\n"))
	v.AltScreen = true
	return v
}

func main() {
	if err := booba.Run(newModel()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
