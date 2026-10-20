package session

import (
	"unicode/utf8"

	"github.com/0magnet/xterm-go/vt"
)

// Limit the observer's grapheme storage before the VT library appends to a cell.
// Repeated string concatenation otherwise makes a long combining sequence cost
// quadratic allocations. Raw PTY output is unaffected by this approximation.
func (s *Preview) boundCombining() {
	const maxCellRunes = 64
	handler := s.screen.InputHandler()
	parser := handler.Parser()
	parser.SetPrintHandler(func(data []uint32, start, end int) {
		pending := start
		preceding := uint32(parser.PrecedingJoinState)
		for pos := start; pos < end; pos++ {
			properties := vt.CharProperties(data[pos], preceding)
			if !vt.ExtractShouldJoin(properties) {
				preceding = properties
				continue
			}
			// Ordinary text retains the library's batched print path. Flush it
			// before inspecting the cell that this combining mark would join.
			if pending < pos {
				handler.Print(data, pending, pos)
			}
			buffer := s.screen.Buffer()
			keep := true
			if buffer.X > 0 {
				line := buffer.Lines.Get(buffer.YBase + buffer.Y)
				x := buffer.X - 1
				if line.GetWidth(x) == 0 {
					x-- // Skip the continuation cell of a wide character.
				}
				if x >= 0 && line.IsCombined(x) {
					keep = utf8.RuneCountInString(line.GetString(x)) < maxCellRunes
				}
			}
			if keep {
				handler.Print(data, pos, pos+1)
			}
			pending = pos + 1
			preceding = uint32(parser.PrecedingJoinState)
		}
		if pending < end {
			handler.Print(data, pending, end)
		}
	})
}
