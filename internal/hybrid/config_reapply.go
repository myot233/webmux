package hybrid

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/agentprofile"
)

// wanted is an option value to set after an open: recorded, or derived
// from launch argument arg.
type wanted struct {
	id, value, arg string
}

// reapplyValues reads the recorded values of the options the profile says
// a load does not keep. It is read before the new connection reports any
// option, since reported options replace the recorded values. A session
// that never had options recorded (its first open) gets the values the
// profile derives from the launch arguments instead.
func (a *actor) reapplyValues() []wanted {
	rec, ok := a.svc.records.get(a.id)
	if !ok {
		return nil
	}
	if len(rec.Config) == 0 {
		return a.seedValues()
	}
	var want []wanted
	for _, id := range agentprofile.ReapplyOnLoad(a.cmd.Agent) {
		if v, ok := rec.Config[id]; ok && v != "" {
			want = append(want, wanted{id: id, value: v})
		}
	}
	return want
}

// seedValues are the option values derived from the launch arguments.
func (a *actor) seedValues() []wanted {
	seeds, _ := a.cmd.ChatConfig()
	want := make([]wanted, 0, len(seeds))
	for _, s := range seeds {
		want = append(want, wanted{id: s.ConfigID, value: s.Value, arg: s.Arg})
	}
	if mode := a.cmd.Profile.DefaultChatMode; mode != "" && !slices.ContainsFunc(want, func(w wanted) bool { return w.id == "mode" }) {
		want = append(want, wanted{id: "mode", value: mode, arg: "默认聊天权限"})
	}
	return want
}

// reapply sets each wanted option the agent answered with another value
// (or not at all) and returns the resulting options. Refusals become
// notes, not failures; a derived value the agent does not offer is not
// set and noted as not taking effect.
func (a *actor) reapply(link *acpLink, want []wanted, options []acp.ConfigOption) ([]acp.ConfigOption, []string) {
	var notes, ignored []string
	for _, w := range want {
		i := slices.IndexFunc(options, func(o acp.ConfigOption) bool { return o.ID == w.id })
		// Resolve version-specific mode aliases against advertised choices.
		if i >= 0 && w.id == "mode" && !offers(options[i], w.value) {
			for _, aliases := range a.cmd.Profile.ChatModeAliases {
				if slices.Contains(aliases, w.value) {
					for _, alias := range aliases {
						if offers(options[i], alias) {
							w.value = alias
							break
						}
					}
				}
			}
		}
		if w.arg != "" && (i < 0 || !offers(options[i], w.value)) {
			ignored = append(ignored, w.arg)
			continue
		}
		var v acp.ConfigValue
		switch {
		case i >= 0 && options[i].Type == acp.ConfigBoolean:
			b, err := strconv.ParseBool(w.value)
			if err != nil || (options[i].Current.Bool != nil && *options[i].Current.Bool == b) {
				continue
			}
			v.Bool = &b
		case i >= 0 && options[i].Current.ID == w.value:
			continue
		default:
			v.ID = w.value
		}
		ctx, cancel := context.WithTimeout(context.Background(), acp.RequestTimeout)
		resp, err := link.conn.SetConfigOption(ctx, acp.SetConfigOptionRequest{SessionID: link.sessionID, ConfigID: w.id, Value: v})
		cancel()
		if err != nil {
			if agentprofile.ConfigErrorHandled(a.cmd.Agent, err, w.id, w.value) {
				options[i].Current = v
				continue
			}
			a.svc.log.Warn("hybrid config reapply failed", "id", a.id, "option", w.id, "error", err)
			notes = append(notes, "未能恢复配置项 "+w.id)
			continue
		}
		switch {
		case resp.ConfigOptions != nil:
			options = resp.ConfigOptions
		case i >= 0:
			options = slices.Clone(options)
			options[i].Current = v
		}
	}
	if len(ignored) > 0 {
		notes = append(notes, "以下参数在聊天端不生效："+strings.Join(ignored, " "))
	}
	return options, notes
}

// offers reports whether select option o has choice value.
func offers(o acp.ConfigOption, value string) bool {
	return o.Type != acp.ConfigBoolean && slices.ContainsFunc(o.Options, func(c acp.ConfigSelectOption) bool { return c.Value == value })
}
