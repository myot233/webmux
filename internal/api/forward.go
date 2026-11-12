package api

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/forward"
	"github.com/rainchan/webmux/internal/wire"
)

const (
	forwardCookie = "webmux_forward"
	forwardRoot   = "/forward/"
	forwardAuth   = "/forward/_auth"
)

func (s *Server) forwardRoutes(mux *http.ServeMux) {
	if s.Forward == nil {
		return
	}
	f := s.Forward
	mux.HandleFunc("GET /api/forwards", func(w http.ResponseWriter, r *http.Request) {
		reply(w, http.StatusOK, wire.Forwards(f.List()))
	})
	serveIn(mux, "POST /api/forwards", http.StatusCreated, forwardFailure, func(r *http.Request, req contract.ForwardRequest) (contract.Forward, error) {
		out, err := f.Add(r.Context(), wire.ForwardInput(req))
		return wire.Forward(out), err
	})
	serveIn(mux, "PATCH /api/forwards/{port}", http.StatusOK, forwardFailure, func(r *http.Request, req contract.ForwardUpdate) (contract.Forward, error) {
		port, ok := forwardPort(r.PathValue("port"))
		if !ok {
			return contract.Forward{}, fmt.Errorf("%w: %s", forward.ErrNotFound, r.PathValue("port"))
		}
		out, err := f.Update(port, wire.ForwardUpdate(req))
		return wire.Forward(out), err
	})
	serveDone(mux, "DELETE /api/forwards/{port}", forwardFailure, func(r *http.Request) error {
		port, ok := forwardPort(r.PathValue("port"))
		if !ok {
			return fmt.Errorf("%w: %s", forward.ErrNotFound, r.PathValue("port"))
		}
		return f.Remove(port)
	})
	mux.HandleFunc("POST /api/forwards/tickets", func(w http.ResponseWriter, r *http.Request) {
		ticket, err := f.Credentials().IssueTicket()
		if err != nil {
			failure(w, http.StatusInternalServerError, err)
			return
		}
		reply(w, http.StatusCreated, wire.ForwardTicket(ticket))
	})
}

