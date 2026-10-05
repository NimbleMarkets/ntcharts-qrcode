package qrcode

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"reflect"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/makiuchi-d/gozxing"
	decoder "github.com/makiuchi-d/gozxing/qrcode"
)

func component(t *testing.T, text string, next func() int) *Model {
	t.Helper()
	m, err := New(encoded(t, text, Medium), Config{NextID: next})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

func ids() func() int {
	id := 100
	return func() int { id += 1000; return id }
}

// run follows Bubble Tea batches and sequences, delivering async frames to
// every model just as the host event loop does. Raw output goes to the test.
func run(cmd tea.Cmd, models ...*Model) (frames []picture.KittyFrameMsg, raw string) {
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
			frames = append(frames, f)
		}
		for _, model := range models {
			visit(model.Update(msg))
		}
	}
	visit(cmd)
	return
}

// Reconstruct only from emitted characters and ANSI foreground/background,
// independently of the QR matrix and halfBlocks implementation.
func glyphImage(t *testing.T, output string, geometry ...int) image.Image {
	t.Helper()
	lines := strings.Split(output, "\n")
	w, h := ansi.StringWidth(lines[0]), len(lines)*2
	cw, halfHeight := 4, 4
	if len(geometry) == 2 {
		cw, halfHeight = geometry[0], geometry[1]
	}
	out := image.NewGray(image.Rect(0, 0, w*cw, h*halfHeight))
	tokens := regexp.MustCompile("\x1b\\[[0-9;]+m|▀").FindAllString
	for y, line := range lines {
		x, fg, bg := 0, -1, -1
		for _, token := range tokens(line, -1) {
			if token == "\x1b[0m" {
				fg, bg = -1, -1
				continue
			}
			if token != "▀" {
				var r, g, b, br, bgc, bb int
				if n, err := fmt.Sscanf(token, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm", &r, &g, &b, &br, &bgc, &bb); err != nil || n != 6 || r != g || g != b || br != bgc || bgc != bb {
					t.Fatalf("unexpected colors: %q", token)
				}
				fg, bg = r, br
				continue
			}
			if (fg != 0 && fg != 255) || (bg != 0 && bg != 255) {
				t.Fatalf("glyph depends on terminal colors: %d %d", fg, bg)
			}
			for dy := range 2 * halfHeight {
				c := fg
				if dy >= halfHeight {
					c = bg
				}
				for dx := range cw {
					out.SetGray(x*cw+dx, y*2*halfHeight+dy, color.Gray{Y: uint8(c)})
				}
			}
			x++
		}
		if x != w {
			t.Fatalf("not a whole glyph row: %d/%d", x, w)
		}
	}
	return out
}

