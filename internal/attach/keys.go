package attach

import (
	"bytes"
	"strconv"
)

// takeBackground removes the answers to this client's background queries
// from chunk. While controlling, it returns the colour scheme report for the
// last one: CSI ? 997 ; 1 n for dark, 2 for light. A reply split across reads
// is not recognised and reaches the session as input.
func (a *attachment) takeBackground(chunk []byte) ([]byte, []byte) {
	a.controlMu.Lock()
	defer a.controlMu.Unlock()
	var report []byte
	for a.probes > 0 {
		start, end, dark, ok := backgroundReply(chunk)
		if !ok {
			break
		}
		a.probes--
		chunk = append(chunk[:start:start], chunk[end:]...)
		if a.controlled && !a.claiming && a.schemeReports {
			report = []byte("\x1b[?997;2n")
			if dark {
				report = []byte("\x1b[?997;1n")
			}
		}
	}
	return chunk, report
}

// backgroundReply finds an OSC 11 answer, ESC ] 11 ; rgb:R/G/B ended by BEL
// or ST, in p and reports whether that colour is dark.
func backgroundReply(p []byte) (start, end int, dark, ok bool) {
	const prefix = "\x1b]11;rgb:"
	start = bytes.Index(p, []byte(prefix))
	if start < 0 {
		return 0, 0, false, false
	}
	body := p[start+len(prefix):]
	n := bytes.IndexAny(body, "\a\x1b")
	if n < 0 {
		return 0, 0, false, false
	}
	end = start + len(prefix) + n + 1
	if body[n] == 0x1b {
		if n+1 >= len(body) || body[n+1] != '\\' {
			return 0, 0, false, false
		}
		end++
	}
	parts := bytes.Split(body[:n], []byte("/"))
	if len(parts) != 3 {
		return 0, 0, false, false
	}
	luminance := 0.0
	for i, weight := range []float64{0.2126, 0.7152, 0.0722} {
		digits := len(parts[i])
		v, err := strconv.ParseUint(string(parts[i]), 16, 16)
		if err != nil || digits == 0 || digits > 4 {
			return 0, 0, false, false
		}
		luminance += weight * float64(v) / float64(uint64(1)<<(4*digits)-1)
	}
	return start, end, luminance < 0.5, true
}

// key describes one key in each encoding a native terminal may use.
type key struct {
	raw  byte   // legacy byte
	code string // kitty and modifyOtherKeys key code
	mods int    // exact modifier bits (Ctrl is 4)
	vk   int    // win32-input-mode virtual-key code
	char int    // win32-input-mode character
}

var (
	detachKey      = key{raw: DetachKey, code: "93", mods: 4, vk: 0xdd, char: DetachKey}
	takeControlKey = key{raw: TakeControlKey, code: "13", vk: 0x0d, char: TakeControlKey}
)

// detachIndex returns where the first Ctrl+] starts in chunk, or -1.
func detachIndex(chunk []byte) int { return keyIndex(chunk, detachKey) }

// takeControlIndex returns where the first Enter starts in chunk, or -1.
func takeControlIndex(chunk []byte) int { return keyIndex(chunk, takeControlKey) }

// keyIndex returns where the first press of k starts in chunk, or -1.
// Besides the raw byte it recognises the key as terminals encode it once a
// session's program enables xterm modifyOtherKeys (CSI 27;mods;code~), the
// kitty keyboard protocol (CSI code[:alt][;mods[:event][;text]]u) or Windows
// Terminal's win32-input-mode (CSI Vk;Sc;Uc;Kd;Cs;Rc_); those modes reach
// the native terminal with the session output.
func keyIndex(chunk []byte, k key) int {
	for i := 0; i < len(chunk); i++ {
		switch {
		case chunk[i] == k.raw:
			return i
		case chunk[i] == 0x1b && i+1 < len(chunk) && chunk[i+1] == '[':
			if isEncodedKey(chunk[i+2:], k) {
				return i
			}
		}
	}
	return -1
}

// isEncodedKey reports whether the CSI parameters and final byte at the
// start of p are a press (not a release) of k with exactly its modifiers.
func isEncodedKey(p []byte, k key) bool {
	end := 0
	for end < len(p) && (p[end] >= '0' && p[end] <= '9' || p[end] == ';' || p[end] == ':') {
		end++
	}
	if end == len(p) {
		return false
	}
	fields := bytes.Split(p[:end], []byte(";"))
	var code, modifiers []byte
	switch p[end] {
	case 'u':
		code = fields[0]
		if len(fields) > 1 {
			modifiers = fields[1]
		}
	case '~':
		if len(fields) != 3 || string(fields[0]) != "27" {
			return false
		}
		code, modifiers = fields[2], fields[1]
	case '_':
		return isWin32Key(fields, k)
	default:
		return false
	}
	code, _, _ = bytes.Cut(code, []byte(":"))
	modifiers, event, _ := bytes.Cut(modifiers, []byte(":"))
	m := 1
	if len(modifiers) > 0 {
		var ok bool
		if m, ok = atoi(modifiers); !ok {
			return false
		}
	}
	// Modifiers are 1+bits; ignore Caps Lock (64) and Num Lock (128).
	return string(code) == k.code && (m-1)&^(64|128) == k.mods && string(event) != "3"
}

// Control key state bits of a win32-input-mode event.
const (
	win32Alt   = 0x01 | 0x02
	win32Ctrl  = 0x04 | 0x08
	win32Shift = 0x10
)

// isWin32Key matches a win32-input-mode key-down event by virtual key or
// character; omitted parameters default to 0, except repeat count.
func isWin32Key(fields [][]byte, k key) bool {
	var v [6]int
	for i, f := range fields {
		if i == len(v) {
			return false
		}
		if len(f) == 0 {
			continue
		}
		n, ok := atoi(f)
		if !ok {
			return false
		}
		v[i] = n
	}
	vk, char, down, state := v[0], v[2], v[3], v[4]
	wantCtrl := k.mods&4 != 0
	if down != 1 || state&(win32Alt|win32Shift) != 0 || (state&win32Ctrl != 0) != wantCtrl {
		return false
	}
	return vk == k.vk || char == k.char
}

func atoi(b []byte) (int, bool) {
	if len(b) == 0 || len(b) > 4 {
		return 0, false
	}
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}
