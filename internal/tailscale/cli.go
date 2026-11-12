package tailscale

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type statusJSON struct {
	BackendState string      `json:"BackendState"`
	Self         *statusSelf `json:"Self"`
	CertDomains  []string    `json:"CertDomains"`
}

type statusSelf struct {
	DNSName      string          `json:"DNSName"`
	Online       bool            `json:"Online"`
	Capabilities []string        `json:"Capabilities"`
	CapMap       json.RawMessage `json:"CapMap"`
}

type serveStatusJSON struct {
	Web         map[string]serveWeb `json:"Web"`
	AllowFunnel map[string]bool     `json:"AllowFunnel"`
}

type serveWeb struct {
	Handlers    map[string]serveHandler `json:"Handlers"`
	AllowFunnel bool                    `json:"AllowFunnel"`
}

type serveHandler struct {
	Proxy string `json:"Proxy"`
}

func (s *Service) serveStatus(ctx context.Context, cli string) (ServeStatus, error) {
	out, stderr, err := s.exec(ctx, cli, "serve", "status", "--json")
	if err != nil {
		return ServeStatus{}, errors.New("Tailscale Serve 状态读取失败: " + safeCLIReason(stderr, err))
	}
	var raw serveStatusJSON
	if err := json.Unmarshal(out, &raw); err != nil {
		return ServeStatus{}, errors.New("tailscale serve status returned invalid JSON")
	}
	return parseServe(raw), nil
}

func parseServe(raw serveStatusJSON) ServeStatus {
	status := ServeStatus{web: make(map[string]HandlerInfo, len(raw.Web))}
	for key, web := range raw.Web {
		host, port, ok := splitAuthority(key)
		if !ok {
			continue
		}
		public := raw.AllowFunnel[key] || web.AllowFunnel
		info := HandlerInfo{Handlers: len(web.Handlers), Public: public}
		if handler, ok := web.Handlers["/"]; ok {
			info.Proxy = handler.Proxy
		}
		canonical := joinHostPort(host, port)
		status.web[canonical] = info
		status.Entries = append(status.Entries, Entry{Host: host, Port: port, Public: public})
	}
	return status
}

// ServeStatus reads the current entries using the selected CLI.
func (s *Service) ServeStatus(ctx context.Context) (ServeStatus, error) {
	cli := s.command()
	if cli == "" {
		status := s.Detect(ctx)
		cli = s.command()
		if cli == "" {
			return ServeStatus{}, ErrNotFound
		}
		_ = status
	}
	return s.serveStatus(ctx, cli)
}

// Apply creates the exact Serve or Funnel target. The caller persists its
// ownership intent before calling.
func (s *Service) Apply(ctx context.Context, mode string, httpsPort, localPort int) error {
	cli, err := s.requireCLI(ctx)
	if err != nil {
		return err
	}
	verb := "serve"
	if mode == "funnel" {
		verb = "funnel"
	}
	target := fmt.Sprintf("http://127.0.0.1:%d", localPort)
	_, stderr, err := s.exec(ctx, cli, verb, "--bg", "--https="+strconv.Itoa(httpsPort), target)
	if err != nil {
		return cliError(fmt.Sprintf("tailscale %s", verb), stderr, err)
	}
	return nil
}

// remove closes only the exact HTTPS port owned by this instance. A single
// `serve off` removes the handler and the Funnel flag; running `funnel off`
// first would make the follow-up fail with "handler does not exist". A missing
// handler means the entry is already gone, so retries succeed.
func (s *Service) remove(ctx context.Context, cli string, httpsPort int) error {
	port := strconv.Itoa(httpsPort)
	if _, stderr, err := s.exec(ctx, cli, "serve", "--https="+port, "off"); err != nil {
		if missingHandler(stderr) {
			return nil
		}
		return cliError("tailscale serve off", stderr, err)
	}
	return nil
}

func missingHandler(stderr []byte) bool {
	text := strings.ToLower(string(stderr))
	return strings.Contains(text, "handler does not exist") || strings.Contains(text, "no serve config")
}

