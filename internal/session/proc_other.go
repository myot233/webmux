//go:build !linux && !darwin

package session

type processTable struct{}
type processSnapshot struct{}

func processes() processTable                         { return processTable{} }
func newProcessSnapshot(processTable) processSnapshot { return processSnapshot{} }
func (processSnapshot) foregroundBlocked(int) bool    { return false }
