// Package settings owns server preferences and ordered change subscriptions.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

var (
	ErrUnavailable = errors.New("settings unavailable")
	ErrInvalid     = errors.New("invalid settings")
	ErrConflict    = errors.New("settings changed; read the current revision and retry")
)

type Preview struct {
	Lines int `json:"lines"`
	Skip  int `json:"skip"`
}
type CodeReader struct {
	LineNumbers  string `json:"lineNumbers"`
	DiffOverflow string `json:"diffOverflow"`
}
type Command struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
	Enter   bool   `json:"enter"`
}
type Values struct {
	Theme         *string             `json:"theme"`
	Preview       *Preview            `json:"preview"`
	CodeReader    *CodeReader         `json:"codeReader"`
	QuickCommands *[]Command          `json:"quickCommands"`
	HomeWorktrees map[string][]string `json:"homeWorktrees"`
	SessionPins   []string            `json:"sessionPins,omitempty"`
	ForceSoftKeys *bool               `json:"forceSoftKeys"`
	// IgnoreHighPorts suppresses automatic port-forwarding prompts and
	// detected-port rows for ports 30000 and above when nil or true;
	// omitempty keeps files written before this setting loadable.
	IgnoreHighPorts *bool `json:"ignoreHighPorts,omitempty"`
	// LegacyFontSize, LegacyChatIdleSleepMinutes and LegacyArtifactPrompt
	// only let files written before those settings were removed load under
	// strict decoding. Open discards them and they are never written back.
	LegacyFontSize             json.RawMessage `json:"fontSize,omitempty"`
	LegacyChatIdleSleepMinutes json.RawMessage `json:"chatIdleSleepMinutes,omitempty"`
	LegacyArtifactPrompt       json.RawMessage `json:"artifactPrompt,omitempty"`
}

type Snapshot struct {
	Revision  int64  `json:"revision"`
	UpdatedAt string `json:"updatedAt"`
	Values    Values `json:"values"`
}
type Changes struct {
	Fields map[string]bool
	Values Values
}
type Event struct {
	Snapshot  *Snapshot
	Revision  int64
	UpdatedAt string
	Changes   Changes
}
type storedFile struct {
	V int `json:"v"`
	Snapshot
}

// Zod's string limits count UTF-16 code units. Keep accepted API values
// readable by every browser, including non-BMP characters.
func validText(s string, max int) bool {
	if !utf8.ValidString(s) {
		return false
	}
	size := 0
	for _, r := range s {
		size += utf16.RuneLen(r)
	}
	return size <= max
}
func validate(f storedFile) error {
	if f.V != 1 || f.Revision < 0 || f.Revision > 9007199254740991 {
		return ErrInvalid
	}
	if f.Revision > 0 {
		if _, err := time.Parse(time.RFC3339Nano, f.UpdatedAt); err != nil {
			return ErrInvalid
		}
	} else if f.UpdatedAt != "" {
		return ErrInvalid
	}
	return validateValues(f.Values)
}

const maxSessionPins = 50

var sessionPinID = regexp.MustCompile(`^[a-z2-7]{8}$`)

func validateValues(v Values) error {
	if len(v.SessionPins) > maxSessionPins {
		return ErrInvalid
	}
	pins := make(map[string]bool, len(v.SessionPins))
	for _, id := range v.SessionPins {
		if !sessionPinID.MatchString(id) || pins[id] {
			return ErrInvalid
		}
		pins[id] = true
	}
	if v.Theme != nil && *v.Theme != "light" && *v.Theme != "dark" {
		return ErrInvalid
	}
	if v.Preview != nil && (v.Preview.Lines < 1 || v.Preview.Lines > 50 || v.Preview.Skip < 0 || v.Preview.Skip > 50) {
		return ErrInvalid
	}
	if v.CodeReader != nil && ((v.CodeReader.LineNumbers != "show" && v.CodeReader.LineNumbers != "hide") || (v.CodeReader.DiffOverflow != "scroll" && v.CodeReader.DiffOverflow != "wrap")) {
		return ErrInvalid
	}
	if v.QuickCommands != nil {
		if len(*v.QuickCommands) > 50 {
			return ErrInvalid
		}
		ids := map[string]bool{}
		for _, c := range *v.QuickCommands {
			if c.ID == "" || !validText(c.ID, 64) || ids[c.ID] || !validText(c.Name, 40) || strings.TrimSpace(c.Command) == "" || !validText(c.Command, 4096) {
				return ErrInvalid
			}
			ids[c.ID] = true
		}
	}
	if len(v.HomeWorktrees) > 200 {
		return ErrInvalid
	}
	total := 0
	for id, list := range v.HomeWorktrees {
		if id == "" || !validText(id, 128) || list == nil || len(list) > 1000 {
			return ErrInvalid
		}
		total += len(list)
		seen := map[string]bool{}
		for _, wid := range list {
			if wid == "" || !validText(wid, 128) || seen[wid] {
				return ErrInvalid
			}
			seen[wid] = true
		}
	}
	if total > 2000 {
		return ErrInvalid
	}
	return nil
}
func normalize(v Values) Values {
	v = cloneValues(v)
	if v.QuickCommands != nil {
		for i := range *v.QuickCommands {
			c := &(*v.QuickCommands)[i]
			c.Name = strings.TrimSpace(c.Name)
			c.Command = normalizeText(c.Command)
		}
	}
	return v
}

