// Optimal segmentation adapted from piglig/go-qr v2.3.0 (MIT).
// Copyright (c) 2023 piglig
// See THIRD_PARTY_NOTICES.md for the license.
package qrcode

import (
	"strings"
	"unicode/utf8"

	encoder "github.com/piglig/go-qr/v2"
)

// optimalSegments optimizes a run with ambiguous Kanji characters removed.
// Upstream v2 keeps its optimizer private; explicit segments let us preserve
// interoperable byte encoding at those boundaries.
func optimalSegments(text string, version int) ([]encoder.Segment, error) {
	runes := []rune(text)
	modes := charModes(runes, version)
	var segments []encoder.Segment
	start, pos := 0, 0
	for i, r := range runes {
		pos += utf8.RuneLen(r)
		if i+1 < len(runes) && modes[i+1] == modes[i] {
			continue
		}
		var segment encoder.Segment
		var err error
		switch modes[i] {
		case encoder.ModeNumeric:
			segment, err = encoder.NumericSegment(text[start:pos])
		case encoder.ModeAlphanumeric:
			segment, err = encoder.AlphanumericSegment(text[start:pos])
		case encoder.ModeKanji:
			segment, err = encoder.KanjiSegment(text[start:pos])
		default:
			segment = encoder.BytesSegment([]byte(text[start:pos]))
		}
		if err != nil {
			return nil, err
		}
		segments = append(segments, segment)
		start = pos
	}
	return segments, nil
}

// optimalModes are the modes considered by charModes, in DP column order.
var optimalModes = [4]encoder.Mode{encoder.ModeByte, encoder.ModeAlphanumeric, encoder.ModeNumeric, encoder.ModeKanji}

// charModes returns the mode of every character in the cheapest encoding of
// runes at version ver. It is a dynamic program over (character, mode) pairs;
// costs are in sixths of a bit so fractional mode costs stay integral.
func charModes(runes []rune, ver int) []encoder.Mode {
	const n = len(optimalModes)
	versionRange := 0
	if ver >= 27 {
		versionRange = 2
	} else if ver >= 10 {
		versionRange = 1
	}
	countBits := [n][3]int{{8, 16, 16}, {9, 11, 13}, {10, 12, 14}, {8, 10, 12}}
	var head [n]int // cost of starting a new segment in each mode
	for j := range optimalModes {
		head[j] = (4 + countBits[j][versionRange]) * 6
	}

	// from[i*n+j] is the mode of character i on the cheapest path whose
	// character i is in mode j; 0 means character i cannot be in mode j.
	from := make([]encoder.Mode, len(runes)*n)
	prev := head
	for i, r := range runes {
		var cur [n]int
		step := from[i*n : i*n+n]

		cur[0] = prev[0] + utf8.RuneLen(r)*8*6
		step[0] = encoder.ModeByte
		if r < utf8.RuneSelf && strings.ContainsRune("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:", r) {
			cur[1] = prev[1] + 33
			step[1] = encoder.ModeAlphanumeric
		}
		if r < utf8.RuneSelf && r >= '0' && r <= '9' {
			cur[2] = prev[2] + 20
			step[2] = encoder.ModeNumeric
		}
		if _, err := encoder.KanjiSegment(string(r)); err == nil {
			cur[3] = prev[3] + 78
			step[3] = encoder.ModeKanji
		}

		// Switching from mode k to mode j after this character costs the
		// partial bit rounded up plus the new segment header.
		for j := 0; j < n; j++ {
			for k := 0; k < n; k++ {
				cost := (cur[k]+5)/6*6 + head[j]
				if step[k] != 0 && (step[j] == 0 || cost < cur[j]) {
					cur[j] = cost
					step[j] = optimalModes[k]
				}
			}
		}
		prev = cur
	}

	best := 0
	for j := 1; j < n; j++ {
		if prev[j] < prev[best] {
			best = j
		}
	}

	modes := make([]encoder.Mode, len(runes))
	mode := optimalModes[best]
	for i := len(runes) - 1; i >= 0; i-- {
		mode = from[i*n+modeColumn(mode)]
		modes[i] = mode
	}
	return modes
}

// modeColumn returns the index of m in optimalModes.
func modeColumn(m encoder.Mode) int {
	switch m {
	case encoder.ModeAlphanumeric:
		return 1
	case encoder.ModeNumeric:
		return 2
	case encoder.ModeKanji:
		return 3
	}
	return 0
}
