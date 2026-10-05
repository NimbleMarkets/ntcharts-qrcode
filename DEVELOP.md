# Developing ntcharts-qrcode

Run `task ci` before submitting changes. The module follows the sibling
NTCharts projects: public component in `qrcode/`, a runnable example in
`examples/qrcode/`, native Task builds, and a booba WASM demo.

The component was extracted from gloss; application overlays, key bindings,
URL selection, and export-file policy remain in gloss.

## Component and dependency contract

`qrcode.Encode(text, Options{Level: Medium})` returns an immutable
`Code`. `Matrix()` returns a copy including four white quiet-zone modules on
every side; `Image(scale)` returns an opaque black/white `image.Image` for
`png.Encode`. Medium (M) is the default minimum; Low (L), Quartile (Q), and
High (H) are supported, and the encoder raises the level for free when the
payload still fits the chosen symbol version. Numeric, alphanumeric, byte,
and Kanji runs are optimized; non-ASCII payloads carry a UTF-8 ECI header.
Characters with ambiguous Kanji mappings stay in UTF-8 byte segments.
`Options.MinVersion` and
`MaxVersion` bound the symbol version (1..40); setting both pins an exact
size. Empty/invalid UTF-8 and invalid options are errors. Input is
bounded to 7,089 bytes before UTF-8 validation or encoding; actual capacity
depends on content and correction. Capacity errors wrap `ErrCapacity`. Image scale is an integer
1..16 and the edge cannot exceed 2,048 pixels.

`New(code, Config{NextID: allocator})` constructs the picture-style component.
The allocator must return fresh positive 24-bit IDs shared with the host's other
pictures; gloss passes `nextKittyID`. Drive `SetCode`, `SetSize`, `SetTerminal`,
and `Update` from the existing Bubble Tea loop and execute their returned
commands. Render `View()` as a whole. `Err()` wraps `ErrDoesNotFit` when there
is insufficient room; no partial QR is rendered. Before removal call `Close()`
and execute its cleanup command. It is idempotent and rejects late frames.

There is no terminal Init, input read, mode change, or synchronous probe.
The host's picture component performs normal capability detection; the QR
component receives that choice and measured cell size. Unknown/unsupported
Kitty falls back to explicit black/white Unicode half-blocks built directly
from modules. Glyphs use the smallest integer column/half-row replication
whose longer module side is at most 9/8 of its shorter side. This slight
aspect tolerance avoids enormous layouts for ordinary sizes such as 8×17;
glyph modules are approximately square, with sharp edges and no resampling.
The initial 8×16 cell estimate assumes a 1:2 font. Unusual measured ratios
can still require more space. Kitty and PNG modules remain exactly square.

For Kitty, the component builds an opaque bitmap with square, integer-sized
modules (up to eight pixels each), padded white to exactly fill its cell
rectangle. NTCharts v2.4.0 normally uses Catmull–Rom resizing; its `FitFill`
fast path bypasses this only with exact source/target sizes and transparent
configured background. The QR source itself remains opaque. Resolution stays
1.0. Tests inspect and independently decode the transmitted PNG, guarding this
dependency behavior. Until the frame is transmitted, the component shows a
preparation message instead of picture's resampled transitional glyph view.
Content/layout changes get fresh IDs and delete the retired image using
picture's existing commands and tmux handling. Presentation allocations are
bounded to 2,048-pixel edges, 256 Kitty cells per axis, or 65,536 glyph cells.

Encoder review (October 2026):

