package hybrid

// Replacing an ACP connection must preserve unreported process-tree work as
// well as the provider's tool and background-work signals.
func (a *actor) admitHistoryWriter() error {
	if !a.canSendNow() || a.chat.Turn.Unfinished || a.work.running() || len(a.requests) > 0 {
		return ErrBusy
	}
	if a.link != nil && a.bgBase != nil && hasNewProcess(a.link.sidecar.PID(), a.bgBase) {
		return ErrBusy
	}
	if a.externallyHeld(a.agentSessionID, true) {
		return ErrExternalWriter
	}
	return nil
}
