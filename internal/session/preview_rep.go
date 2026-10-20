package session

import "github.com/0magnet/xterm-go/vt"

// This observer may approximate pathological output; the raw PTY bytes still
// reach the browser unchanged. A CSI count must not dictate server allocation.
func (s *Preview) boundRepeats() {
	handler := s.screen.InputHandler()
	bound := func(final string, limit func() int, apply func(*vt.Params) bool) {
		handler.RegisterCsiHandler(vt.FunctionID{Final: final}, func(params *vt.Params) bool {
			maximum := int32(limit())
			if params.Length > 0 && params.Params[0] > maximum {
				params = params.Clone()
				params.Params[0] = maximum
			}
			return apply(params)
		})
	}
	bound("b", func() int { return 4096 }, handler.RepeatPrecedingCharacter)
	// Once the cursor reaches the last tab stop or the entire scroll region
	// is blank, further iterations have no visible effect. Read the active
	// dimensions at dispatch time, including alternate buffers and resizes.
	cols := func() int { return s.screen.Cols() }
	rows := func() int {
		buffer := s.screen.Buffer()
		return buffer.ScrollBottom - buffer.ScrollTop + 1
	}
	bound("I", cols, handler.CursorForwardTab)
	bound("Z", cols, handler.CursorBackwardTab)
	bound("L", rows, handler.InsertLines)
	bound("M", rows, handler.DeleteLines)
	bound("S", rows, handler.ScrollUp)
	bound("T", rows, handler.ScrollDown)
}
