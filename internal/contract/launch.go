package contract

import "time"

// PreviewSettings selects the session output preview rows. A null config
// preview inherits the browser's global preview setting.
type PreviewSettings struct {
	Lines int `json:"lines"`
	Skip  int `json:"skip"`
}

// HybridSupport reports whether a command can run as a hybrid session.
// Reason is a stable code (unknown_agent, unsupported_form, session_store,
// directory_arg, non_interactive) set when Supported is false.
type HybridSupport struct {
	Supported bool   `json:"supported"`
	Agent     string `json:"agent,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// HybridSupportRequest is the body of POST /api/launch-configs/hybrid-support.
type HybridSupportRequest struct {
	Cmd []string `json:"cmd"`
}

// LaunchConfig is one saved launch config with its environment values.
type LaunchConfig struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Icon          string            `json:"icon,omitempty"`
	Cmd           []string          `json:"cmd"`
	Env           map[string]string `json:"env"`
	Preview       *PreviewSettings  `json:"preview"`
	Hybrid        bool              `json:"hybrid"`
	HybridSupport HybridSupport     `json:"hybridSupport"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

// LaunchConfigSummary is a list entry; it names environment variables only.
type LaunchConfigSummary struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Icon          string           `json:"icon,omitempty"`
	Cmd           []string         `json:"cmd"`
	EnvNames      []string         `json:"envNames"`
	Preview       *PreviewSettings `json:"preview"`
	Hybrid        bool             `json:"hybrid"`
	HybridSupport HybridSupport    `json:"hybridSupport"`
	CreatedAt     time.Time        `json:"createdAt"`
	UpdatedAt     time.Time        `json:"updatedAt"`
}

// LaunchConfigInput is the body of a launch config create or update. The
// read-only hybridSupport is not part of it, so sending it is rejected as an
// unknown field.
type LaunchConfigInput struct {
	Name    string            `json:"name"`
	Icon    string            `json:"icon,omitempty"`
	Cmd     []string          `json:"cmd"`
	Env     map[string]string `json:"env"`
	Preview *PreviewSettings  `json:"preview"`
	Hybrid  bool              `json:"hybrid"`
}
