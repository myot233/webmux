package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/rainchan/webmux/internal/agentguide"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/artifacts"
	"github.com/rainchan/webmux/internal/codeview"
	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/files"
	"github.com/rainchan/webmux/internal/forward"
	"github.com/rainchan/webmux/internal/handoff"
	"github.com/rainchan/webmux/internal/hoststat"
	"github.com/rainchan/webmux/internal/hub"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/jsonutil"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/push"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/settings"
	"github.com/rainchan/webmux/internal/voice"
	"github.com/rainchan/webmux/internal/wire"
	"github.com/rainchan/webmux/internal/workspace"
)

// Deps are the services the handlers call. cmd/ constructs every service;
// the controller never builds one itself.
type Deps struct {
	Handoffs      *handoff.Service
	Conversations *conversation.Service
	Settings      *settings.Service
	Config        config.Config
	Manager       *session.Manager
	Workspace     *workspace.Service
	Code          *codeview.Service
	Files         *files.Service
	// Artifacts owns each session's interaction artifacts folder, where
	// uploads are stored; nil answers 503 for uploads and the folder.
	Artifacts *artifacts.Service
	// DefaultShell is the startup-file shell resolved from the service's
	// $SHELL: "bash", "zsh", or "" for a shell WebMux does not wrap.
	DefaultShell string
	// Public authorizes runtime HTTPS entry points; nil means no tunnel.
	Public *PublicOrigins
	// Push is nil when [push] enabled = false.
	Push *push.Service
	// Launch is nil when the launch config store was not opened; its
	// endpoints then answer 503.
	Launch *launch.Service
	// Tunnel reports the runtime tunnel state; nil leaves GET /api/config
	// describing only the configured mode.
	Tunnel         func() contract.TunnelStatus
	TunnelSettings TunnelSettingsControl
	// Tailscale reports and updates the managed Tailscale entry; nil
	// answers 503.
	Tailscale TailscaleControl
	// Hybrid serves the chat channel of hybrid sessions; nil answers 404
	// for every chat endpoint.
	Hybrid *hybrid.Service
	// Forward is the port forwarding registry; nil serves no forwards.
	Forward *forward.Service
	// Voice runs the optional external speech recognition command; nil or
	// disabled answers 404 on GET /ws/voice.
	Voice *voice.Service
	// VoiceSettings reads and replaces the recognizer from the settings
	// page; nil answers 503.
	VoiceSettings VoiceControl
	// P2P manages opportunistic WebRTC P2P direct upgrades and its settings;
	// nil answers 503 on /api/p2p* (except /api/p2p/ping).
	P2P P2PControl
	// InstalledVersion returns the installed version awaiting a restart,
	// or ""; nil never reports one.
	InstalledVersion func() string
	// Guide renders the shell guide and the {{webmux}} brief; nil answers
	// 503 for the brief and serves the guide without instance facts.
	Guide *agentguide.Instance
	// LocalURL is the token-free local origin, such as
	// http://127.0.0.1:7681; empty leaves session details without a link.
	LocalURL string
	// Launcher is the recorded launcher. It selects the restart method of a
	// pending update; on the local API, systemd and launchd refuse an
	// unforced shutdown.
	Launcher string
}

type Server struct {
	Deps
	// local marks the Unix-socket handler, which serves no browser page.
	local bool
	// gatewayProxy serves /forward/{port}/; nil without Deps.Forward.
	gatewayProxy *httputil.ReverseProxy
}

// New builds the TCP handler. Local-only endpoints are not found here.
func New(d Deps) http.Handler {
	return NewBridge(d).HTTPHandler
}

func newServer(d Deps) *Server {
	if d.Manager == nil || d.Workspace == nil || d.Code == nil || d.Files == nil {
		panic("api: Deps requires Manager, Workspace, Code and Files")
	}
	s := &Server{Deps: d}
	if d.Forward != nil {
		s.gatewayProxy = newGatewayProxy(d.Forward)
	}
	return s
}

