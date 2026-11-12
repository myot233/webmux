package hub

import "context"

// MessageType classifies a framed stream payload.
type MessageType int

const (
	// MessageText is a UTF-8 JSON message frame.
	MessageText MessageType = iota + 1
	// MessageBinary is an arbitrary byte frame.
	MessageBinary
)

// StatusCode is an RFC 6455 or WebMux 44xx application close code.
type StatusCode uint16

const (
	StatusNormalClosure StatusCode = 1000
	StatusInternalError StatusCode = 1011
	StatusTryAgainLater StatusCode = 1013

	StatusNotFound      StatusCode = 4404
	StatusSlowClient    StatusCode = 4408
	StatusSessionExited StatusCode = 4410
)

// MessageConn is the transport-agnostic framed stream contract used by hub
// stream handlers across WebSocket upgrades and alternative stream transports.
type MessageConn interface {
	Read(ctx context.Context) (MessageType, []byte, error)
	Write(ctx context.Context, typ MessageType, p []byte) error
	Close(code StatusCode, reason string) error
	CloseNow() error
	SetReadLimit(n int64)
	CloseRead(ctx context.Context) context.Context
}
