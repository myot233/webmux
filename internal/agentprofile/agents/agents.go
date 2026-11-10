// Package agents lists the built-in hybrid agent adapters. Adding an agent
// is a new adapter subpackage of agentprofile and one entry here.
package agents

import (
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/agentprofile/agy"
	"github.com/rainchan/webmux/internal/agentprofile/claudecode"
	"github.com/rainchan/webmux/internal/agentprofile/codex"
	"github.com/rainchan/webmux/internal/agentprofile/kimi"
	"github.com/rainchan/webmux/internal/agentprofile/omp"
	"github.com/rainchan/webmux/internal/agentprofile/opencode"
	"github.com/rainchan/webmux/internal/agentprofile/qoder"
	"github.com/rainchan/webmux/internal/agentprofile/qodercn"
)

// defaultRegistry is in Identify order: omp's Warp events take precedence
// over another agent's OSC 7501 record.
var defaultRegistry = agentprofile.NewRegistry(omp.Agent, opencode.Agent, codex.Agent, claudecode.Agent, qoder.Agent, qodercn.Agent, kimi.Agent, agy.Agent)

// Default returns the immutable registry of built-in agents.
func Default() *agentprofile.Registry { return defaultRegistry }
