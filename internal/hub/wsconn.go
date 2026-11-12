package hub

import (
	"context"
	"fmt"

	"github.com/coder/websocket"
)

type wsConn struct {
	c *websocket.Conn
}

var _ MessageConn = (*wsConn)(nil)

// NewWebSocketConn adapts a coder/websocket connection to MessageConn.
func NewWebSocketConn(c *websocket.Conn) MessageConn {
	if c == nil {
		return nil
	}
	return &wsConn{c: c}
}

func (w *wsConn) Read(ctx context.Context) (MessageType, []byte, error) {
	typ, p, err := w.c.Read(ctx)
	if err != nil {
		return 0, nil, err
	}
	switch typ {
	case websocket.MessageText:
		return MessageText, p, nil
	case websocket.MessageBinary:
		return MessageBinary, p, nil
	default:
		return 0, nil, fmt.Errorf("unsupported websocket message type %v", typ)
	}
}

func (w *wsConn) Write(ctx context.Context, typ MessageType, p []byte) error {
	var wsType websocket.MessageType
	switch typ {
	case MessageText:
		wsType = websocket.MessageText
	case MessageBinary:
		wsType = websocket.MessageBinary
	default:
		return fmt.Errorf("unsupported message type %v", typ)
	}
	return w.c.Write(ctx, wsType, p)
}

func (w *wsConn) Close(code StatusCode, reason string) error {
	return w.c.Close(websocket.StatusCode(code), reason)
}

func (w *wsConn) CloseNow() error {
	return w.c.CloseNow()
}

func (w *wsConn) SetReadLimit(n int64) {
	w.c.SetReadLimit(n)
}

func (w *wsConn) CloseRead(ctx context.Context) context.Context {
	return w.c.CloseRead(ctx)
}
