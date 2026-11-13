package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hub"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/p2p"
	"github.com/rainchan/webmux/internal/p2psettings"
)

// P2PControl is injected by cmd/. HTTP handlers never construct the P2P
// service or touch its settings file directly.
type P2PControl interface {
	Enabled() bool
	Status() contract.P2PStatus
	UpdateSettings(contract.P2PSettingsUpdate) (contract.P2PStatus, error)
	ResetSettings() (contract.P2PStatus, error)
	Signal(context.Context, contract.P2PSignalRequest) (contract.P2PSignalResponse, error)
}

// Bridge exposes HTTPHandler along with direct DataChannel stream and RPC
// entry points backed by the exact same server instance and route mux.
type Bridge struct {
	HTTPHandler http.Handler
	ServeStream p2p.StreamHandler
	ServeRPC    p2p.RPCHandler
}

// NewBridge builds the TCP HTTP handler together with DataChannel stream and
// RPC entry points sharing one Server and route mux.
func NewBridge(d Deps) Bridge {
	s := newServer(d)
	mux := s.routes()
	handler := s.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/local/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/mcp" && s.Config.MCP.Enabled && s.MCP != nil {
			s.MCP.ServeHTTP(w, r.WithContext(contract.WithMCPOwner(r.Context())))
			return
		}
		if r.URL.Path == "/mcp" || strings.HasPrefix(r.URL.Path, "/mcp/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/forward" || strings.HasPrefix(r.URL.Path, "/forward/") {
			s.gateway(w, r)
			return
		}
		if r.URL.Path == "/forward-auth" && r.Method == http.MethodGet && forwardAuthPage(w, r) {
			return
		}
		mux.ServeHTTP(w, r)
	}))
	return Bridge{
		HTTPHandler: handler,
		ServeStream: s.serveP2PStream,
		ServeRPC: func(ctx context.Context, req p2p.RPCRequest) p2p.RPCResponse {
			return s.serveP2PRPC(ctx, mux, req)
		},
	}
}

func (s *Server) p2pRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/p2p", func(w http.ResponseWriter, _ *http.Request) {
		if s.p2pControl(w) == nil {
			return
		}
		reply(w, http.StatusOK, s.P2P.Status())
	})
	mux.HandleFunc("PUT /api/p2p/settings", func(w http.ResponseWriter, r *http.Request) {
		if s.p2pControl(w) == nil {
			return
		}
		var req contract.P2PSettingsUpdate
		if !decodeStrict(w, r, &req, 65536, p2psettings.ErrInvalid) {
			return
		}
		view, err := s.P2P.UpdateSettings(req)
		if err != nil {
			failure(w, p2pSettingsStatus(err), err)
			return
		}
		reply(w, http.StatusOK, view)
	})
	mux.HandleFunc("DELETE /api/p2p/settings", func(w http.ResponseWriter, _ *http.Request) {
		if s.p2pControl(w) == nil {
			return
		}
		view, err := s.P2P.ResetSettings()
		if err != nil {
			failure(w, p2pSettingsStatus(err), err)
			return
		}
		reply(w, http.StatusOK, view)
	})
	mux.HandleFunc("POST /api/p2p/signal", func(w http.ResponseWriter, r *http.Request) {
		if s.p2pControl(w) == nil {
			return
		}
		var req contract.P2PSignalRequest
		if !decode(w, r, &req) {
			return
		}
		resp, err := s.P2P.Signal(r.Context(), req)
		if err != nil {
			failure(w, p2pSignalStatus(err), err)
			return
		}
		reply(w, http.StatusOK, resp)
	})
	mux.HandleFunc("GET /api/p2p/ping", func(w http.ResponseWriter, r *http.Request) {
		seq, _ := strconv.Atoi(r.URL.Query().Get("seq"))
		reply(w, http.StatusOK, contract.P2PPingResponse{
			Seq:      seq,
			ServerTs: time.Now().UnixMilli(),
		})
	})
}

func (s *Server) p2pControl(w http.ResponseWriter) P2PControl {
	if s.P2P == nil {
		failure(w, http.StatusServiceUnavailable, errors.New("p2p service unavailable"))
	}
	return s.P2P
}

