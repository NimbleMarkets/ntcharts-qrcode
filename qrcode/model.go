package qrcode

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	uv "github.com/charmbracelet/ultraviolet"
)

var ErrDoesNotFit = errors.New("QR code doesn't fit")

// Config uses the host's image-ID allocator, shared with its other pictures.
// NextID must return a fresh, positive 24-bit ID on every call, including
// across components. Images are retired on replacement, resize, and Close.
type Config struct{ NextID func() int }

// Model is a composable picture-style Bubble Tea component: setters and
// Update return commands for the host to run; View returns only its content.
// Do not copy a Model. The host must call Close and run its cleanup command
// before removing it. No Init/probe is needed: the host supplies capability
// and cell geometry through SetTerminal, from its existing picture model.
type Model struct {
	code         *Code
	nextID       func() int
	pic          *picture.Model
	flight       *placement
	id           int
	w, h, cw, ch int
	kitty        bool
	glyph        string
	grid         string
	err          error
	closed       bool
}

func New(code *Code, cfg Config) (*Model, error) {
	if code == nil || len(code.modules) == 0 || cfg.NextID == nil {
		return nil, errors.New("QR component requires a code and the host's image-ID allocator")
	}
	return &Model{code: code, nextID: cfg.NextID, cw: 8, ch: 16, err: ErrDoesNotFit}, nil
}

// SetCode replaces content. Nil clears it. Encode errors can be handled before
// changing the displayed content. A failed fit never leaves the old QR visible.
func (m *Model) SetCode(code *Code) tea.Cmd {
	if m.closed || code == m.code {
		return nil
	}
	m.code = code
	return m.layout()
}

func (m *Model) SetSize(width, height int) tea.Cmd {
	width, height = max(0, width), max(0, height)
	if m.closed || (m.w == width && m.h == height) {
		return nil
	}
	m.w, m.h = width, height
	return m.layout()
}

// SetTerminal consumes the host's graphics choice and cell pixel dimensions.
// Kitty also requires NTCharts' existing process-wide affirmative detection.
// Unknown/unsupported capability falls back to glyphs; this never probes.
// The default geometry is 8x16 until the host supplies its measured size.
// Glyphs repeat whole columns and half-rows to approximate square modules
// (longer side at most 9/8 of the shorter). Kitty pixels remain exactly square.
func (m *Model) SetTerminal(kitty bool, cellWidth, cellHeight int) tea.Cmd {
	kitty = kitty && picture.KittySupported() == picture.KittyCapabilitySupported
	if m.closed || (m.kitty == kitty && m.cw == cellWidth && m.ch == cellHeight) {
		return nil
	}
	m.kitty, m.cw, m.ch = kitty, cellWidth, cellHeight
	return m.layout()
}

// Update accepts frames from the host event loop. Geometry is laid out before
// touching picture, so a cell-size reply cannot trigger a filtered QR resize.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	if m.closed {
		return nil
	}
	if cell, ok := msg.(uv.CellSizeEvent); ok {
		return m.SetTerminal(m.kitty, cell.Width, cell.Height)
	}
	if ready, ok := msg.(readyMsg); ok {
		if ready.owner == m && ready.id == m.id && m.pic != nil {
			m.grid = ready.grid
		}
		return nil
	}
	if m.pic != nil {
		cmd := m.pic.Update(msg)
		if frame, ok := msg.(picture.KittyFrameMsg); ok && cmd != nil {
			id := m.id
			flight := m.flight
			// Reveal only after picture's transmission and placement sequence.
			// picture.View's temporary image-to-glyph fallback resamples, so we
			// deliberately never use that transitional representation for a QR.
			return tea.Sequence(cmd, func() tea.Msg {
				// Removal may happen after Update accepted the frame but before
				// its raw transmission ran. Repeat the same ID-specific cleanup
				// after that transmission, even if the host removed this model.
				if !flight.active.Load() && flight.cleanup != nil {
					return flight.cleanup()
				}
				return readyMsg{m, id, frame.Grid}
			})
		}
		return cmd
	}
	return nil
}

// Err reports an invalid layout or wraps ErrDoesNotFit with required space.
// View never returns a partial QR. A host may show Err in its own layout.
func (m *Model) Err() error { return m.err }

func (m *Model) View() string {
	if m.closed || m.code == nil {
		return ""
	}
	if m.err != nil {
		return m.err.Error()
	}
	if m.pic != nil {
		if m.grid == "" {
			return "Preparing QR…"
		}
		return m.grid
	}
	return m.glyph
}

// Close clears pending frames and returns NTCharts' ID-specific delete
// command (including its tmux handling). It is idempotent.
func (m *Model) Close() tea.Cmd {
	m.closed, m.glyph = true, ""
	return m.retire()
}