// routes registers the /api, /ws and static routes shared by both listeners.
func (s *Server) routes() *http.ServeMux {
	c, m, workspaces := s.Config, s.Manager, s.Workspace
	mux := http.NewServeMux()
	s.codeRoutes(mux)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		load, mem := hoststat.Read()
		reply(w, 200, wire.Health(len(m.List()), load, mem))
	})
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		hostname, _ := os.Hostname()
		voiceEnabled := s.Voice != nil && s.Voice.Enabled()
		p2pEnabled := s.P2P != nil && s.P2P.Enabled()
		reply(w, 200, contract.ServerConfig{DefaultCwd: c.DefaultCwd, DefaultShell: s.DefaultShell, Hostname: hostname, Workspace: wire.Capability(workspaces.Capability()), Tunnel: s.tunnelInfo(), PendingUpdate: s.pendingUpdate(), PortDetection: session.ListeningPortsSupported, Voice: voiceEnabled, P2P: p2pEnabled})
	})
	s.sessionRoutes(mux)
	serveIn(mux, "PATCH /api/sessions/{id}", http.StatusOK, renameFailure, s.rename)
	mux.HandleFunc("GET /api/sessions/{id}/buffer", s.buffer)
	mux.HandleFunc("GET /api/sessions/{id}/output", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		page, err := s.output(r)
		if err != nil {
			sessionCommandFailure(w, err)
			return
		}
		reply(w, http.StatusOK, page)
	})
	mux.HandleFunc("GET /api/sessions/{id}/upload", s.readUpload)
	mux.HandleFunc("POST /api/sessions/{id}/upload", s.upload)
	mux.HandleFunc("POST /api/sessions/{id}/artifacts", s.ensureArtifacts)
	mux.HandleFunc("GET /api/fs/dirs", s.dirs)
	mux.HandleFunc("POST /api/fs/mkdir", s.mkdir)
	s.sessionMetaRoutes(mux)
	s.guideRoutes(mux)
	s.chatRoutes(mux)
	s.conversationRoutes(mux)
	s.handoffRoutes(mux)
	s.workspaceRoutes(mux)
	s.pushRoutes(mux)
	s.launchRoutes(mux)
	s.settingsRoutes(mux)
	s.tunnelRoutes(mux)
	s.forwardRoutes(mux)
	s.tailscaleRoutes(mux)
	s.voiceRoutes(mux)
	s.p2pRoutes(mux)
	mux.HandleFunc("GET /ws/sessions", s.listWS)
	mux.HandleFunc("GET /ws/s/{id}", s.terminalWS)
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	mux.HandleFunc("GET /ws/", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	mux.HandleFunc("GET /", s.static)
	return mux
}

// reply, failure, decode and decodeOptional are the controller's JSON
// boundary; archtest rule R8 requires their values to be contract types.
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, status int, err error) {
	message := err.Error()
	if status == http.StatusInternalServerError {
		slog.Error("request failed", "status", status, "error", err)
		message = http.StatusText(status)
	}
	reply(w, status, contract.ErrorResponse{Error: message})
}

// chatBodyBytes admits a prompt of hybrid.MaxPromptBytes even when every
// character is JSON-escaped.
const chatBodyBytes = 8 * hybrid.MaxPromptBytes

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	limit := int64(65536)
	switch v.(type) {
	case *contract.SessionInputRequest, *contract.ChatQueueRequest:
		limit = chatBodyBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		if _, sensitive := v.(*contract.TunnelSettingsUpdate); sensitive {
			err = errors.New("invalid tunnel settings JSON: use only known fields with the correct types")
		}
		failure(w, 400, err)
		return false
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		failure(w, 400, errors.New("expected one JSON object"))
		return false
	}
	return true
}

// decodeStrict is decode for bodies that must also reject duplicate and
// case-folded keys. A body over limit bytes or an invalid document is a 400
// that carries only invalid, never any input text.
func decodeStrict(w http.ResponseWriter, r *http.Request, v any, limit int64, invalid error) bool {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil || jsonutil.Decode(data, v) != nil {
		failure(w, http.StatusBadRequest, invalid)
		return false
	}
	return true
}

