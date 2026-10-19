package session

// Preview uses only screen text. Claiming OSC 8 before the default handler
// prevents its link registry from retaining a history that is never displayed.
func (s *Preview) ignoreLinks() {
	s.screen.InputHandler().RegisterOscHandler(8, func(string) bool { return true })
}
