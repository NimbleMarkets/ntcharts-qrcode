// Package qrcode encodes QR symbols and presents them inside an existing
// Bubble Tea event loop. It owns no terminal, input stream, or file output.
// Encoding of images is thanks to github.com/piglig/go-qr/v2
package qrcode

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"unicode/utf8"

	encoder "github.com/piglig/go-qr/v2"
)

// Level selects the minimum error correction. Medium (M, approximately 15%
// recovery) is the zero-value default. The encoder raises the level for free
// when the payload still fits the same symbol version at a higher level.
// Higher correction reduces the available capacity.
type Level uint8

const (
	Medium   Level = iota
	Low            // L, approximately 7%.
	Quartile       // Q, approximately 25%.
	High           // H, approximately 30%.
)

const (
	QuietZone = 4
	// MaxPayloadBytes bounds work before encoding. Actual capacity depends on
	// content and correction; 7,089 numeric digits is the largest QR payload.
	MaxPayloadBytes = 7089
	MaxImageEdge    = 2048
	MaxScale        = 16
)

var ErrCapacity = errors.New("payload exceeds QR capacity")

// Options tunes encoding. MinVersion and MaxVersion bound the QR symbol
// version (1..40, edge 17+4·version modules before the quiet zone). Zero
// selects the widest range. Set both to the same value to pin an exact size;
// content that outgrows it fails with ErrCapacity. The payload is split into
// optimized numeric/alphanumeric/byte/Kanji segments before encoding, with
// ambiguous Kanji mappings kept in UTF-8 byte segments.
type Options struct {
	Level      Level
	MinVersion int
	MaxVersion int
}

// Code is an immutable symbol. Its matrix includes the four-module white
// quiet zone on every side. True means a black module; false means white.
type Code struct{ modules [][]bool }

func Encode(content string, opts Options) (*Code, error) {
	levels := [...]encoder.ECC{encoder.ECCMedium, encoder.ECCLow, encoder.ECCQuartile, encoder.ECCHigh}
	if int(opts.Level) >= len(levels) {
		return nil, fmt.Errorf("invalid QR correction level %d", opts.Level)
	}
	minV, maxV := opts.MinVersion, opts.MaxVersion
	if minV == 0 {
		minV = encoder.MinVersion
	}
	if maxV == 0 {
		maxV = encoder.MaxVersion
	}
	if minV < encoder.MinVersion || maxV > encoder.MaxVersion {
		return nil, fmt.Errorf("QR versions must be within %d..%d", encoder.MinVersion, encoder.MaxVersion)
	}
	if minV > maxV {
		return nil, fmt.Errorf("QR minimum version %d exceeds maximum %d", minV, maxV)
	}
	if len(content) > MaxPayloadBytes {
		return nil, ErrCapacity
	}
	if content == "" || !utf8.ValidString(content) {
		return nil, errors.New("QR content must be nonempty UTF-8")
	}
	ecl := levels[opts.Level]
	qr, err := encodeSymbol(content, ecl, minV, maxV)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCapacity, err)
	}
	// go-qr's matrix excludes the quiet zone; add it so Code owns the contract.
	n := qr.Size()
	modules := make([][]bool, n+2*QuietZone)
	for y := range modules {
		row := make([]bool, n+2*QuietZone)
		if y >= QuietZone && y < n+QuietZone {
			for x := range n {
				row[x+QuietZone] = qr.Module(x, y-QuietZone)
			}
		}
		modules[y] = row
	}
	return &Code{modules: modules}, nil
}

// Matrix returns a copy, including the quiet zone, so callers cannot change
// future renderings of the code.
func (c *Code) Matrix() [][]bool {
	rows := make([][]bool, len(c.modules))
	for y, row := range c.modules {
		rows[y] = append([]bool(nil), row...)
	}
	return rows
}

// Image returns an opaque black/white image, suitable for png.Encode. Scale
// is an integer number of pixels per module (1..16); the edge is at most
// 2,048 pixels. It never interpolates, crops, or removes the quiet zone.
func (c *Code) Image(scale int) (image.Image, error) {
	if c == nil || len(c.modules) == 0 {
		return nil, errors.New("no QR code")
	}
	if scale < 1 || scale > MaxScale || len(c.modules)*scale > MaxImageEdge {
		return nil, fmt.Errorf("QR scale must be 1..%d with an edge at most %d pixels", MaxScale, MaxImageEdge)
	}
	side := len(c.modules) * scale
	out := image.NewGray(image.Rect(0, 0, side, side))
	for y := range side {
		for x := range side {
			if !c.modules[y/scale][x/scale] {
				out.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	return out, nil
}