// baseHeaders sets the response headers and panic recovery both listeners
// share; it returns the deferred recovery.
func baseHeaders(w http.ResponseWriter) func() {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	return func() {
		if p := recover(); p != nil {
			if p == http.ErrAbortHandler {
				// Deliberate connection abort (e.g. raw bytes changed mid-stream).
				panic(p)
			}
			slog.Error("request panic", "panic", p)
			http.Error(w, "internal error", 500)
		}
	}
}
func (s *Server) middleware(next http.Handler) http.Handler {
	hostAllowed := hostCheck(s.Config)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer baseHeaders(w)()
		if !hostAllowed(r.Host) && !s.Public.hostAllowed(r.Host) {
			failure(w, http.StatusForbidden, errors.New("host not allowed"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
			if !s.authorized(r) {
				failure(w, 401, errors.New("authentication required"))
				return
			}
			if !s.originAllowed(r) {
				failure(w, 403, errors.New("origin not allowed"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) authorized(r *http.Request) bool {
	if s.Config.Token == "" {
		return true
	}
	token := ""
	scheme, credential, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if found && strings.EqualFold(scheme, "Bearer") {
		token = credential
	}
	if token == "" && strings.HasPrefix(r.URL.Path, "/ws/") {
		token = protocolToken(r)
	}
	want := sha256.Sum256([]byte(s.Config.Token))
	got := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(want[:], got[:]) == 1
}

// wsProtocol is the subprotocol the server selects. Browsers cannot set
// Authorization on a WebSocket handshake, so the frontend also offers
// wsTokenProtocol+base64url(token). That keeps the token out of request URLs
// and out of cookies, which browsers share across every port of a host; the
// server never selects or echoes the token protocol.
const (
	wsProtocol      = "webmux"
	wsTokenProtocol = "webmux.token."
)

func protocolToken(r *http.Request) string {
	for _, header := range r.Header.Values("Sec-WebSocket-Protocol") {
		for offered := range strings.SplitSeq(header, ",") {
			encoded, ok := strings.CutPrefix(strings.TrimSpace(offered), wsTokenProtocol)
			if !ok {
				continue
			}
			if token, err := base64.RawURLEncoding.DecodeString(encoded); err == nil {
				return string(token)
			}
		}
	}
	return ""
}
func (s *Server) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.Host == "" || u.Path != "" || strings.ContainsAny(origin, "?#") || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	// Tunnel traffic arrives over plain HTTP, so the scheme inferred from
	// r.TLS cannot vouch for the public hostname; only its https origin can.
	if s.Public.hostAllowed(r.Host) {
		if s.Public.originAllowed(origin, r.Host) {
			return true
		}
	} else if strings.EqualFold(u.Host, r.Host) && u.Scheme == scheme {
		return true
	}
	for _, allowed := range s.Config.AllowedOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}
func (s *Server) find(w http.ResponseWriter, r *http.Request) *session.Session {
	item, err := s.Manager.Get(r.PathValue("id"))
	if err != nil {
		failure(w, 404, err)
		return nil
	}
	return item
}

// rename sets the user-chosen name of a session; a hybrid session also keeps
// it in its record, so a recovered session is still named. A broken record
// store fails only hybrid sessions; an ordinary shell's rename never depends
// on it.
func (s *Server) rename(r *http.Request, req contract.RenameSessionRequest) (contract.SessionInfo, error) {
	id := r.PathValue("id")
	info, err := s.Manager.Rename(id, req.Name)
	if err != nil {
		return contract.SessionInfo{}, err
	}
	if s.Hybrid != nil {
		err := s.Hybrid.SetName(id, strings.TrimSpace(req.Name))
		if errors.Is(err, hybrid.ErrNotFound) || (info.Hybrid == nil && errors.Is(err, hybrid.ErrUnavailable)) {
			err = nil
		}
		if err != nil {
			return contract.SessionInfo{}, err
		}
	}
	return wire.Session(info), nil
}

func (s *Server) buffer(w http.ResponseWriter, r *http.Request) {
	if item := s.find(w, r); item != nil {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(item.Buffer())
	}
}
func (s *Server) dirs(w http.ResponseWriter, r *http.Request) {
	out, err := s.Files.Complete(r.URL.Query().Get("prefix"))
	if err != nil {
		filesFailure(w, err)
		return
	}
	reply(w, 200, contract.DirectoryList(out))
}
func (s *Server) mkdir(w http.ResponseWriter, r *http.Request) {
	var req contract.MkdirRequest
	if !decode(w, r, &req) {
		return
	}
	if err := s.Files.Mkdir(req.Path); err != nil {
		filesFailure(w, err)
		return
	}
	w.WriteHeader(204)
}

// accept upgrades a WebSocket and adapts it to hub.MessageConn. Browsers
// always offer permessage-deflate and scripts cannot withdraw the offer, so
// the client opts in with compress=1; otherwise the extension is not negotiated.
func (s *Server) accept(w http.ResponseWriter, r *http.Request) hub.MessageConn {
	opts := &websocket.AcceptOptions{InsecureSkipVerify: true, Subprotocols: []string{wsProtocol}}
	if r.URL.Query().Get("compress") == "1" {
		opts.CompressionMode = websocket.CompressionContextTakeover
	}
	c, err := websocket.Accept(w, r, opts)
	if err != nil {
		return nil
	}
	return hub.NewWebSocketConn(c)
}
func (s *Server) listWS(w http.ResponseWriter, r *http.Request) {
	if c := s.accept(w, r); c != nil {
		hub.List(r.Context(), s.Manager, s.Forward, c)
	}
}
func (s *Server) terminalWS(w http.ResponseWriter, r *http.Request) {
	item, err := s.Manager.Get(r.PathValue("id"))
	c := s.accept(w, r)
	if c == nil {
		return
	}
	if err != nil {
		_ = c.Close(hub.StatusNotFound, "session not found")
		return
	}
	// permessage-deflate already compresses a compress=1 connection; zreplay=1
	// asks for application-level DEFLATE of replay frames where it is off.
	q := r.URL.Query()
	hub.Attach(r.Context(), item, c, hub.AttachOptions(q), q.Get("zreplay") == "1" && q.Get("compress") != "1")
}