// forwardPort parses a decimal port without sign or leading zeros.
func forwardPort(raw string) (int, bool) {
	if raw == "" || raw[0] == '0' || len(raw) > 5 {
		return 0, false
	}
	for _, c := range raw {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	port, err := strconv.Atoi(raw)
	return port, err == nil && port <= 65535
}

// gatewayText answers the gateway's own responses as plain text, so a
// forwarded page never renders a WebMux JSON body.
func gatewayText(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = fmt.Fprintln(w, text)
}

// secureRequest reports whether the browser reached WebMux over HTTPS: TLS
// on this listener, or a runtime HTTPS entry point (tunnel, Tailscale) that
// proxies plain HTTP to it.
func (s *Server) secureRequest(r *http.Request) bool {
	return r.TLS != nil || s.Public.hostAllowed(r.Host)
}

func (s *Server) gatewayOrigin(r *http.Request) string {
	if s.secureRequest(r) {
		return "https://" + r.Host
	}
	return "http://" + r.Host
}

func (s *Server) gateway(w http.ResponseWriter, r *http.Request) {
	if s.Forward == nil {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path == forwardAuth {
		s.redeemForward(w, r)
		return
	}
	raw, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, forwardRoot), "/")
	port, ok := forwardPort(raw)
	if r.URL.Path == "/forward" || !ok {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path == forwardRoot+raw {
		target := forwardRoot + raw + "/"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
		return
	}
	// Authentication precedes the lookup, so an unauthenticated request
	// cannot tell a forwarded port from any other.
	if !s.forwardAuthorized(r) {
		if isNavigation(r) {
			http.Redirect(w, r, "/forward-auth?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}
		gatewayText(w, http.StatusUnauthorized, "需要重新认证端口转发。请从 WebMux 打开这个地址。")
		return
	}
	f, life, ok := s.Forward.Lookup(port)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if isUpgrade(r) && !s.originAllowed(r) {
		gatewayText(w, http.StatusForbidden, "origin not allowed")
		return
	}
	// The server's request read timeout would cut long uploads and every
	// upgraded connection a dev server holds open (HMR).
	_ = http.NewResponseController(w).SetReadDeadline(time.Time{})
	// The middleware's no-referrer policy is WebMux's; a forwarded page
	// keeps its server's policy, since some dev servers rely on Referer.
	w.Header().Del("Referrer-Policy")
	// Removing the forward ends its in-flight requests and upgraded
	// connections.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(life, cancel)
	defer stop()
	ctx = context.WithValue(ctx, gatewayKey{}, gatewayRequest{forward: f, origin: s.gatewayOrigin(r), secure: s.secureRequest(r)})
	s.gatewayProxy.ServeHTTP(w, r.WithContext(ctx))
}

func isUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// isNavigation recognizes a browser's top-level page load. Browsers send
// Sec-Fetch-* only to trustworthy origins (HTTPS, localhost); on plain HTTP
// to a LAN or VPN address a page load is a GET that accepts HTML.
func isNavigation(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if mode := r.Header.Get("Sec-Fetch-Mode"); mode != "" {
		return mode == "navigate"
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// forwardAuthPage guards the SPA route /forward-auth, which redeems a ticket
// on its own. A load that another site started (or whose origin the browser
// does not report) gets confirm=1, and the page then waits for a click, so a
// foreign page cannot make the browser send authenticated requests to a
// forwarded app. The page may not be framed. It reports whether it answered.
func forwardAuthPage(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return false
	}
	query := r.URL.Query()
	if query.Get("confirm") == "1" {
		return false
	}
	query.Set("confirm", "1")
	http.Redirect(w, r, "/forward-auth?"+query.Encode(), http.StatusSeeOther)
	return true
}

// forwardAuthorized accepts a live forwarding credential cookie; with the
// main token unset the gateway, like /api/, needs none.
func (s *Server) forwardAuthorized(r *http.Request) bool {
	if s.Config.Token == "" {
		return true
	}
	for _, c := range r.CookiesNamed(forwardCookie) {
		if s.Forward.Credentials().Valid(c.Value) {
			return true
		}
	}
	return false
}

// validForwardNext admits only gateway paths, so the exchange cannot
// redirect off the WebMux origin.
func validForwardNext(next string) bool {
	if !strings.HasPrefix(next, forwardRoot) || strings.Contains(next, "//") || strings.ContainsAny(next, "\\\r\n") {
		return false
	}
	u, err := url.Parse(next)
	return err == nil && u.Scheme == "" && u.Host == "" && u.User == nil
}

// redeemForward exchanges a one-time ticket, posted by WebMux's own page,
// for the forwarding cookie. The ticket travels in the form body, never in
// a URL.
func (s *Server) redeemForward(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		gatewayText(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if r.Header.Get("Origin") == "" || !s.originAllowed(r) {
		gatewayText(w, http.StatusForbidden, "origin not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		gatewayText(w, http.StatusBadRequest, "invalid form")
		return
	}
	next := r.PostForm.Get("next")
	if !validForwardNext(next) {
		gatewayText(w, http.StatusBadRequest, "invalid next path")
		return
	}
	credential, err := s.Forward.Credentials().Redeem(r.PostForm.Get("code"))
	if err != nil {
		status := forwardRedeemStatus(err)
		if status == http.StatusUnauthorized {
			gatewayText(w, status, "转发认证已失效，请从 WebMux 重新打开。")
			return
		}
		gatewayText(w, status, "internal error")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: forwardCookie, Value: credential.Value, Path: forwardRoot,
		MaxAge: int(forward.CredentialTTL / time.Second), HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Secure: s.secureRequest(r),
	})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, next, http.StatusSeeOther)
}

type gatewayKey struct{}

// gatewayRequest is what the proxy hooks need about the browser request.
type gatewayRequest struct {
	forward forward.Forward
	// origin is the browser-facing origin of WebMux.
	origin string
	secure bool
}

// newGatewayProxy builds the gateway's reverse proxy. Its transport dials
// only through the forward service, so a request can reach nothing but a
// forwarded loopback port, never through an environment proxy.
func newGatewayProxy(forwards *forward.Service) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite:        rewriteForward,
		ModifyResponse: modifyForwardResponse,
		ErrorHandler:   forwardError,
		// Flush every write so SSE and chunked responses arrive immediately.
		FlushInterval: -1,
		// The default logger would record upstream errors that ErrorHandler
		// already answers; request data is never logged.
		ErrorLog: log.New(io.Discard, "", 0),
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
				_, p, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				port, err := strconv.Atoi(p)
				if err != nil {
					return nil, err
				}
				return forwards.Dial(ctx, port)
			},
			MaxIdleConnsPerHost: 8,
			IdleConnTimeout:     30 * time.Second,
		},
	}
}

func gatewayInfo(ctx context.Context) gatewayRequest {
	g, _ := ctx.Value(gatewayKey{}).(gatewayRequest)
	return g
}

