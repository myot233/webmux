//go:build !linux && !darwin

package session

import "github.com/creack/pty"

type processTable struct{}
type processSnapshot struct{}

func processes() processTable                         { return processTable{} }
func newProcessSnapshot(processTable) processSnapshot { return processSnapshot{} }
func (processSnapshot) foregroundBlocked(int) bool    { return false }
func (processSnapshot) awaitingInput(int) bool        { return false }
func rootProcess(int) (exe, cwd string)               { return "", "" }

func (s *Session) resizePTYLocked(cols, rows uint16) error {
	if s.run == nil {
		return ErrNoRun
	}
	return pty.Setsize(s.run.terminal, &pty.Winsize{Cols: cols, Rows: rows})
}