// normalizeText turns CRLF and CR into LF and drops trailing newlines.
func normalizeText(s string) string {
	return strings.TrimRight(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n"), "\n")
}
func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
func cloneValues(v Values) Values {
	v.SessionPins = slices.Clone(v.SessionPins)
	v.Theme = clonePtr(v.Theme)
	v.Preview = clonePtr(v.Preview)
	v.CodeReader = clonePtr(v.CodeReader)
	v.ForceSoftKeys = clonePtr(v.ForceSoftKeys)
	v.IgnoreHighPorts = clonePtr(v.IgnoreHighPorts)
	if v.QuickCommands != nil {
		list := append([]Command{}, (*v.QuickCommands)...)
		v.QuickCommands = &list
	}
	if v.HomeWorktrees != nil {
		m := map[string][]string{}
		for k, list := range v.HomeWorktrees {
			m[k] = append([]string{}, list...)
		}
		v.HomeWorktrees = m
	}
	return v
}
func clone(s Snapshot) Snapshot { s.Values = cloneValues(s.Values); return s }
func apply(v Values, c Changes) (Values, error) {
	if len(c.Fields) == 0 {
		return v, ErrInvalid
	}
	for key := range c.Fields {
		switch key {
		case "theme":
			v.Theme = c.Values.Theme
		case "preview":
			v.Preview = c.Values.Preview
		case "codeReader":
			v.CodeReader = c.Values.CodeReader
		case "quickCommands":
			v.QuickCommands = c.Values.QuickCommands
		case "homeWorktrees":
			v.HomeWorktrees = c.Values.HomeWorktrees
		case "sessionPins":
			v.SessionPins = c.Values.SessionPins
		case "forceSoftKeys":
			v.ForceSoftKeys = c.Values.ForceSoftKeys
		case "ignoreHighPorts":
			v.IgnoreHighPorts = c.Values.IgnoreHighPorts
		default:
			return v, ErrInvalid
		}
	}
	v = normalize(v)
	if c.Fields["quickCommands"] && !validCommandNames(v.QuickCommands) {
		return v, ErrInvalid
	}
	return v, validateValues(v)
}

// legacyDirPlaceholder matches the artifacts folder placeholder: the current
// {{dir}} and the single-brace {dir} older versions wrote.
var legacyDirPlaceholder = regexp.MustCompile(`\{\{dir\}\}|\{dir\}`)

// nameLegacyCommands makes every command name usable in a {{name}}
// reference: a command saved before names were required (empty, repeated or
// containing braces) is named from its name or text, so every list a client
// reads can be saved back. The stored file changes only with the next write
// of the list. A {dir} placeholder in a text becomes {{dir}}.
func nameLegacyCommands(list *[]Command) *[]Command {
	if list == nil {
		return nil
	}
	named := append([]Command{}, (*list)...)
	// "user", "dir" and "webmux" are reserved by initialization instructions.
	seen := map[string]bool{"user": true, "dir": true, "webmux": true}
	for i := range named {
		named[i].Command = legacyDirPlaceholder.ReplaceAllString(named[i].Command, "{{dir}}")
		name := named[i].Name
		if name == "" || seen[name] || strings.ContainsAny(name, "{}\n") {
			source := name
			if strings.TrimSpace(source) == "" {
				source = named[i].Command
			}
			first, _, _ := strings.Cut(strings.TrimSpace(source), "\n")
			base := strings.TrimSpace(strings.NewReplacer("{", "", "}", "").Replace(first))
			if base == "" {
				base = "未命名"
			}
			base = truncateUTF16(base, 30)
			name = base
			for n := 2; seen[name]; n++ {
				name = fmt.Sprintf("%s %d", base, n)
			}
			named[i].Name = name
		}
		seen[name] = true
	}
	return &named
}

// truncateUTF16 cuts s to at most units UTF-16 code units, the unit the
// browser counts name lengths in.
func truncateUTF16(s string, units int) string {
	used := 0
	for i, r := range s {
		if used += utf16.RuneLen(r); used > units {
			return s[:i]
		}
	}
	return s
}

// validCommandNames reports whether every command has a unique, non-empty
// name usable in a {{name}} reference. It is checked only on a patch of the
// list: files saved before names were required still load.
func validCommandNames(list *[]Command) bool {
	if list == nil {
		return true
	}
	seen := map[string]bool{"user": true, "dir": true, "webmux": true}
	for _, c := range *list {
		if c.Name == "" || seen[c.Name] || strings.ContainsAny(c.Name, "{}\n") {
			return false
		}
		seen[c.Name] = true
	}
	return true
}