func rewriteForward(pr *httputil.ProxyRequest) {
	g := gatewayInfo(pr.In.Context())
	f, o := g.forward, g.forward.Options
	port := strconv.Itoa(f.Port)
	prefix := "/forward/" + port
	pr.Out.URL.Scheme = "http"
	pr.Out.URL.Host = net.JoinHostPort("127.0.0.1", port)
	if o.Prefix == forward.PrefixStrip {
		pr.Out.URL.Path = strings.TrimPrefix(pr.In.URL.Path, prefix)
		if raw := pr.In.URL.RawPath; raw != "" {
			pr.Out.URL.RawPath = strings.TrimPrefix(raw, prefix)
		}
	}
	// The outbound request keeps the browser's Host unless rewritten.
	pr.Out.Host = pr.In.Host
	if o.RewriteHost {
		pr.Out.Host = "localhost:" + port
	}
	stripForwardCookie(pr.Out.Header)
	// Rewrite already drops the inbound Forwarded and X-Forwarded-For/Host/
	// Proto; the prefix header is the gateway's alone too.
	pr.Out.Header.Del("X-Forwarded-Prefix")
	if o.ForwardedHeaders {
		pr.SetXForwarded()
		proto := "http"
		if g.secure {
			proto = "https"
		}
		pr.Out.Header.Set("X-Forwarded-Proto", proto)
		pr.Out.Header.Set("X-Forwarded-Prefix", prefix)
	}
	if o.RewriteOrigin {
		local := "http://localhost:" + port
		if origin := pr.Out.Header.Get("Origin"); origin != "" && sameAuthority(origin, pr.In.Host) {
			pr.Out.Header.Set("Origin", local)
		}
		if referer := pr.Out.Header.Get("Referer"); referer != "" {
			if u, err := url.Parse(referer); err == nil && strings.EqualFold(u.Host, pr.In.Host) {
				u.Scheme, u.Host = "http", "localhost:"+port
				pr.Out.Header.Set("Referer", u.String())
			}
		}
	}
}

func sameAuthority(origin, host string) bool {
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, host)
}

// stripForwardCookie keeps every cookie except the forwarding credential,
// which belongs to WebMux and never reaches the dev server.
func stripForwardCookie(h http.Header) {
	values := h.Values("Cookie")
	if len(values) == 0 {
		return
	}
	var kept []string
	for _, line := range values {
		for part := range strings.SplitSeq(line, ";") {
			name, _, _ := strings.Cut(strings.TrimSpace(part), "=")
			if strings.TrimSpace(part) == "" || name == forwardCookie {
				continue
			}
			kept = append(kept, strings.TrimSpace(part))
		}
	}
	h.Del("Cookie")
	if len(kept) > 0 {
		h.Set("Cookie", strings.Join(kept, "; "))
	}
}

func modifyForwardResponse(res *http.Response) error {
	// A forwarded service worker must not claim a scope beyond its prefix.
	res.Header.Del("Service-Worker-Allowed")
	g := gatewayInfo(res.Request.Context())
	f := g.forward
	if !f.Options.RewriteResponse {
		return nil
	}
	prefix := "/forward/" + strconv.Itoa(f.Port)
	strip := f.Options.Prefix == forward.PrefixStrip
	if loc := res.Header.Get("Location"); loc != "" {
		res.Header.Set("Location", rewriteLocation(loc, f.Port, prefix, strip))
	}
	if acao := res.Header.Get("Access-Control-Allow-Origin"); acao != "" && localOrigin(acao, f.Port) {
		res.Header.Set("Access-Control-Allow-Origin", g.origin)
	}
	if cookies := res.Header.Values("Set-Cookie"); len(cookies) > 0 {
		res.Header.Del("Set-Cookie")
		for _, line := range cookies {
			res.Header.Add("Set-Cookie", rewriteSetCookie(line, prefix))
		}
	}
	return nil
}

// localOrigin reports whether u names the upstream itself, as a dev server
// sees it: localhost or 127.0.0.1 on the forwarded port.
func localOrigin(raw string, port int) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host, p, err := net.SplitHostPort(u.Host)
	if err != nil || p != strconv.Itoa(port) {
		return false
	}
	return strings.EqualFold(host, "localhost") || host == "127.0.0.1"
}

// rewriteLocation maps a redirect to the upstream back onto the gateway. A
// stripped forward also prefixes root-relative redirects, which would
// otherwise leave the prefix.
func rewriteLocation(loc string, port int, prefix string, strip bool) string {
	u, err := url.Parse(loc)
	if err != nil {
		return loc
	}
	if u.IsAbs() {
		if !localOrigin(loc, port) {
			return loc
		}
		u.Scheme, u.Host, u.User = "", "", nil
	} else if u.Host != "" || !strings.HasPrefix(u.Path, "/") {
		// Scheme-relative or document-relative: leave as is.
		return loc
	}
	if strip {
		u.Path = prefix + u.Path
		if u.RawPath != "" {
			u.RawPath = prefix + u.RawPath
		}
	}
	return u.String()
}

// rewriteSetCookie scopes an upstream cookie to the forward: Domain is
// dropped and Path gains the prefix unless it already has it.
func rewriteSetCookie(line, prefix string) string {
	c, err := http.ParseSetCookie(line)
	if err != nil {
		return line
	}
	c.Domain = ""
	switch {
	case c.Path == "" || c.Path == "/":
		c.Path = prefix + "/"
	case c.Path == prefix || strings.HasPrefix(c.Path, prefix+"/"):
	default:
		c.Path = prefix + c.Path
	}
	return c.String()
}

func forwardError(w http.ResponseWriter, r *http.Request, _ error) {
	port := gatewayInfo(r.Context()).forward.Port
	gatewayText(w, http.StatusBadGateway, fmt.Sprintf("端口 %d 没有可用的服务。请确认开发服务器正在运行。", port))
}
