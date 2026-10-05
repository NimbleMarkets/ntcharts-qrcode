# ntcharts-qrcode

An experimental NTCharts companion for QR codes in Bubble Tea v2, extracted
from gloss. Module: `github.com/NimbleMarkets/ntcharts-qrcode`, Go 1.26.8+.

## Layout

- `qrcode`: encoding, image generation, and terminal component. No application dependencies.
- `examples/qrcode`: two-component native/WASM demo using the public API.
- `web`: booba browser demo shell; generated assets are ignored.
- `wasm.work`: Bubble Tea and clipboard forks for WASM only.
- `internal/tools/notices`: generates demo dependency licenses with `task notices`.

## Conventions

Keep the public API small. Encoding and presentation are separate types.
The component never reads stdin, takes over the terminal, or synchronously
probes capabilities. Hosts own the event loop, terminal detection, and the
shared image-ID allocator. Run every returned command, including `Close()`.

Keep black modules on opaque white with a four-module quiet zone. Never crop
a QR or filter its edges. Kitty and image modules are exactly square; glyphs
may approximate the cell geometry within the documented 9/8 side ratio.
Preserve allocation and input bounds. Sixel, logos, styling, and product
decoding are out of scope. The decoder dependency is for tests only.

NTCharts picture's exact-size `FitFill` path must bypass resampling. Keep the
tests that decode actual Kitty PNG bytes and reconstructed glyph output;
they protect this dependency contract when upgrading NTCharts.

## Verification

Run `task ci` before finishing: formatting, tidy/verify, race tests, vet,
Windows build, native examples, WASM build, and native/WASM vulnerability
scans (network required). Use `task test` and
`task build-ex-qrcode` during development. Follow the sibling NTCharts
projects' MIT license, Task, examples, and documentation conventions.

Keep README, DEVELOP, and CHANGELOG consistent. Never represent automated
decoding as an actual phone scan. Terminal/font/tmux and camera checks remain
manual acceptance checks until someone performs and records them.
