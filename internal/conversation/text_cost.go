package conversation

import "unicode/utf8"

// escapedTextBytes matches encoding/json's default string escaping, excluding
// the surrounding quotes. UTF-8 bytes are conservatively charged as tokens.
func escapedTextBytes(text string) int64 {
	var cost int64
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		text = text[size:]
		if r == utf8.RuneError && size == 1 {
			cost += 6 // encoding/json replaces an invalid byte with \ufffd.
			continue
		}
		switch r {
		case '"', '\\', '\b', '\f', '\n', '\r', '\t':
			cost += 2
		case '<', '>', '&', '\u2028', '\u2029':
			cost += 6
		default:
			if r < 0x20 {
				cost += 6
			} else {
				cost += int64(utf8.RuneLen(r))
			}
		}
	}
	return cost
}
