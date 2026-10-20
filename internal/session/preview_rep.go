package session

import "github.com/0magnet/xterm-go/vt"

// This observer may approximate pathological output; the raw PTY bytes still
// reach the browser unchanged. A CSI count must not dictate server allocation.
func (s *Preview) boundRepeats() {
	handler := s.screen.InputHandler()
	handler.RegisterCsiHandler(vt.FunctionID{Final: "b"}, func(params *vt.Params) bool {
		const repeatLimit = 4096
		if params.Length > 0 && params.Params[0] > repeatLimit {
			params = params.Clone()
			params.Params[0] = repeatLimit
		}
		return handler.RepeatPrecedingCharacter(params)
	})
}