func (s *Service) requireCLI(ctx context.Context) (string, error) {
	if cli := s.command(); cli != "" {
		return cli, nil
	}
	s.Refresh()
	s.Detect(ctx)
	if cli := s.command(); cli != "" {
		return cli, nil
	}
	return "", ErrNotFound
}

// PublicURL formats the HTTPS entry point for the selected mode.
func PublicURL(dns string, port int) string {
	dns = normalizeDNS(dns)
	if dns == "" {
		return ""
	}
	if port == 443 || port == 0 {
		return "https://" + dns
	}
	return "https://" + net.JoinHostPort(dns, strconv.Itoa(port))
}

func (s *Service) exec(ctx context.Context, cli string, args ...string) ([]byte, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, s.opts.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, cli, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.Stdin = nil
	cmd.Env = append(os.Environ(), "TAILSCALE_BE_CLI=1", "NO_COLOR=1")
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return stdout.Bytes(), stderr.Bytes(), errors.New("tailscale command timed out")
	}
	return stdout.Bytes(), stderr.Bytes(), err
}

// CLIError is a failed Tailscale CLI invocation. Callers may map only this
// type to user-facing guidance; controller-authored reasons stay verbatim.
type CLIError struct {
	Action string
	Detail string
}

func (e *CLIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Action, e.Detail)
}

func cliError(action string, stderr []byte, err error) error {
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		msg = err.Error()
	}
	return &CLIError{Action: action, Detail: msg}
}

// safeCLIReason maps CLI stderr to a fixed reason so detection errors never
// expose raw output (which may contain local paths or targets).
func safeCLIReason(stderr []byte, err error) string {
	text := strings.ToLower(string(stderr) + " " + err.Error())
	switch {
	case strings.Contains(text, "timed out"), strings.Contains(text, "deadline"):
		return "命令超时"
	case strings.Contains(text, "access denied"), strings.Contains(text, "permission"), strings.Contains(text, "operator"):
		return "权限不足"
	case strings.Contains(text, "needslogin"), strings.Contains(text, "needs login"), strings.Contains(text, "logged out"):
		return "需要登录"
	case strings.Contains(text, "not running"):
		return "后端未运行"
	default:
		return "命令执行失败"
	}
}

func mapBackend(state string) string {
	switch strings.ToLower(state) {
	case "running":
		return BackendRunning
	case "stopped":
		return BackendStopped
	case "needslogin", "needs-login":
		return BackendNeedsLogin
	case "":
		return BackendUnknown
	default:
		return BackendUnknown
	}
}

func normalizeDNS(name string) string {
	return strings.TrimSuffix(strings.TrimSpace(name), ".")
}

func certCovers(domains []string, dns string) bool {
	if dns == "" {
		return false
	}
	for _, domain := range domains {
		if normalizeDNS(domain) == dns {
			return true
		}
	}
	return false
}

func funnelCapability(capabilities []string, capMap json.RawMessage) string {
	seen := false
	for _, capability := range capabilities {
		seen = true
		if isFunnelCapability(capability) {
			return FunnelAvailable
		}
	}
	if len(capMap) > 0 {
		var values map[string]any
		if err := json.Unmarshal(capMap, &values); err == nil {
			seen = true
			for key := range values {
				if isFunnelCapability(key) {
					return FunnelAvailable
				}
			}
		}
	}
	if seen {
		return FunnelUnavailable
	}
	return FunnelUnknown
}

func isFunnelCapability(capability string) bool {
	lower := strings.ToLower(capability)
	return lower == "funnel" || strings.HasSuffix(lower, "/funnel") || strings.HasSuffix(lower, "/funnel-ports")
}

func splitAuthority(key string) (string, int, bool) {
	host, port, err := net.SplitHostPort(key)
	if err != nil {
		return "", 0, false
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return "", 0, false
	}
	return normalHost(host), value, true
}

func joinHostPort(host string, port int) string {
	return net.JoinHostPort(normalHost(host), strconv.Itoa(port))
}

func normalHost(host string) string {
	return strings.ToLower(strings.Trim(host, "[]"))
}
