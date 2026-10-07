package qrcode

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	encoder "github.com/piglig/go-qr/v2"
)

// Safe runs should retain upstream's compression and ECC boosting at each
// character-count transition. Ambiguous mappings are covered by the independent
// decoder regressions in code_test.go; upstream is not their reference decoder.
func TestSegmentationMatchesUpstreamV2(t *testing.T) {
	payloads := []string{
		"https://example.com/order/12345678901234567890",
		strings.Repeat("a111111", 5), strings.Repeat("1234567890", 150),
		strings.Repeat("HELLO WORLD 1234", 40), strings.Repeat("日本語123abc", 40),
		strings.Repeat("café🙂1234567890", 20), strings.Repeat("a", 1060),
	}
	for i, payload := range payloads {
		for _, level := range []encoder.ECC{encoder.ECCLow, encoder.ECCMedium, encoder.ECCQuartile, encoder.ECCHigh} {
			for _, bounds := range [][2]int{{1, 9}, {10, 26}, {27, 40}, {1, 40}} {
				t.Run(fmt.Sprintf("payload%d/%s/%d-%d", i, level, bounds[0], bounds[1]), func(t *testing.T) {
					got, err := encodeSymbol(payload, level, bounds[0], bounds[1])
					want, wantErr := encoder.Encode(payload, encoder.WithECC(level), encoder.WithVersionRange(bounds[0], bounds[1]), encoder.WithUTF8ECI())
					if wantErr != nil {
						if !errors.Is(wantErr, encoder.ErrDataTooLong) || !errors.Is(err, encoder.ErrDataTooLong) {
							t.Fatalf("errors: local %v, upstream %v", err, wantErr)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if got.Version() != want.Version() || got.ECC() != want.ECC() {
						t.Fatalf("local %d-%s, upstream %d-%s", got.Version(), got.ECC(), want.Version(), want.ECC())
					}
				})
			}
		}
	}
}
