package qrcode

import (
	"errors"
	"strings"
	"unicode/utf8"

	encoder "github.com/piglig/go-qr"
)

// These JIS X 0208 characters map to different Unicode characters in common
// Shift-JIS decoders (including ZXing). Keep them out of Kanji segments.
const ambiguousKanji = "\\¢£¬‖−〜"

func encodeSymbol(content string, level encoder.Ecc, minV, maxV int) (*encoder.QrCode, error) {
	var prefix []*encoder.QrSegment
	for _, r := range content {
		if r >= utf8.RuneSelf {
			eci, err := encoder.MakeEci(26) // Explicit UTF-8 for every byte segment.
			if err != nil {
				return nil, err
			}
			prefix = append(prefix, eci)
			break
		}
	}
	// Each candidate appends to the same ECI prefix; prevent one append from
	// overwriting another candidate's segments through spare slice capacity.
	prefix = prefix[:len(prefix):len(prefix)]

	// Splitting around ambiguous characters can add more headers than a single
	// byte segment. Retain that alternative, so segmentation cannot reject text
	// that fits in byte mode or inflate its symbol size.
	var fallback *encoder.QrCode
	var byteSegments []*encoder.QrSegment
	if strings.ContainsAny(content, ambiguousKanji) {
		segment, err := encoder.MakeBytes([]byte(content))
		if err != nil {
			return nil, err
		}
		byteSegments = append(prefix, segment)
		fallback, err = encoder.EncodeSegments(byteSegments, level, minV, maxV, -1, true)
		if err != nil && !errors.Is(err, encoder.ErrDataTooLong) {
			return nil, err
		}
		if fallback != nil {
			maxV = (fallback.Size() - 17) / 4
		}
	}

	// Character-count widths, and hence optimal segmentation, only change at
	// versions 10 and 27. Ask upstream for an EXACT version at each range's end:
	// v1.1.0's multi-version search can loop forever. EncodeSegments performs
	// the bounded capacity search within each range, including the ECI header.
	for _, boundary := range [...]int{9, 26, 40} {
		end := min(boundary, maxV)
		if end < minV {
			continue
		}
		segments, err := safeSegments(content, level, end)
		if err == nil {
			segments = append(prefix, segments...)
			var qr *encoder.QrCode
			qr, err = encoder.EncodeSegments(segments, level, minV, end, -1, true)
			if err == nil {
				if fallback != nil && fallback.Size() == qr.Size() {
					return strongestAtVersion(qr, level, segments, byteSegments)
				}
				return qr, nil
			}
		}
		if !errors.Is(err, encoder.ErrDataTooLong) {
			return nil, err
		}
		minV = end + 1
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, encoder.ErrDataTooLong
}

// Size wins first. For equal-sized candidates, ask the encoder for the
// strongest correction either segmentation can fit at that exact version.
// QrCode exposes no correction-level accessor; using EncodeSegments keeps
// this independent of its private fields and QR format-bit positions.
func strongestAtVersion(current *encoder.QrCode, minimum encoder.Ecc, candidates ...[]*encoder.QrSegment) (*encoder.QrCode, error) {
	version := (current.Size() - 17) / 4
	for _, level := range [...]encoder.Ecc{encoder.High, encoder.Quartile, encoder.Medium} {
		if level <= minimum {
			break
		}
		for _, segments := range candidates {
			qr, err := encoder.EncodeSegments(segments, level, version, version, -1, false)
			if err == nil {
				return qr, nil
			}
			if !errors.Is(err, encoder.ErrDataTooLong) {
				return nil, err
			}
		}
	}
	return current, nil
}

// Optimize safe runs and encode consecutive ambiguous characters as UTF-8.
// Splitting only at those boundaries preserves compression of long numeric,
// alphanumeric, and Japanese runs surrounding an ambiguous character.
func safeSegments(content string, level encoder.Ecc, version int) ([]*encoder.QrSegment, error) {
	var segments []*encoder.QrSegment
	for len(content) > 0 {
		end := strings.IndexAny(content, ambiguousKanji)
		if end < 0 {
			end = len(content)
		}
		if end > 0 {
			run, err := encoder.MakeSegmentsOptimally(content[:end], level, version, version)
			if err != nil {
				return nil, err
			}
			segments = append(segments, run...)
		} else {
			for _, r := range content {
				if !strings.ContainsRune(ambiguousKanji, r) {
					break
				}
				end += utf8.RuneLen(r)
			}
			run, err := encoder.MakeBytes([]byte(content[:end]))
			if err != nil {
				return nil, err
			}
			segments = append(segments, run)
		}
		content = content[end:]
	}
	return segments, nil
}
