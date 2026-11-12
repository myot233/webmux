package hybrid

import (
	"github.com/rainchan/webmux/internal/conversation"
	"time"
)

// emit stores events in the log, keeping the replaceable state they carry.
func (a *actor) emit(events ...Event) {
	for _, e := range events {
		if e.Kind == EventResolved && e.Resolved != nil {
			if err := a.writeActivity(conversation.ActivityInput{LocalKey: a.activityKey("permission"), Kind: "permission", Status: e.Resolved.Outcome, Text: e.Resolved.OptionID}); err != nil {
				a.activityFailure()
			}
		}
		if e.Kind == EventMarker && e.Marker != nil {
			if err := a.writeActivity(conversation.ActivityInput{LocalKey: a.activityKey("marker"), Kind: "marker", Status: e.Marker.Kind}); err != nil {
				a.activityFailure()
			}
		}
		toolsOnly := false
		switch e.Kind {
		case EventPlan:
			a.chat.Plan = e.Plan
			a.work.setPlan(e.Plan)
		case EventCommands:
			a.chat.Commands = e.Commands
		case EventConfig:
			a.chat.Config = e.Config
			values := configValues(e.Config)
			a.record(false, func(r *Record) { r.Config = values })
			a.svc.options.remember(a.configID, e.Config, time.Now())
		case EventUsage:
			a.chat.Usage = e.Usage
		case EventInfo:
			if e.Info == nil || e.Info.Title == nil {
				continue
			}
			a.title = *e.Info.Title
			a.promptTitle = false
		case EventMessage:
			if e.Message.Role != RoleAgent || !a.preview.add(e.Message) {
				continue
			}
		case EventTool:
			// Tool calls only matter to the session card's work
			// line; flush only when it changes.
			if a.turn != nil {
				a.work.tool(*e.Tool)
			}
			toolsOnly = true
		default:
			continue
		}
		if !toolsOnly {
			a.dirty = true
		}
	}
	a.refreshWork()
	a.log.append(time.Now().UTC(), events...)
}