func (s *Server) serveP2PStream(ctx context.Context, rawPath string, rawConn p2p.StreamConn) {
	conn := hub.NewP2PStreamConn(rawConn)
	if conn == nil {
		return
	}
	parsed, err := url.ParseRequestURI(rawPath)
	if err != nil || !strings.HasPrefix(parsed.Path, "/ws/") {
		_ = conn.Close(hub.StatusNotFound, "invalid stream path")
		return
	}
	q := parsed.Query()
	switch {
	case parsed.Path == "/ws/sessions":
		hub.List(ctx, s.Manager, s.Forward, conn)

	case parsed.Path == "/ws/settings":
		if _, err := s.Settings.PruneSessionPins(s.keepSessionPin); err != nil {
			_ = conn.Close(hub.StatusInternalError, "settings unavailable")
			return
		}
		sub, err := s.Settings.Subscribe()
		if err != nil {
			_ = conn.Close(hub.StatusInternalError, "settings unavailable")
			return
		}
		defer sub.Close()
		hub.Settings(ctx, sub, conn)

	case parsed.Path == "/ws/voice":
		if s.Voice == nil || !s.Voice.Enabled() {
			_ = conn.Close(hub.StatusNotFound, "voice disabled")
			return
		}
		sess, err := s.Voice.Start(ctx)
		if err != nil {
			_ = conn.Close(hub.StatusInternalError, "voice failed")
			return
		}
		hub.Voice(ctx, sess, s.Voice.FinishTimeout(), conn)

	case strings.HasPrefix(parsed.Path, "/ws/s/"):
		rest := strings.TrimPrefix(parsed.Path, "/ws/s/")
		if id, ok := strings.CutSuffix(rest, "/chat"); ok && id != "" && !strings.Contains(id, "/") {
			if err := s.chatTarget(id); errors.Is(err, hybrid.ErrNoChatView) {
				_ = conn.Close(hub.StatusNotFound, "no chat view")
				return
			}
			var sub *hybrid.Subscription
			err := hybrid.ErrNotFound
			if s.Hybrid != nil {
				sub, err = s.Hybrid.Subscribe(id, hub.ChatResume(q))
			}
			if err != nil {
				_ = conn.Close(hub.StatusNotFound, "session not found")
				return
			}
			hub.Chat(ctx, sub, conn)
			return
		}
		if rest != "" && !strings.Contains(rest, "/") {
			item, err := s.Manager.Get(rest)
			if err != nil {
				_ = conn.Close(hub.StatusNotFound, "session not found")
				return
			}
			hub.Attach(ctx, item, conn, hub.AttachOptions(q), q.Get("zreplay") == "1" && q.Get("compress") != "1")
			return
		}
		_ = conn.Close(hub.StatusNotFound, "stream not found")

	default:
		_ = conn.Close(hub.StatusNotFound, "stream not found")
	}
}

type rpcResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	code   int
}

func (w *rpcResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *rpcResponseWriter) WriteHeader(statusCode int) {
	if w.code == 0 {
		w.code = statusCode
	}
}

func (w *rpcResponseWriter) Write(p []byte) (int, error) {
	if w.code == 0 {
		w.code = http.StatusOK
	}
	return w.body.Write(p)
}

func (s *Server) serveP2PRPC(ctx context.Context, mux *http.ServeMux, in p2p.RPCRequest) p2p.RPCResponse {
	rec := &rpcResponseWriter{header: make(http.Header)}
	finish := func() p2p.RPCResponse {
		code := rec.code
		if code == 0 {
			code = http.StatusOK
		}
		headers := make(map[string]string, len(rec.header))
		for k, vals := range rec.header {
			if len(vals) > 0 {
				headers[strings.ToLower(k)] = vals[0]
			}
		}
		return p2p.RPCResponse{
			Status:     code,
			StatusText: http.StatusText(code),
			Headers:    headers,
			Body:       rec.body.Bytes(),
		}
	}

	defer baseHeaders(rec)()

	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		failure(rec, http.StatusBadRequest, errors.New("missing method"))
		return finish()
	}
	parsed, err := url.ParseRequestURI(in.Path)
	if err != nil || !strings.HasPrefix(parsed.Path, "/api/") {
		http.NotFound(rec, &http.Request{Method: method, URL: &url.URL{Path: "/api/"}})
		return finish()
	}

	req, err := http.NewRequestWithContext(ctx, method, "http://127.0.0.1"+in.Path, bytes.NewReader(in.Body))
	if err != nil {
		failure(rec, http.StatusBadRequest, errors.New("invalid request"))
		return finish()
	}
	req.ContentLength = int64(len(in.Body))
	for k, v := range in.Headers {
		if strings.EqualFold(k, "Origin") || strings.EqualFold(k, "Host") {
			continue
		}
		req.Header.Set(k, v)
	}
	if !s.authorized(req) {
		failure(rec, http.StatusUnauthorized, errors.New("authentication required"))
		return finish()
	}
	mux.ServeHTTP(rec, req)
	return finish()
}
