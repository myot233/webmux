package api

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/push"
	"github.com/rainchan/webmux/internal/wire"
)

// Push availability reasons shared with web/src/schemas.ts.
const (
	pushDisabled          = "disabled"
	pushStorage           = "storage"
	pushQuickTunnel       = "quick_tunnel"
	pushUnsupportedOrigin = "unsupported_origin"
)

var pushOriginErrors = map[string]error{
	pushQuickTunnel:       errors.New("推送需要稳定的 HTTPS 入口：Quick Tunnel 地址每次启动都会变化，旧订阅点开即失效；请改用 Cloudflare Named Tunnel 或 Tailscale Serve/Funnel"),
	pushUnsupportedOrigin: errors.New("推送需要 HTTPS 入口或本机回环地址；可用 Tailscale Serve/Funnel、Cloudflare Named Tunnel 或其他已授权的 HTTPS 入口"),
}

// pushOrigin decides whether the page that sent r may subscribe. It returns
// the canonical origin to anchor notification links, or a reason. Any HTTPS
// origin that reached this handler already passed the Host and Origin checks,
// so it is an authorized secure entry point: the managed Tailscale or Named
// Tunnel, or a host listed in allowed_origins. Loopback is accepted over http
// and https; other plain-http origins and the local socket are not.
func (s *Server) pushOrigin(r *http.Request) (string, string) {
	if s.local {
		return "", pushUnsupportedOrigin
	}
	named := s.Config.Tunnel.NamedHostname()
	if s.TunnelSettings != nil {
		named = s.TunnelSettings.Settings().Hostname
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Same-origin GETs omit Origin. Tunnel and reverse-proxy traffic
		// reaches this server over plain HTTP, but the browser saw https.
		host := authorityHostname(r.Host)
		switch {
		case r.TLS != nil || host != "" && (host == named || s.Public.hostAllowed(r.Host)):
			origin = "https://" + r.Host
		default:
			// A reverse proxy may forward a port-stripped Host, so anchor an
			// allowed_origins match on the configured port instead.
			if authority, ok := allowedHTTPSAuthority(s.Config.AllowedOrigins, r.Host); ok {
				origin = "https://" + authority
			} else {
				origin = "http://" + r.Host
			}
		}
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || strings.ContainsAny(origin, "?#") {
		return "", pushUnsupportedOrigin
	}
	host := authorityHostname(u.Host)
	authority, ok := normalizeAuthority(u.Host)
	// Quick Tunnel addresses rotate on every start, so subscriptions anchored
	// there die. Check this before the generic HTTPS branch; a running Named
	// Tunnel also publishes a cloudflare origin and stays eligible.
	if cloudflare := s.Public.load("cloudflare"); cloudflare != nil && ok && u.Scheme == "https" &&
		cloudflare.authority == authority && authority != named {
		return "", pushQuickTunnel
	}
	switch {
	case u.Scheme == "https" && ok && authority == named:
		return "https://" + named, ""
	case (u.Scheme == "http" || u.Scheme == "https") && (host == "127.0.0.1" || host == "::1" || host == "localhost"):
		return u.Scheme + "://" + u.Host, ""
	case u.Scheme == "https" && ok:
		return "https://" + anchorAuthority(authority), ""
	}
	return "", pushUnsupportedOrigin
}

// anchorAuthority brackets a bare IPv6 literal so it can prefix an https URL.
func anchorAuthority(authority string) string {
	if strings.HasPrefix(authority, "[") {
		return authority
	}
	if ip := net.ParseIP(authority); ip != nil && ip.To4() == nil {
		return "[" + authority + "]"
	}
	return authority
}

// allowedHTTPSAuthority returns the canonical authority of an https
// allowed_origins entry whose hostname matches host. A reverse proxy may
// forward a port-stripped Host on a same-origin GET, so matching by hostname
// keeps the configured port for the notification anchor.
func allowedHTTPSAuthority(origins []string, host string) (string, bool) {
	want := authorityHostname(host)
	if want == "" {
		return "", false
	}
	exact, _ := normalizeAuthority(host)
	fallback := ""
	for _, allowed := range origins {
		u, err := url.Parse(allowed)
		if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" || u.Path != "" || strings.ContainsAny(allowed, "?#") {
			continue
		}
		if authorityHostname(u.Host) != want {
			continue
		}
		authority, ok := normalizeAuthority(u.Host)
		if !ok {
			continue
		}
		if authority == exact {
			return anchorAuthority(authority), true
		}
		if fallback == "" {
			fallback = anchorAuthority(authority)
		}
	}
	return fallback, fallback != ""
}

