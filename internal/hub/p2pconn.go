package hub

import (
	"context"
	"fmt"

	"github.com/rainchan/webmux/internal/p2p"
)

type p2pStreamConn struct {
	c p2p.StreamConn
}

var _ MessageConn = (*p2pStreamConn)(nil)

// NewP2PStreamConn adapts a p2p.StreamConn to MessageConn.
func NewP2PStreamConn(c p2p.StreamConn) MessageConn {
	if c == nil {
		return nil
	}
	return &p2pStreamConn{c: c}
}

func (p *p2pStreamConn) Read(ctx context.Context) (MessageType, []byte, error) {
	typ, data, err := p.c.Read(ctx)
	if err != nil {
		return 0, nil, err
	}
	switch typ {
	case p2p.MessageText:
		return MessageText, data, nil
	case p2p.MessageBinary:
		return MessageBinary, data, nil
	default:
		return 0, nil, fmt.Errorf("unsupported p2p message type %v", typ)
	}
}

func (p *p2pStreamConn) Write(ctx context.Context, typ MessageType, data []byte) error {
	var p2pType p2p.MessageType
	switch typ {
	case MessageText:
		p2pType = p2p.MessageText
	case MessageBinary:
		p2pType = p2p.MessageBinary
	default:
		return fmt.Errorf("unsupported message type %v", typ)
	}
	return p.c.Write(ctx, p2pType, data)
}

func (p *p2pStreamConn) Close(code StatusCode, reason string) error {
	return p.c.Close(uint16(code), reason)
}

func (p *p2pStreamConn) CloseNow() error {
	return p.c.CloseNow()
}

func (p *p2pStreamConn) SetReadLimit(n int64) {
	p.c.SetReadLimit(n)
}

func (p *p2pStreamConn) CloseRead(ctx context.Context) context.Context {
	return p.c.CloseRead(ctx)
}
