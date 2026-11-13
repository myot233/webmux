package contract

// AgentGuideTopic describes an available control guide topic.
type AgentGuideTopic struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

// AgentGuideResult is the complete transport-aware guide response.
type AgentGuideResult struct {
	Kind          string            `json:"kind"`
	Transport     string            `json:"transport"`
	Locality      string            `json:"locality"`
	SelfSessionID *string           `json:"selfSessionId"`
	Tools         []string          `json:"tools"`
	Topics        []AgentGuideTopic `json:"topics"`
	Topic         *string           `json:"topic"`
	Markdown      string            `json:"markdown"`
}