func pushUnavailableReason(err error) string {
	if errors.Is(err, push.ErrStorage) {
		return pushStorage
	}
	return pushDisabled
}

func pushConflict(w http.ResponseWriter, reason string, err error) {
	reply(w, http.StatusConflict, contract.PushConflict{Error: err.Error(), Reason: reason})
}

func (s *Server) pushRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/push", func(w http.ResponseWriter, r *http.Request) {
		status := contract.APIPushStatus{Enabled: s.Push != nil, Subscriptions: s.Push.Subscriptions()}
		if err := s.Push.Unavailable(); err != nil {
			status.Reason = pushUnavailableReason(err)
		} else if _, reason := s.pushOrigin(r); reason != "" {
			status.Reason = reason
		} else {
			status.Available = true
			status.PublicKey = s.Push.PublicKey()
		}
		reply(w, http.StatusOK, status)
	})
	mux.HandleFunc("POST /api/push/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		// The body is PushSubscription.toJSON(). Browsers always include
		// expirationTime (usually null); it is accepted but not stored, because
		// a stale subscription is pruned when the push service answers 404/410.
		var req contract.PushSubscriptionRequest
		if !decode(w, r, &req) {
			return
		}
		if err := s.Push.Unavailable(); err != nil {
			pushConflict(w, pushUnavailableReason(err), err)
			return
		}
		if err := push.Validate(req.Endpoint, req.Keys.P256dh, req.Keys.Auth); err != nil {
			failure(w, http.StatusBadRequest, err)
			return
		}
		origin, reason := s.pushOrigin(r)
		if reason != "" {
			pushConflict(w, reason, pushOriginErrors[reason])
			return
		}
		id, err := s.Push.Register(req.Endpoint, req.Keys.P256dh, req.Keys.Auth, origin)
		switch {
		case err != nil && pushStatus(err) != http.StatusInternalServerError:
			failure(w, pushStatus(err), err)
		case err != nil:
			slog.Error("save push subscription", "err", err)
			reply(w, http.StatusInternalServerError, contract.ErrorResponse{Error: "推送订阅保存失败，请查看服务日志"})
		default:
			reply(w, http.StatusOK, contract.PushSubscriptionCreated{ID: id})
		}
	})
	mux.HandleFunc("DELETE /api/push/subscriptions/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Push.Unregister(r.PathValue("id")); err != nil {
			if unavailable := s.Push.Unavailable(); unavailable != nil {
				pushConflict(w, pushUnavailableReason(unavailable), unavailable)
				return
			}
			slog.Error("delete push subscription", "err", err)
			reply(w, http.StatusInternalServerError, contract.ErrorResponse{Error: "推送订阅删除失败，请查看服务日志"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// The test goes to every registered device, so a desktop can check what
	// the phones show in the background. Per-device failures are reported in
	// the body; only an unavailable push service fails the request.
	mux.HandleFunc("POST /api/push/test", func(w http.ResponseWriter, r *http.Request) {
		report, err := s.Push.SendTestAll(r.Context())
		if err != nil {
			pushConflict(w, pushUnavailableReason(err), err)
			return
		}
		reply(w, http.StatusOK, wire.PushTestReport(report))
	})
}
