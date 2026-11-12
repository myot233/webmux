package wire

import (
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/codeview"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hoststat"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/push"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/tailscale"
	"github.com/rainchan/webmux/internal/voice"
)

func previewSettings(p *launch.PreviewSettings) *contract.PreviewSettings {
	if p == nil {
		return nil
	}
	return &contract.PreviewSettings{Lines: p.Lines, Skip: p.Skip}
}

func launchPreview(p *contract.PreviewSettings) *launch.PreviewSettings {
	if p == nil {
		return nil
	}
	return &launch.PreviewSettings{Lines: p.Lines, Skip: p.Skip}
}

func attentionSettings(a *launch.AttentionSettings) *contract.AttentionSettings {
	if a == nil {
		return nil
	}
	return &contract.AttentionSettings{
		Mode:             string(a.Mode),
		ProgramStatus:    a.ProgramStatus,
		AgentProtocol:    a.AgentProtocol,
		Notify:           a.Notify,
		Bell:             a.Bell,
		MinTurnSeconds:   a.MinTurnSeconds,
		DoneDelaySeconds: a.DoneDelaySeconds,
	}
}

func launchAttention(a *contract.AttentionSettings) *launch.AttentionSettings {
	if a == nil {
		return nil
	}
	return &launch.AttentionSettings{
		Mode:             launch.AttentionMode(a.Mode),
		ProgramStatus:    a.ProgramStatus,
		AgentProtocol:    a.AgentProtocol,
		Notify:           a.Notify,
		Bell:             a.Bell,
		MinTurnSeconds:   a.MinTurnSeconds,
		DoneDelaySeconds: a.DoneDelaySeconds,
	}
}

