# `ntcharts-qrcode` CHANGELOG

## v0.1.1 (2026-10-07)

* Upgrade to [`piglig/go-qr`](https://github.com/piglig/go-qr) `v2.3.0`, including the version-search fix.
  Adapt segmentation to its new API while preserving ambiguous-Kanji byte
  encoding, UTF-8 ECI, version bounds, and correction boosting.

## v0.1.0 (2026-10-06)

Initial release. `ntcharts-qrcode` encodes QR codes and presents them inside
an existing Bubble Tea v2 event loop — with Kitty graphics or explicit
black/white Unicode half-blocks — as an experimental NTCharts companion
extracted from [gloss](https://github.com/NimbleMarkets/gloss).

* Bounded UTF-8 encoding, four correction levels, and configurable QR versions.
* Immutable module matrices and PNG-ready images with a four-module quiet zone.
* Composable Kitty and Unicode half-block rendering with fit checks,
  host-owned image IDs, and asynchronous cleanup.
* `Config.SolidCells` renders whole-cell modules for terminals, such as Apple
  Terminal, that misplace font-drawn half-block glyphs.
* Independent decoding tests, a two-code terminal example, and a WASM demo.
* Task-based CI and vulnerability scans, with Pages deployment gated on checks.
