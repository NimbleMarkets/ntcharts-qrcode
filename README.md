# ntcharts-qrcode — Terminal QR codes for Bubble Tea

<p>
    <a href="https://pkg.go.dev/github.com/NimbleMarkets/ntcharts-qrcode/qrcode"><img src="https://pkg.go.dev/badge/github.com/NimbleMarkets/ntcharts-qrcode/qrcode.svg" alt="Go Reference"></a>
    <a href="https://github.com/NimbleMarkets/ntcharts-qrcode/actions/workflows/ci.yml"><img src="https://github.com/NimbleMarkets/ntcharts-qrcode/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
    <a href="./CODE_OF_CONDUCT.md"><img src="https://img.shields.io/badge/Contributor%20Covenant-2.1-4baaaa.svg" alt="Code Of Conduct"></a>
</p>

`ntcharts-qrcode` is an experimental [NTCharts](https://github.com/NimbleMarkets/ntcharts)
companion for [Bubble Tea v2](https://github.com/charmbracelet/bubbletea).
It follows the companion [ntcharts-osm](https://github.com/NimbleMarkets/ntcharts-osm),
[ntcharts-svg](https://github.com/NimbleMarkets/ntcharts-svg), and
[ntcharts-pdf](https://github.com/NimbleMarkets/ntcharts-pdf) project conventions.
It encodes QR codes, exposes their module matrix and image, and displays them
with Kitty graphics or explicit black/white Unicode half-blocks.
Developed and used in [gloss](https://github.com/NimbleMarkets/gloss).

The host owns terminal detection, the event loop, layout, and image IDs.
The component never reads stdin or probes the terminal. If the complete QR
and its four-module quiet zone cannot fit, it returns a fit error.

## Quickstart

Requires Go 1.26.8+ and Bubble Tea v2. The API is experimental.

```sh
go get github.com/NimbleMarkets/ntcharts-qrcode/qrcode
go run ./examples/qrcode
```

The example shows two independently managed codes. `n` changes the first
URL, `2` removes/restores the second, `g` switches graphics, and `q` quits.
Resize the terminal to exercise fit errors and recovery. At narrow widths,
hide the second code with `2` to give the first the full width.

```go
import "github.com/NimbleMarkets/ntcharts-qrcode/qrcode"

code, err := qrcode.Encode("https://nimble.markets/", qrcode.Options{})
if err != nil {
    return err
}
matrix := code.Matrix() // copy; true = black, includes the white quiet zone
img, err := code.Image(8) // opaque image.Image, exactly 8 pixels per module
// Handle err, then png.Encode(writer, img) to export it.
```

`Options{}` selects medium error correction. `Low`, `Medium`, `Quartile`,
and `High` are supported; the level is a minimum, raised for free when the
payload still fits the chosen symbol version. Payloads use optimized
numeric/alphanumeric/byte/Kanji segments. Non-ASCII text carries an explicit
UTF-8 ECI header; ambiguous Kanji characters stay in byte segments so the
decoded text is preserved exactly. Equal-sized segmentation candidates use
the strongest available correction. `Options.MinVersion` and `MaxVersion`
bound the symbol version (1..40) — set both to the same value to pin an exact
size so changing content never reflows a layout; content that outgrows the
bound fails with `ErrCapacity`. Encoding is separate from presentation; the
library does not open files, fetch URLs, or save images itself.

## Composing the terminal component

```go
qr, err := qrcode.New(code, qrcode.Config{NextID: hostNextImageID})
// Handle err, retain qr, and run the commands returned by these methods:
cmd := tea.Batch(
    qr.SetSize(columns, rows),
    qr.SetTerminal(useKitty, cellPixelWidth, cellPixelHeight),
    qr.Update(msg),
)
// Render qr.View() as a whole; inspect qr.Err() for fit/layout failures.
// Before removing it, execute the command returned by qr.Close().
```

**Execute every returned command.** See the
[complete host example](./examples/qrcode/main.go) for batching, native/WASM
startup, and cleanup before quitting.

- Use an existing `picture.Model.Init()`/`Update()` for asynchronous terminal
  detection. Forward its `CellPixelSize()` and the user's graphics preference
  through `SetTerminal`. Unknown or unsupported Kitty capability uses glyphs.
- Broadcast messages to all components. Each filters its own image frames.
- `NextID` must allocate fresh positive 24-bit IDs shared with every picture
  in the application, including across replacement and resize. Do not reuse IDs.
- Execute commands from setters, `Update`, and `Close`. A model is a pointer
  with event-loop-owned state; do not copy it or mutate it concurrently.
- `SetCode(nil)` clears content. Encode replacements before setting them so
  invalid input can leave the old content intact. `Close` is idempotent and final.
- `errors.Is(qr.Err(), qrcode.ErrDoesNotFit)` identifies insufficient space.
  The component never truncates the matrix or removes the quiet zone.

The component returns content as a string so the host can compose it into
its own `tea.View`. It owns no keyboard bindings, title, border, or overlay.

## Rendering and limits

Kitty and exported images have square modules, integer scaling, sharp edges,
and an opaque white background. The glyph path renders directly from modules,
with explicit foreground and background colors. It approximates square modules
using whole columns and half-rows: the longer side is at most 9/8 of the shorter.
An 8×16 cell is assumed until measured geometry arrives. Unusual font geometry
can still need more terminal space.

**Apple Terminal** (and any terminal that draws `▀` from the font rather than
filling the exact half cell) misplaces half-block glyphs, which breaks finder
and alignment patterns. Set `Config{SolidCells: true}` there: modules fill
whole cells with solid color (2 columns × 1 row on 1:2 cells), so nothing
depends on glyph geometry, at four times the half-block area (about 66×33
cells for a short URL). The host owns the choice; the example enables it for
`TERM_PROGRAM=Apple_Terminal` or `NTCHARTS_QRCODE_SOLID=1` (for example inside tmux).

Input must be nonempty UTF-8, bounded to 7,089 bytes before UTF-8 validation
or encoding; actual capacity depends on content and correction level. Capacity errors wrap
`ErrCapacity`. `Image(scale)` accepts 1..16 with at most 2,048 pixels per edge.
Terminal rendering is bounded to 2,048-pixel edges, 256 Kitty cells per axis,
and 65,536 glyph cells. The display chooses at most eight pixels per module.

NTCharts **v2.4.0** is the tested baseline. The Kitty implementation deliberately
uses picture's exact-size `FitFill` path to avoid filtered resizing. Tests decode
the actual transmitted PNG and verify every pixel; retain those checks when
upgrading NTCharts. See [DEVELOP.md](./DEVELOP.md) for the dependency assessment.

Independent decoder tests cover images and reconstructed glyph output, Unicode,
capacity, quiet zones, scaling, resize, fallback, multiple instances, and cleanup.
Manual acceptance on 2026-10-06 (see [DEVELOP.md](./DEVELOP.md)) scanned the
example with a phone in Apple Terminal (solid cells) and iTerm2 (half-blocks),
and checked a Kitty-graphics terminal and tmux visually. Other terminals,
fonts, and phones were not tested. Decoding tests do not establish camera scanability. A QR code also does
not make a localhost URL reachable remotely.

Sixel, logos, styling, and decoding as a product feature are outside this scope.

## Development and browser demo

```sh
task ci                 # tests, race detector, vet, builds, vulnerability scans
task vuln               # native + WASM vulnerability scans (requires network)
task build-ex-qrcode    # bin/ntcharts-qrcode
task serve-wasm-site    # http://localhost:8000/ntcharts-qrcode/
task clean              # remove generated binaries and demo assets
```

Like ntcharts-osm, `wasm.work` selects the Bubble Tea and clipboard WASM forks;
native builds use upstream versions. The browser example uses go-booba. Its
generated assets are ignored. CI runs the same `task ci` checks as local
development, including a separate consumer module that catches dependency
replacement regressions. Dependabot checks Go modules and Actions weekly.
`task ci` also runs `task vuln`: a pinned `govulncheck` scans native code
(including tests) and the WASM workspace against the current Go vulnerability
database. Reachable vulnerabilities or scan failures fail CI; the scans
require network access.
The GitHub Pages workflow builds on pushes to `main` and can also be run
manually. It runs the full `task ci` checks on the revision being deployed
before uploading the site; failed checks prevent deployment. Enable Pages
with GitHub Actions before deploying. No hosted demo is assumed to be
deployed yet.

## License

[MIT License](./LICENSE.txt) — Copyright (c) 2026 [Neomantra Corp](https://www.neomantra.com).
Dependency notices are in [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md).

----
Made with :heart: and :fire: by the team behind [Nimble.Markets](https://nimble.markets).
