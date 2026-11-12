package contract

// APIHealth is the body of GET /api/health. (Health is the CLI probe.)
type APIHealth struct {
	OK       bool `json:"ok"`
	Sessions int  `json:"sessions"`
	// Load and Memory are omitted where the platform cannot report them.
	Load   *HostLoad   `json:"load,omitempty"`
	Memory *HostMemory `json:"memory,omitempty"`
}

// HostLoad is the host's 1, 5 and 15 minute load averages and its logical
// CPU count.
type HostLoad struct {
	One     float64 `json:"one"`
	Five    float64 `json:"five"`
	Fifteen float64 `json:"fifteen"`
	CPUs    int     `json:"cpus"`
}

// HostMemory is physical memory in bytes; Available excludes what cannot be
// reclaimed without swapping.
type HostMemory struct {
	Total     uint64 `json:"total"`
	Available uint64 `json:"available"`
}

// ServerConfig is the body of GET /api/config.
type ServerConfig struct {
	DefaultCwd string `json:"defaultCwd"`
	// DefaultShell is the startup-file shell of the service's $SHELL:
	// "bash", "zsh", or "" for a shell WebMux does not wrap.
	DefaultShell string              `json:"defaultShell"`
	Hostname     string              `json:"hostname"`
	Workspace    WorkspaceCapability `json:"workspace"`
	Tunnel       TunnelInfo          `json:"tunnel"`
	// PendingUpdate is present when a different version is installed and
	// takes effect after a restart; older servers omit it.
	PendingUpdate *PendingUpdate `json:"pendingUpdate,omitempty"`
	// PortDetection reports whether sessions report their listening ports on
	// this platform.
	PortDetection bool `json:"portDetection"`
	// Voice reports whether external speech recognition is configured.
	Voice bool `json:"voice"`
	// P2P reports whether opportunistic P2P direct upgrade is supported and
	// enabled on this server; older servers omit it.
	P2P bool `json:"p2p"`
}

// PendingUpdate is an installed version awaiting a restart. Launcher is the
// recorded launcher (Launcher* constants), which selects the restart method.
type PendingUpdate struct {
	InstalledVersion string `json:"installedVersion"`
	Launcher         string `json:"launcher"`
}

// TunnelInfo is the read-only tunnel state in GET /api/config. It never
// carries the Cloudflare credential.
type TunnelInfo struct {
	// Enabled reports whether the runtime tunnel is starting or serving. A
	// failed tunnel stays disabled so the page does not present it as usable.
	Enabled   bool   `json:"enabled"`
	State     string `json:"state"`
	Named     bool   `json:"named"`
	Hostname  string `json:"hostname"`
	PublicURL string `json:"publicUrl,omitempty"`
}

// DirectoryList is the body of GET /api/fs/dirs.
type DirectoryList []string

// MkdirRequest is the body of POST /api/fs/mkdir.
type MkdirRequest struct {
	Path string `json:"path"`
}

// UploadResult is the body of a successful POST /api/sessions/{id}/upload.
type UploadResult struct {
	Path string `json:"path"`
}

// ArtifactsFolder is the body of a successful POST
// /api/sessions/{id}/artifacts: the session's interaction artifacts folder.
type ArtifactsFolder struct {
	Path string `json:"path"`
}

// Ack is a bare acknowledgement.
type Ack struct {
	OK bool `json:"ok"`
}