func LaunchConfig(c launch.Config) contract.LaunchConfig {
	return contract.LaunchConfig{
		ID: c.ID, Name: c.Name, Icon: c.Icon, Cmd: required(c.Cmd), Env: requiredMap(c.Env), Preview: previewSettings(c.Preview),
		Attention: attentionSettings(c.Attention), Chat: c.Hybrid, ChatSupport: ChatSupport(c.HybridSupport()),
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

// LaunchSummaries converts the launch config list; chatOptions returns the
// remembered agent options of a config (nil: none known).
func LaunchSummaries(in []launch.Summary, chatOptions func(configID string) []hybrid.ConfigOption) []contract.LaunchConfigSummary {
	return each(in, func(s launch.Summary) contract.LaunchConfigSummary {
		var options []hybrid.ConfigOption
		if chatOptions != nil {
			options = chatOptions(s.ID)
		}
		return contract.LaunchConfigSummary{
			ID: s.ID, Name: s.Name, Icon: s.Icon, Cmd: required(s.Cmd), EnvNames: required(s.EnvNames), Preview: previewSettings(s.Preview),
			Attention: attentionSettings(s.Attention), Chat: s.Hybrid, ChatSupport: ChatSupport(s.HybridSupport), ChatOptions: each(options, chatConfig),
			CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		}
	})
}

func LaunchInput(r contract.LaunchConfigInput) launch.Input {
	return launch.Input{Name: r.Name, Icon: r.Icon, Cmd: r.Cmd, Env: r.Env, Preview: launchPreview(r.Preview), Attention: launchAttention(r.Attention), Hybrid: r.Chat}
}

func LaunchPresets(in []launch.Preset) []contract.LaunchPreset {
	return each(in, func(p launch.Preset) contract.LaunchPreset {
		return contract.LaunchPreset{
			Name: p.Name, Icon: p.Icon, Cmd: required(p.Cmd), EnvNames: required(p.EnvNames), Chat: p.Hybrid,
		}
	})
}

func ChatSupport(s agentprofile.Support) contract.ChatSupport {
	return contract.ChatSupport{Supported: s.Supported, Agent: s.Agent, Reason: string(s.Reason), Connector: AgentConnector(s.Connector),
		SupportedAgents: required(s.Agents),
	}
}

// AgentConnector converts a profile's connector; nil stays nil. Its
// verified version stays on the server.
func AgentConnector(c *agentprofile.Connector) *contract.AgentConnector {
	if c == nil {
		return nil
	}
	return &contract.AgentConnector{Executable: c.Executable, Title: c.Title, InstallCommand: c.InstallCommand, DocsURL: c.DocsURL}
}

func PushTestReport(r push.TestReport) contract.PushTestReport {
	out := contract.PushTestReport(r)
	out.Removed, out.Errors = required(r.Removed), required(r.Errors)
	return out
}

func CodeContext(c codeview.Context) contract.CodeContext { return contract.CodeContext(c) }

func codeNode(n codeview.Node) contract.CodeNode { return contract.CodeNode(n) }

func CodeListing(l codeview.Listing) contract.CodeListing {
	return contract.CodeListing{Children: each(l.Children, codeNode), Next: l.Next, Limited: l.Limited, Complete: l.Complete, Version: l.Version, Checked: l.Checked}
}

func CodeLocation(l codeview.Location) contract.CodeLocation { return contract.CodeLocation(l) }

func codeDocument(d codeview.Document) contract.CodeDocument {
	return contract.CodeDocument{
		NodeID: d.NodeID, Path: d.Path, Absolute: d.Absolute, Source: d.Source, OID: d.OID,
		Version: d.Version, Kind: d.Kind, Text: d.Text, Lines: d.Lines, Referenceable: d.Referenceable,
		Reason: d.Reason, MediaType: d.MediaType, Size: d.Size, Downloadable: d.Downloadable,
	}
}

func CodeFile(f codeview.File) contract.CodeFile {
	return contract.CodeFile{Snapshot: f.Snapshot, Document: codeDocument(f.Document), Checked: f.Checked}
}

func codeChange(c codeview.Change) contract.CodeChange { return contract.CodeChange(c) }

func CodeChanges(c codeview.Changes) contract.CodeChanges {
	return contract.CodeChanges{Next: c.Next, Total: c.Total, Entries: each(c.Entries, codeChange), Version: c.Version, Checked: c.Checked}
}

func CodeDiff(d codeview.Diff) contract.CodeDiff {
	return contract.CodeDiff{
		Snapshot: d.Snapshot, Old: codeDocument(d.Old), New: codeDocument(d.New),
		Kind: d.Kind, Reason: d.Reason, Checked: d.Checked, Change: codeChange(d.Change),
	}
}

func CodeCheck(c codeview.Check) contract.CodeCheck {
	out := contract.CodeCheck(c)
	out.Snapshots, out.Directories = requiredMap(c.Snapshots), requiredMap(c.Directories)
	return out
}

func CodeCheckRequest(r contract.CodeCheckRequest) codeview.CheckRequest {
	return codeview.CheckRequest(r)
}

func CodeMutation(m codeview.Mutation) contract.CodeMutation {
	return contract.CodeMutation{Nodes: each(m.Nodes, codeNode)}
}

func CodeUsage(u codeview.Usage) contract.CodeUsage { return contract.CodeUsage(u) }

// SessionEvent converts one session list event.
func SessionEvent(e session.Event) contract.SessionEvent {
	out := contract.SessionEvent{Type: e.Type, Sessions: Sessions(e.Sessions), ID: e.ID}
	if e.Session != nil {
		info := Session(*e.Session)
		out.Session = &info
	}
	return out
}

func StreamPosition(p session.Position) contract.StreamPosition {
	return contract.StreamPosition(p)
}

// TailscaleStatus converts the read-only detection result. Runtime state and
// configured mode are layered on by the controller.
func TailscaleStatus(s tailscale.Status) contract.TailscaleStatus {
	out := contract.TailscaleStatus{
		Provider: s.Provider, ConfiguredProvider: s.ConfiguredProvider,
		BackendState: s.BackendState, DNSName: s.DNSName,
		HTTPSReady: s.HTTPSReady, FunnelAvailable: s.FunnelAvailable,
		Entries: each(s.Entries, func(e tailscale.Entry) contract.TailscaleEntry {
			return contract.TailscaleEntry{Host: e.Host, Port: e.Port, Public: e.Public}
		}),
	}
	if s.WSL != nil {
		out.WSL = &contract.TailscaleWSL{
			NetworkingMode: s.WSL.NetworkingMode, WindowsDetected: s.WSL.WindowsDetected,
			WindowsRunning: s.WSL.WindowsRunning, WindowsDNSName: s.WSL.WindowsDNSName,
		}
	}
	return out
}

func SubmissionStatus(s session.SubmissionStatus) contract.SubmissionStatus {
	return contract.SubmissionStatus{ID: s.ID, State: string(s.State), Error: s.Error}
}

func ControlState(c session.ControlState) contract.ControlState {
	out := contract.ControlState{Cols: c.Cols, Rows: c.Rows, ColorSchemeReports: c.ColorSchemeReports}
	if c.Controller != nil {
		controller := contract.TerminalController(*c.Controller)
		out.Controller = &controller
	}
	return out
}

func VoiceEvent(e voice.Event) contract.VoiceServerEvent {
	return contract.VoiceServerEvent{
		Type:  string(e.Type),
		Text:  e.Text,
		Error: e.Error,
	}
}

// Health is the GET /api/health body; nil load or memory is left out.
func Health(sessions int, load *hoststat.Load, mem *hoststat.Memory) contract.APIHealth {
	h := contract.APIHealth{OK: true, Sessions: sessions}
	if load != nil {
		h.Load = &contract.HostLoad{One: load.One, Five: load.Five, Fifteen: load.Fifteen, CPUs: load.CPUs}
	}
	if mem != nil {
		h.Memory = &contract.HostMemory{Total: mem.Total, Available: mem.Available}
	}
	return h
}
