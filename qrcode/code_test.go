package qrcode

import (
	"errors"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/makiuchi-d/gozxing"
	decoder "github.com/makiuchi-d/gozxing/qrcode"
)

func decode(t *testing.T, img image.Image, want string) {
	t.Helper()
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		t.Fatal(err)
	}
	// The images are generated, axis-aligned symbols, so use ZXing's pure
	// extraction path. Its perspective detector can mislocate alignment marks
	// in dense one-pixel-per-module codes; data/ECC decoding remains independent.
	result, err := decoder.NewQRCodeReader().Decode(bitmap, map[gozxing.DecodeHintType]interface{}{gozxing.DecodeHintType_PURE_BARCODE: true})
	if err != nil {
		t.Fatalf("independent decode of %q: %v", want, err)
	}
	if got := result.GetText(); got != want {
		t.Fatalf("decoded %q, want exact payload %q", got, want)
	}
}

func TestNormalDetectorFindsDefaultImage(t *testing.T) {
	payload := "https://nimblemarkets.github.io/gloss/docs/"
	img, _ := encoded(t, payload, Medium).Image(8)
	bitmap, _ := gozxing.NewBinaryBitmapFromImage(img)
	result, err := decoder.NewQRCodeReader().Decode(bitmap, nil)
	if err != nil || result.GetText() != payload {
		t.Fatalf("normal finder-pattern detection: %v", err)
	}
}