func transmitted(t *testing.T, frame picture.KittyFrameMsg) image.Image {
	t.Helper()
	var encoded strings.Builder
	for _, chunk := range strings.Split(frame.APC, "\x1b_G")[1:] {
		_, data, ok := strings.Cut(chunk, ";")
		if ok {
			data, _, _ = strings.Cut(data, "\x1b\\")
			encoded.WriteString(data)
		}
	}
	data, err := base64.StdEncoding.DecodeString(encoded.String())
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestGlyphRoundTripFitAndUpdates(t *testing.T) {
	m := component(t, "https://example.org/東京?q=☕", ids())
	n := len(m.code.modules)
	m.SetSize(n, (n+1)/2)
	decode(t, glyphImage(t, m.View()), "https://example.org/東京?q=☕")
	for _, size := range [][2]int{{n - 1, (n + 1) / 2}, {n, (n - 1) / 2}, {0, 0}, {-1, -1}} {
		m.SetSize(size[0], size[1])
		if !errors.Is(m.Err(), ErrDoesNotFit) || strings.Contains(m.View(), "▀") {
			t.Fatal("cropped QR rather than reporting insufficient space")
		}
	}
	m.SetSize(185, 100)
	m.SetCode(encoded(t, "replacement ✓", High))
	decode(t, glyphImage(t, m.View()), "replacement ✓")
	m.SetTerminal(false, 10, 20)
	decode(t, glyphImage(t, m.View()), "replacement ✓")
	// 8x24 cells need three columns and two half-rows per square module.
	m.SetTerminal(false, 8, 24)
	if m.Err() != nil || ansi.StringWidth(strings.Split(m.View(), "\n")[0]) != 3*len(m.code.modules) {
		t.Fatalf("nonstandard cell geometry: %v", m.Err())
	}
	decode(t, glyphImage(t, m.View(), 8, 12), "replacement ✓")
	m.SetTerminal(false, 0, 16)
	if m.Err() == nil {
		t.Fatal("invalid geometry accepted")
	}
	m.SetCode(nil)
	if m.View() != "" {
		t.Fatal("clear retained content")
	}
}

func TestGlyphNearSquareCellsStayCompactAndDecode(t *testing.T) {
	for _, cell := range [][2]int{{8, 17}, {9, 17}, {9, 19}, {9, 20}, {8, 18}} {
		t.Run(fmt.Sprint(cell), func(t *testing.T) {
			payload := "https://example.org/東京?q=☕"
			m := component(t, payload, ids())
			n := len(m.code.modules)
			m.SetSize(n, (n+1)/2)
			m.Update(uv.CellSizeEvent{Width: cell[0], Height: cell[1]})
			if m.Err() != nil {
				t.Fatalf("near-square cells inflated the layout: %v", m.Err())
			}
			if ansi.StringWidth(strings.Split(m.View(), "\n")[0]) != n || len(strings.Split(m.View(), "\n")) != (n+1)/2 {
				t.Fatal("expected one column and one half-row per module")
			}
			// Double both pixel axes so odd cell heights have integral halves.
			// Decode the actual physical aspect, without PURE_BARCODE's square
			// module assumption or any normalization back to the source matrix.
			img := glyphImage(t, m.View(), 2*cell[0], cell[1])
			bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
			if err != nil {
				t.Fatal(err)
			}
			result, err := decoder.NewQRCodeReader().Decode(bitmap, nil)
			if err != nil || result.GetText() != payload {
				t.Fatalf("physical glyph roundtrip: %v", err)
			}
			m.SetSize(n-1, (n+1)/2)
			if !errors.Is(m.Err(), ErrDoesNotFit) || strings.Contains(m.View(), "▀") {
				t.Fatal("insufficient space must still reject the complete code")
			}
			m.SetSize(n, (n+1)/2)
			if m.Err() != nil {
				t.Fatal("resize did not recover")
			}
		})
	}
}

func TestKittyPixelsAreUnscaledAndOpaque(t *testing.T) {
	picture.ForceKittyCapability(picture.KittyCapabilitySupported)
	picture.SetTmuxPassthrough(false)
	t.Cleanup(func() { picture.ForceKittyCapability(picture.KittyCapabilityUnknown) })
	text := "https://nimblemarkets.github.io/gloss/docs/"
	m := component(t, text, ids())
	m.SetSize(61, 30)
	cmd := m.SetTerminal(true, 9, 19)
	if m.View() != "Preparing QR…" {
		t.Fatal("pending Kitty frame exposed a resampled transitional QR")
	}
	frames, _ := run(cmd, m)
	if len(frames) != 1 || !strings.ContainsRune(m.View(), kitty.Placeholder) {
		t.Fatalf("no Kitty placement: %d %s", len(frames), m.View())
	}
	img := transmitted(t, frames[0])
	decode(t, img, text)
	// Compare the actual wire PNG with every integer-scaled source module,
	// including cell-alignment padding. A change in picture's fit pipeline
	// must fail here if it interpolates even one black/white boundary pixel.
	n, scale := len(m.code.modules), 8
	if img.Bounds().Dx()%9 != 0 || img.Bounds().Dy()%19 != 0 {
		t.Fatal("image does not exactly fill the placement")
	}
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			want := uint32(65535)
			if x < n*scale && y < n*scale && m.code.modules[y/scale][x/scale] {
				want = 0
			}
			r, g, b, a := img.At(x, y).RGBA()
			if r != want || g != want || b != want || a != 65535 {
				t.Fatalf("wire image resampled at %d,%d: %d,%d,%d,%d", x, y, r, g, b, a)
			}
		}
	}
	old := frames[0].ID
	frames, raw := run(m.Update(uv.CellSizeEvent{Width: 10, Height: 20}), m)
	if len(frames) != 1 || frames[0].ID == old || !strings.Contains(raw, fmt.Sprintf("a=d,d=I,i=%d", old)) {
		t.Fatal("cell resize did not retire and replace image")
	}
	decode(t, transmitted(t, frames[0]), text)
	frames, raw = run(m.SetSize(25, 14), m)
	if len(frames) != 1 || !strings.Contains(raw, "a=d,d=I") {
		t.Fatal("layout resize not replaced")
	}
	decode(t, transmitted(t, frames[0]), text)
}