- [piglig/go-qr](https://github.com/piglig/go-qr) v1.1.0 is the encoder:
  MIT-licensed pure Go with no external runtime dependencies, implementing QR
  Model 2 versions 1–40, four correction levels, optimized mixed-mode
  segmentation, mask selection, and Reed–Solomon correction. It is a young,
  single-author project; we accept that risk because the decode-based tests
  below validate every symbol independently, and a differential corpus against
  skip2/go-qrcode (decode round-trips plus symbol sizes, 176 cases) matched.
  We use only its segment/matrix API and own the quiet zone, bounded image,
  and terminal presentation layers.
- Its v1.1.0 `MakeSegmentsOptimally` can hang during multi-version searches
  (capacity is only checked at versions 1/10/27). `qrcode/segments.go` calls it
  with an exact version at the end of each character-count range (1–9,
  10–26, 27–40), then uses the bounded `EncodeSegments` search within that
  range. This requires at most three segmentation passes and respects
  caller-supplied version bounds. There is no encoder `replace` directive:
  the fix ships in this library, so downstream consumers receive it too.
- Its Kanji table has seven Unicode mappings that differ from common
  Shift-JIS decoders: backslash, `¢`, `£`, `¬`, `‖`, `−`, and `〜`. We split
  around those characters and encode them in byte mode, preserving optimized
  runs on either side. A whole-payload byte alternative prevents splitting
  overhead from inflating the symbol or rejecting byte-encodable text.
  When both candidates use the same version, we choose the strongest
  correction either can fit, keeping the version fixed. Tests independently
  decode both the payload and the correction level in each selection direction.
  Non-ASCII payloads include UTF-8 ECI assignment 26, counted toward capacity.
  Regression tests decode individual characters and mixed Japanese/text
  payloads, exercise numeric compression and byte fallback, and check the
  version transitions and ECI overhead.
- [skip2/go-qrcode](https://github.com/skip2/go-qrcode), the first encoder, is
  unmaintained since 2024 and lacks optimal segmentation and correction
  boosting. Its `Bitmap()` included the quiet zone; the wrapper now adds it.
- [yeqown/go-qrcode](https://github.com/yeqown/go-qrcode) was also considered:
  it has recent maintenance (August 2026); its v2 module has external
  Reed–Solomon and x/text dependencies and a broader
  customization surface. Style and logos are out of scope.
- [gozxing](https://github.com/makiuchi-d/gozxing) v0.1.1 is the independent,
  MIT-licensed pure-Go decoder used only in tests. It is not linked into gloss.
  Tests cover Unicode, all correction levels, version bounds, capacity, quiet
  zones, integer scaling, reconstructed colored half-blocks, actual Kitty PNG
  bytes, fallback, resize, content replacement, separate instances, and
  cleanup. Dense one-pixel images use ZXing's pure-symbol extraction path;
  ordinary finder-pattern detection is also tested on a default-size URL image.

## Automation and companion conventions

GitHub Actions uses `actions/checkout@v7`, `actions/setup-go@v7`, and
`go-task/setup-task@v2`, following the main NTCharts repo. CI invokes `task ci`
so local and hosted checks stay in sync. `TestDownstreamConsumer` creates a
separate main module with `GOWORK=off` and only a local replacement for this
library; it verifies dense encoding and capacity rejection without inheriting
any of the library's dependency replacements.

`task ci` includes `task vuln`, which installs `govulncheck` v1.8.0 into the
ignored `.task/bin/` directory. The scanner runs as a host executable, first
against native packages and tests with `GOWORK=off`, then against js/wasm
packages using `wasm.work` so the WASM forks are covered. The scanner version
is pinned in Taskfile.yml; the vulnerability database stays current. Both
reachable vulnerability findings and scanner/network failures stop CI and
therefore prevent Pages deployment. Run `task vuln` on its own for a security
check; `task test` remains available without the vulnerability-database query.

Like the SVG/PDF companions, weekly Dependabot checks cover Actions and Go
modules. Task targets include `go-update`, `clean`, and `clean-wasm-site`;
`serve-wasm-site` serves `/ntcharts-qrcode/` to match the GitHub Pages prefix.
Pages builds on `main` pushes or manual dispatch, with deployment permissions
limited to the deploy job. Its build job runs the full `task ci` checks on
the selected revision before uploading the site, so automatic and manual
deployments require passing checks. Repository Pages settings must use
GitHub Actions.

## Manual acceptance checks

Run the example in a Kitty-capable terminal and a glyph-only terminal, then
through tmux with its normal graphics setup. Resize and change fonts, switch
render modes, change content, remove/restore the second code, and quit. Check
for stale images and scan both codes with a phone. Record terminal, font, cell
geometry, transport, and exact decoded URL. These checks have not yet been
completed for the extraction; automated decoders are not a substitute.
