package hub

import (
	"context"
	"encoding/json"

	"github.com/rainchan/webmux/internal/settings"
	"github.com/rainchan/webmux/internal/wire"
)

// Settings has one data writer; CloseRead consumes ping/close control frames.
func Settings(ctx context.Context, sub *settings.Subscription, c MessageConn) {
	defer func() { _ = c.CloseNow() }()
	ctx = c.CloseRead(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-sub.Events:
			if !ok {
				_ = c.Close(StatusTryAgainLater, "settings changed; reconnect")
				return
			}
			data, err := json.Marshal(wire.SettingsEvent(event))
			if err != nil {
				return
			}
			if write(ctx, c, MessageText, data) != nil {
				return
			}
		}
	}
}