func TestCapabilityMultipleInstancesAndCleanup(t *testing.T) {
	t.Cleanup(func() { picture.ForceKittyCapability(picture.KittyCapabilityUnknown) })
	next := ids()
	a, b := component(t, "first code", next), component(t, "second code", next)
	a.SetSize(80, 40)
	b.SetSize(80, 40)
	for _, capability := range []picture.KittyCapability{picture.KittyCapabilityUnknown, picture.KittyCapabilityUnsupported} {
		picture.ForceKittyCapability(capability)
		_, raw := run(a.SetTerminal(true, 8, 16), a, b)
		if raw != "" || !strings.Contains(a.View(), "▀") {
			t.Fatal("unknown/unsupported capability emitted Kitty or probed")
		}
	}
	picture.ForceKittyCapability(picture.KittyCapabilitySupported)
	fa, _ := run(a.SetTerminal(true, 8, 16), a, b)
	fb, _ := run(b.SetTerminal(true, 8, 16), a, b)
	if len(fa) != 1 || len(fb) != 1 || fa[0].ID == fb[0].ID {
		t.Fatal("instances collided")
	}
	viewB := b.View()
	stale, _ := run(a.SetCode(encoded(t, "stale encoded frame", Medium)))
	frames, raw := run(a.SetCode(encoded(t, "current code", Medium)), a, b)
	if len(frames) != 1 || !strings.Contains(raw, fmt.Sprintf("a=d,d=I,i=%d", stale[0].ID)) {
		t.Fatal("replacement did not delete old ID")
	}
	decode(t, transmitted(t, frames[0]), "current code")
	if a.Update(stale[0]) != nil || b.Update(frames[0]) != nil || b.View() != viewB {
		t.Fatal("stale or foreign frame was accepted")
	}
	_, raw = run(a.Close(), a, b)
	if !strings.Contains(raw, fmt.Sprintf("a=d,d=I,i=%d", frames[0].ID)) || a.View() != "" || a.Close() != nil || a.Update(frames[0]) != nil {
		t.Fatal("removal retained graphics or accepted a late frame")
	}
	_, raw = run(b.SetSize(1, 1), a, b)
	if !errors.Is(b.Err(), ErrDoesNotFit) || !strings.Contains(raw, fmt.Sprintf("a=d,d=I,i=%d", fb[0].ID)) {
		t.Fatal("insufficient space retained old Kitty code")
	}
	run(b.SetSize(80, 40), b)
	picture.ForceKittyCapability(picture.KittyCapabilityUnsupported)
	_, raw = run(b.SetTerminal(true, 8, 16), b)
	if !strings.Contains(raw, "a=d,d=I") {
		t.Fatal("capability fallback did not clear Kitty")
	}
	decode(t, glyphImage(t, b.View()), "second code")
}

func TestRemovalAfterFrameAcceptedStillDeletesLateTransmission(t *testing.T) {
	picture.ForceKittyCapability(picture.KittyCapabilitySupported)
	t.Cleanup(func() { picture.ForceKittyCapability(picture.KittyCapabilityUnknown) })
	m := component(t, "accepted but not transmitted", ids())
	m.SetSize(80, 40)
	frames, _ := run(m.SetTerminal(true, 8, 16))
	accepted := m.Update(frames[0])
	run(m.Close())
	// Deliberately finish an accepted command after its component is gone.
	// No model remains to consume readyMsg: cleanup must happen in the command.
	_, raw := run(accepted)
	delete := fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", frames[0].ID)
	if !strings.HasSuffix(raw, delete) || !strings.Contains(raw, frames[0].APC) {
		t.Fatal("a late raw transmission resurrected the removed QR")
	}
}