func (m *Model) retire() tea.Cmd {
	m.grid = ""
	if m.pic == nil {
		return nil
	}
	cmd := m.pic.SetImage(nil)
	m.flight.cleanup = cmd
	m.flight.active.Store(false)
	m.pic = nil
	return cmd
}

func (m *Model) layout() tea.Cmd {
	cleanup := m.retire()
	m.err, m.glyph = nil, ""
	if m.code == nil {
		return cleanup
	}
	if len(m.code.modules) == 0 {
		m.err = errors.New("no QR code")
		return cleanup
	}
	if m.cw < 1 || m.ch < 1 || m.cw > 256 || m.ch > 256 {
		m.err = errors.New("QR cell dimensions must be 1..256 pixels")
		return cleanup
	}
	n := len(m.code.modules)
	if !m.kitty {
		x, y := glyphScale(m.cw, m.ch)
		cols, rows := n*x, (n*y+1)/2
		if cols > m.w || rows > m.h || cols*rows > 65536 {
			m.err = fmt.Errorf("%w: needs %d columns × %d rows in glyph mode", ErrDoesNotFit, cols, rows)
			return cleanup
		}
		m.glyph = halfBlocks(m.code.modules, x, y)
		return cleanup
	}
	// Bound allocation, cell multiplication, and Kitty's diacritic grid.
	cols, rows := min(m.w, 256, MaxImageEdge/m.cw), min(m.h, 256, MaxImageEdge/m.ch)
	scale := min(8, cols*m.cw/n, rows*m.ch/n)
	if scale < 1 {
		m.err = fmt.Errorf("%w: needs at least %d × %d pixels including the quiet zone", ErrDoesNotFit, n, n)
		return cleanup
	}
	src, err := m.code.Image(scale)
	if err != nil {
		m.err = err
		return cleanup
	}
	side := n * scale
	cols, rows = (side+m.cw-1)/m.cw, (side+m.ch-1)/m.ch
	// FitFill + transparent Background + EXACT target-sized source bypasses
	// picture.fillTo's CatmullRom scaler in NTCharts v2.4.0. The source itself
	// is opaque white. ResolutionFactor must stay 1. Decoder tests inspect
	// the actual transmitted PNG, not only this source image.
	out := image.NewRGBA(image.Rect(0, 0, cols*m.cw, rows*m.ch))
	draw.Draw(out, out.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(out, src.Bounds(), src, image.Point{}, draw.Src)
	id := m.nextID()
	if id <= 0 || id >= 1<<24 || id == m.id {
		m.err = errors.New("QR image allocator must return fresh positive 24-bit IDs")
		return cleanup
	}
	m.id = id
	p := picture.NewWithConfig(picture.Config{KittyID: id, KittyZ: -1, Fit: picture.FitFill,
		CellPixelWidth: m.cw, CellPixelHeight: m.ch, KittyResolutionFactor: 1})
	m.pic = &p
	m.flight = &placement{}
	m.flight.active.Store(true)
	p.SetSize(cols, rows)
	p.Toggle()
	return tea.Sequence(cleanup, p.SetImage(out))
}

type readyMsg struct {
	owner *Model
	id    int
	grid  string
}

// cleanup is written before active becomes false; commands use the atomic
// flag's ordering to read it. No event-loop-owned Model state is read by them.
type placement struct {
	active  atomic.Bool
	cleanup tea.Cmd
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// Find the smallest whole-cell replication with nearly square modules.
// Exact ratios alone are impractical: 8x17 cells would require 17 columns
// and 16 half-rows per module. Allowing at most 12.5% aspect error keeps that
// case at one column/half-row, without filtering or losing any modules.
func glyphScale(cw, ch int) (int, int) {
	g := gcd(2*cw, ch)
	for x := 1; x < ch/g; x++ {
		y := max(1, (2*cw*x+ch/2)/ch)
		w, h := 2*cw*x, ch*y // doubled pixels to represent odd cell heights
		if 8*max(w, h) <= 9*min(w, h) {
			return x, y
		}
	}
	return ch / g, 2 * cw / g
}

// halfBlocks maps modules directly to explicit black/white foreground AND
// background. The unused bottom half-row is white, never terminal-default.
func halfBlocks(matrix [][]bool, sx, sy int) string {
	n := len(matrix)
	var out strings.Builder
	for y := 0; y < n*sy; y += 2 {
		if y > 0 {
			out.WriteByte('\n')
		}
		last := -1
		for x := range n * sx {
			top := matrix[y/sy][x/sx]
			bottom := y+1 < n*sy && matrix[(y+1)/sy][x/sx]
			fg, bg := 255, 255
			if top {
				fg = 0
			}
			if bottom {
				bg = 0
			}
			key := fg*256 + bg
			if key != last {
				fmt.Fprintf(&out, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm", fg, fg, fg, bg, bg, bg)
				last = key
			}
			out.WriteRune('▀')
		}
		out.WriteString("\x1b[0m")
	}
	return out.String()
}
