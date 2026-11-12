package agentprofile

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Registry is an immutable, ordered set of agents. Order matters only for
// Identify: the first agent that recognises the signals wins.
type Registry struct {
	agents []Agent
}

// NewRegistry lists agents in Identify order. It panics on an agent
// without a name or a duplicate name: the list is fixed in source.
func NewRegistry(agents ...Agent) *Registry {
	seen := map[string]bool{}
	for _, a := range agents {
		name := a.Profile().Name
		if name == "" || seen[name] {
			panic(fmt.Sprintf("agentprofile: invalid or duplicate agent name %q", name))
		}
		seen[name] = true
	}
	return &Registry{agents: agents}
}

// Agents returns the registered agents in order.
func (r *Registry) Agents() []Agent { return append([]Agent(nil), r.agents...) }

// Lookup returns the agent named name.
func (r *Registry) Lookup(name string) (Agent, bool) {
	for _, a := range r.agents {
		if a.Profile().Name == name {
			return a, true
		}
	}
	return nil, false
}

// ForExecutable returns the agent that runs as executable base name base.
func (r *Registry) ForExecutable(base string) (Agent, bool) {
	for _, a := range r.agents {
		if a.Profile().RunsAs(base) {
			return a, true
		}
	}
	return nil, false
}

// SubmitDelay returns the terminal submission separation for argv's agent.
// It depends only on the executable, so ordinary sessions need not be
// hybrid-compatible to use the same input behavior.
func (r *Registry) SubmitDelay(argv []string) time.Duration {
	_, _, exe, _, ok := splitForm(argv)
	if !ok {
		return 0
	}
	a, ok := r.ForExecutable(filepath.Base(exe))
	if !ok {
		return 0
	}
	return SubmitDelay(a)
}

// Identify maps signals to the first agent that recognises them.
func (r *Registry) Identify(s Signals) (Identity, bool) {
	for _, a := range r.agents {
		if id, ok := a.Identify(s); ok {
			return id, true
		}
	}
	return Identity{}, false
}

// Explain is the user-facing explanation of reason, used in API errors;
// an unknown agent is told which agents are supported.
func (r *Registry) Explain(reason Reason) string {
	if reason != ReasonUnknownAgent {
		return reason.Message()
	}
	return reason.Message() + "，当前版本支持：" + strings.Join(r.Titles(), "、")
}

// Titles lists the agents' display names in order.
func (r *Registry) Titles() []string {
	titles := make([]string, len(r.agents))
	for i, a := range r.agents {
		titles[i] = a.Profile().Title
	}
	return titles
}

// Check reports whether argv can run as a hybrid session.
func (r *Registry) Check(argv []string) Support {
	cmd, err := r.Parse(argv)
	if err != nil {
		return Support{Reason: err.(*UnsupportedError).Reason, Agents: r.Titles()}
	}
	return Support{Supported: true, Agent: cmd.Profile.Name, Connector: cmd.Profile.Connector, Agents: r.Titles()}
}

// Parse recognises argv as an agent's command in a supported form and
// classifies its arguments. The returned error is always *UnsupportedError.
func (r *Registry) Parse(argv []string) (Command, error) {
	form, shell, exe, args, ok := splitForm(argv)
	agent, known := r.ForExecutable(filepath.Base(exe))
	if !ok || !known {
		return Command{}, &UnsupportedError{Reason: r.unrecognised(argv)}
	}
	return newCommand(agent, form, shell, exe, args)
}

// unrecognised distinguishes a wrapper around a known agent, which the user
// can rewrite, from a command that runs no supported agent at all.
func (r *Registry) unrecognised(argv []string) Reason {
	for _, token := range argv {
		for _, word := range strings.Fields(token) {
			if _, ok := r.ForExecutable(filepath.Base(word)); ok {
				return ReasonUnsupportedForm
			}
		}
	}
	return ReasonUnknownAgent
}
