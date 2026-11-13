package acp

// MCPServer is an ACP v1 stdio server descriptor. Stdio support is implicit in
// ACP v1; remote transports require additional capabilities and are not used.
// Env is explicit and empty for WebMux control/history; no token is passed.
type MCPServer struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Env     []MCPEnv `json:"env"`
}

type MCPEnv struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