func encoded(t *testing.T, payload string, level Level) *Code {
	t.Helper()
	c, err := Encode(payload, Options{Level: level})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestImagesRoundTrip(t *testing.T) {
	for _, payload := range []string{
		"https://nimblemarkets.github.io/gloss/docs/", "01234567890123456789",
		"Café — 東京 / مرحبا / 🐈", "日本語", "\x00binary byte in UTF-8",
		strings.Repeat("longer payload, 0123456789; ", 35),
	} {
		for _, level := range []Level{Low, Medium, Quartile, High} {
			code := encoded(t, payload, level)
			for _, scale := range []int{1, 3, 8} {
				img, err := code.Image(scale)
				if err != nil {
					t.Fatal(err)
				}
				decode(t, img, payload)
				// Every source pixel must be opaque and exactly its module color.
				for y := range img.Bounds().Dy() {
					for x := range img.Bounds().Dx() {
						want := color.Gray{Y: 255}
						if code.modules[y/scale][x/scale] {
							want.Y = 0
						}
						if img.At(x, y) != want {
							t.Fatal("interpolated or non-opaque pixel")
						}
					}
				}
			}
		}
	}
}

func TestQuietZoneAndMatrixOwnership(t *testing.T) {
	c := encoded(t, "quiet zone", Medium)
	n := len(c.modules)
	for y, row := range c.Matrix() {
		for x, dark := range row {
			if (x < QuietZone || y < QuietZone || x >= n-QuietZone || y >= n-QuietZone) && dark {
				t.Fatalf("quiet zone is dark at %d,%d", x, y)
			}
		}
	}
	copy := c.Matrix()
	copy[0][0] = true
	copy[QuietZone][QuietZone] = false
	if c.Matrix()[0][0] || !c.Matrix()[QuietZone][QuietZone] {
		t.Fatal("returned matrix aliases the code")
	}
}

func TestEncodingBoundsAndOptions(t *testing.T) {
	for _, payload := range []string{"", "\xff"} {
		if _, err := Encode(payload, Options{}); err == nil {
			t.Fatal("accepted empty or invalid UTF-8")
		}
	}
	if _, err := Encode("x", Options{Level: 255}); err == nil {
		t.Fatal("accepted invalid correction")
	}
	for _, payload := range []string{
		strings.Repeat("1", MaxPayloadBytes+1),
		strings.Repeat("1", MaxPayloadBytes) + "\xff", // Reject size before UTF-8 validation.
		strings.Repeat("x", 3000),
	} {
		if _, err := Encode(payload, Options{}); !errors.Is(err, ErrCapacity) {
			t.Fatalf("capacity error: %v", err)
		}
	}
	large := encoded(t, strings.Repeat("1", MaxPayloadBytes), Low)
	if len(large.modules) != 185 {
		t.Fatalf("maximum QR edge: %d", len(large.modules))
	}
	for _, scale := range []int{0, -1, MaxScale + 1, int(^uint(0) >> 1)} {
		if _, err := large.Image(scale); err == nil {
			t.Fatalf("accepted scale %d", scale)
		}
	}
	if _, err := large.Image(MaxScale); err == nil {
		t.Fatal("ignored image edge bound")
	}
	if _, err := (&Code{}).Image(1); err == nil {
		t.Fatal("accepted zero-value code")
	}
}

func TestVersionBoundsAndBoost(t *testing.T) {
	// A floor keeps small payloads in a larger symbol: version 10 has a
	// 57-module edge before the quiet zone.
	floor, err := Encode("hi", Options{Level: Low, MinVersion: 10})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(floor.modules); n != 57+2*QuietZone {
		t.Fatalf("version floor: edge %d", n)
	}
	img, err := floor.Image(2)
	if err != nil {
		t.Fatal(err)
	}
	decode(t, img, "hi")

	// An exact pin fixes the symbol size; overflowing it is a capacity error.
	pinned, err := Encode("hi", Options{Level: High, MinVersion: 1, MaxVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(pinned.modules); n != 21+2*QuietZone {
		t.Fatalf("pinned version: edge %d", n)
	}
	if _, err := Encode(strings.Repeat("payload ", 20), Options{MinVersion: 1, MaxVersion: 1}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("pinned overflow: %v", err)
	}
	for _, opts := range []Options{{MinVersion: 41}, {MaxVersion: 41}, {MinVersion: -1}, {MinVersion: 5, MaxVersion: 4}} {
		if _, err := Encode("hi", opts); err == nil {
			t.Fatalf("accepted invalid version bounds %+v", opts)
		}
	}

	// Correction is raised for free when the chosen version has spare
	// capacity: a tiny payload stays in version 1 at every requested level.
	for _, level := range []Level{Low, Medium, Quartile, High} {
		if n := len(encoded(t, "hi", level).modules); n != 21+2*QuietZone {
			t.Fatalf("level %d changed the symbol version: edge %d", level, n)
		}
	}
}

func TestAmbiguousKanjiRoundTrip(t *testing.T) {
	for _, character := range "\\¢£¬‖−〜" {
		for _, payload := range []string{
			string(character),
			"東京" + strings.Repeat(string(character), 3) + "大阪",
			"https://example.org/東京/" + string(character),
		} {
			t.Run(payload, func(t *testing.T) {
				for _, level := range []Level{Low, Medium, Quartile, High} {
					img, err := encoded(t, payload, level).Image(3)
					if err != nil {
						t.Fatal(err)
					}
					decode(t, img, payload)
				}
			})
		}
	}
}

func TestSameSizeCandidatesPreferStrongerCorrection(t *testing.T) {
	for _, payload := range []string{
		"a£a",    // The whole-byte candidate fits H; split runs only fit Q.
		"12345£", // Numeric compression fits H; the whole-byte candidate only fits Q.
	} {
		for _, level := range []Level{Low, Medium, Quartile, High} {
			code := encoded(t, payload, level)
			if len(code.Matrix()) != 21+2*QuietZone {
				t.Fatalf("%q at level %d: stronger correction increased the version", payload, level)
			}
			img, _ := code.Image(3)
			bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
			if err != nil {
				t.Fatal(err)
			}
			result, err := decoder.NewQRCodeReader().Decode(bitmap, map[gozxing.DecodeHintType]interface{}{gozxing.DecodeHintType_PURE_BARCODE: true})
			if err != nil {
				t.Fatal(err)
			}
			if result.GetText() != payload {
				t.Fatalf("decoded %q, want %q", result.GetText(), payload)
			}
			if got := result.GetResultMetadata()[gozxing.ResultMetadataType_ERROR_CORRECTION_LEVEL]; got != "H" {
				t.Fatalf("%q at level %d: correction %v, want H", payload, level, got)
			}
		}
	}
}

func TestUnicodeSegmentationCapacity(t *testing.T) {
	for _, payload := range []string{
		strings.Repeat("1", 7000) + "£", // Must preserve numeric compression.
		strings.Repeat("a£", 800),       // Must fall back to a single byte segment.
	} {
		img, err := encoded(t, payload, Low).Image(3)
		if err != nil {
			t.Fatal(err)
		}
		decode(t, img, payload)
	}
	// 16 bytes fit version 1/M without ECI, but the UTF-8 ECI requires version 2.
	payload := strings.Repeat("é", 8)
	if _, err := Encode(payload, Options{MaxVersion: 1}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("ECI header ignored in pinned capacity: %v", err)
	}
	code := encoded(t, payload, Medium)
	if len(code.Matrix()) != 25+2*QuietZone {
		t.Fatalf("ECI capacity search chose edge %d, want version 2", len(code.Matrix()))
	}
	img, _ := code.Image(3)
	decode(t, img, payload)
}

func TestVersionSearchRanges(t *testing.T) {
	// Byte capacities at M exercise both character-count transitions, the
	// former >27 hang, and maxima inside (rather than at the end of) a range.
	for _, tc := range []struct{ length, version int }{
		{180, 9}, {181, 10}, {1059, 26}, {1060, 27}, {1126, 28}, {2331, 40},
	} {
		payload := strings.Repeat("a", tc.length)
		code, err := Encode(payload, Options{MaxVersion: tc.version})
		if err != nil {
			t.Fatalf("length %d, max version %d: %v", tc.length, tc.version, err)
		}
		if got := (len(code.Matrix()) - 2*QuietZone - 17) / 4; got != tc.version {
			t.Fatalf("length %d: version %d, want %d", tc.length, got, tc.version)
		}
		img, _ := code.Image(3)
		decode(t, img, payload)
		if _, err := Encode(payload, Options{MaxVersion: tc.version - 1}); !errors.Is(err, ErrCapacity) {
			t.Fatalf("length %d exceeded version bound: %v", tc.length, err)
		}
	}
}
