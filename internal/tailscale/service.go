package tailscale

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ErrNotFound means no usable Tailscale CLI was found.
var ErrNotFound = errors.New("tailscale CLI not found")

const (
	defaultTimeout  = 3 * time.Second
	defaultCacheTTL = 2 * time.Second
)

// Service detects Tailscale and executes Serve/Funnel commands. Detection is
// cached and coalesced; writes are serialized by the owning controller.
type Service struct {
	opts Options

	mu       sync.Mutex
	provider string
	gen      uint64
	cached   *Status
	cachedAt time.Time
	inflight chan struct{}
	cli      string
}

// New builds a detection service. Zero values select production defaults.
func New(opts Options) *Service {
	if opts.Provider == "" {
		opts.Provider = "auto"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}
	if opts.CacheTTL <= 0 {
		opts.CacheTTL = defaultCacheTTL
	}
	if opts.LookPath == nil {
		opts.LookPath = exec.LookPath
	}
	if opts.IsWSL == nil {
		opts.IsWSL = func() bool { return false }
	}
	if opts.NetworkingMode == nil {
		opts.NetworkingMode = func(context.Context) string { return "unknown" }
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Service{opts: opts, provider: opts.Provider}
}

// Refresh invalidates the detection cache. A detection already running may
// predate the caller's write, so its result is discarded as well.
func (s *Service) Refresh() {
	s.mu.Lock()
	s.gen++
	s.cached, s.cachedAt, s.inflight = nil, time.Time{}, nil
	s.mu.Unlock()
}

// Configure switches the configured provider for later detection, Apply and
// ServeStatus. A detection that started under the previous provider can no
// longer publish its cache or CLI. The caller must not run Apply concurrently.
func (s *Service) Configure(provider string) {
	if provider == "" {
		provider = "auto"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if provider == s.provider {
		return
	}
	s.provider = provider
	s.gen++
	s.cached, s.cachedAt, s.cli, s.inflight = nil, time.Time{}, "", nil
}

// Node is one uncached probe bound to the CLI it resolved. Removing through a
// Node never consults or changes the configured provider, the cached
// detection or the CLI used by Apply, so it is safe while the API serves
// concurrent status requests.
type Node struct {
	Status   Status
	Serve    ServeStatus
	ServeErr error
	cli      string
	svc      *Service
}

// Inspect probes provider ("auto", "linux" or "windows") without the cache.
func (s *Service) Inspect(ctx context.Context, provider string) Node {
	status, cli, serve, serveErr := s.detect(ctx, provider)
	return Node{Status: status, Serve: serve, ServeErr: serveErr, cli: cli, svc: s}
}

// Remove closes the exact HTTPS port through the CLI this probe resolved.
func (n Node) Remove(ctx context.Context, httpsPort int) error {
	if n.cli == "" {
		return ErrNotFound
	}
	return n.svc.remove(ctx, n.cli, httpsPort)
}

// Detect returns a cached or freshly coalesced status.
func (s *Service) Detect(ctx context.Context) Status {
	s.mu.Lock()
	if s.cached != nil && s.opts.Now().Sub(s.cachedAt) < s.opts.CacheTTL {
		status := *s.cached
		s.mu.Unlock()
		return status
	}
	if s.inflight != nil {
		wait := s.inflight
		s.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
		}
		s.mu.Lock()
		if s.cached != nil {
			status := *s.cached
			s.mu.Unlock()
			return status
		}
		s.mu.Unlock()
		if ctx.Err() == nil {
			// The coalesced detection was discarded by Refresh or Configure.
			return s.Detect(ctx)
		}
		return Status{Provider: ProviderNone, ConfiguredProvider: s.configured(), BackendState: BackendUnknown, FunnelAvailable: FunnelUnknown, Error: ctx.Err().Error()}
	}
	wait := make(chan struct{})
	s.inflight = wait
	provider, gen := s.provider, s.gen
	s.mu.Unlock()

	status, cli, _, _ := s.detect(ctx, provider)

	s.mu.Lock()
	if gen == s.gen {
		s.cli = cli
		s.cached, s.cachedAt = &status, s.opts.Now()
	}
	if s.inflight == wait {
		s.inflight = nil
	}
	close(wait)
	s.mu.Unlock()
	return status
}

func (s *Service) configured() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.provider
}

type nodeResult struct {
	node nodeStatus
	err  error
}

// detect resolves provider and reads each CLI's node status at most once.
// It returns the chosen CLI and its serve status; it never mutates s.
func (s *Service) detect(ctx context.Context, provider string) (Status, string, ServeStatus, error) {
	status := Status{ConfiguredProvider: provider, FunnelAvailable: FunnelUnknown}
	nodes := make(map[string]nodeResult, 2)
	read := func(cli string) (nodeStatus, error) {
		if result, ok := nodes[cli]; ok {
			return result.node, result.err
		}
		node, err := s.readNode(ctx, cli)
		nodes[cli] = nodeResult{node, err}
		return node, err
	}
	inWSL := s.opts.IsWSL()
	native, nativeErr := s.locateNative(provider)
	windows, windowsErr := "", ErrNotFound
	if inWSL {
		windows, windowsErr = s.locateWindows(provider)
	}
	// Only automatic selection inside WSL needs to know whether the native
	// node runs before choosing a CLI.
	nativeRunning := false
	if provider == "auto" && inWSL && nativeErr == nil {
		if node, err := read(native); err == nil {
			nativeRunning = node.backend == BackendRunning
		}
	}
	if inWSL {
		status.WSL = &WSLStatus{NetworkingMode: s.opts.NetworkingMode(ctx)}
		if windowsErr == nil {
			status.WSL.WindowsDetected = true
			if node, err := read(windows); err == nil {
				status.WSL.WindowsRunning = node.backend == BackendRunning
				status.WSL.WindowsDNSName = node.dns
			}
		}
	}

	chosen, cli := choose(provider, inWSL, native, nativeErr, nativeRunning, windows, windowsErr)
	status.Provider = chosen
	if cli == "" {
		status.Provider = ProviderNone
		status.BackendState = BackendUnavailable
		status.Error = "Tailscale CLI not found"
		if inWSL && windowsErr == nil && windows != "" {
			status.Provider = ProviderWindows
		}
		return status, "", ServeStatus{}, ErrNotFound
	}

	node, err := read(cli)
	if err != nil {
		status.BackendState = BackendUnknown
		status.Error = err.Error()
		return status, cli, ServeStatus{}, err
	}
	status.BackendState = node.backend
	status.DNSName = node.dns
	status.HTTPSReady = node.certs
	status.FunnelAvailable = node.funnel
	serve, serveErr := s.serveStatus(ctx, cli)
	if serveErr == nil {
		status.Entries = serve.Entries
	}
	return status, cli, serve, serveErr
}

func choose(provider string, inWSL bool, native string, nativeErr error, nativeRunning bool, windows string, windowsErr error) (string, string) {
	switch provider {
	case "windows":
		if windowsErr != nil || windows == "" {
			return ProviderWindows, ""
		}
		return ProviderWindows, windows
	case "linux":
		if nativeErr != nil || native == "" {
			return ProviderLinux, ""
		}
		return ProviderLinux, native
	}
	// auto
	if !inWSL {
		if nativeErr != nil || native == "" {
			return ProviderNone, ""
		}
		return ProviderLinux, native
	}
	if nativeErr == nil && native != "" {
		if nativeRunning {
			return ProviderWSLNative, native
		}
		if windowsErr == nil && windows != "" {
			return ProviderWindows, windows
		}
		return ProviderWSLNative, native
	}
	if windowsErr == nil && windows != "" {
		return ProviderWindows, windows
	}
	return ProviderNone, ""
}

func (s *Service) command() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cli
}

func (s *Service) locateNative(provider string) (string, error) {
	if s.opts.Command != "" {
		if isWindowsCommand(s.opts.Command) && provider != "windows" {
			return "", ErrNotFound
		}
		return verifyExecutable(s.opts.Command)
	}
	path, err := s.opts.LookPath("tailscale")
	if err != nil {
		return "", ErrNotFound
	}
	return verifyExecutable(path)
}

func (s *Service) locateWindows(provider string) (string, error) {
	if s.opts.Command != "" {
		if provider != "windows" {
			return "", ErrNotFound
		}
		return verifyExecutable(s.opts.Command)
	}
	if path, err := s.opts.LookPath("tailscale.exe"); err == nil {
		return verifyExecutable(path)
	}
	for _, drive := range []string{"c", "d"} {
		path := filepath.Join("/mnt", drive, "Program Files", "Tailscale", "tailscale.exe")
		if _, err := os.Stat(path); err == nil {
			return verifyExecutable(path)
		}
	}
	return "", ErrNotFound
}

func isWindowsCommand(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".exe") || strings.HasPrefix(lower, "/mnt/")
}

func verifyExecutable(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return "", ErrNotFound
	}
	return path, nil
}

type nodeStatus struct {
	backend string
	dns     string
	certs   bool
	funnel  string
}

func (s *Service) readNode(ctx context.Context, cli string) (nodeStatus, error) {
	out, stderr, err := s.exec(ctx, cli, "status", "--json", "--peers=false")
	if err != nil {
		return nodeStatus{}, errors.New("Tailscale 状态读取失败: " + safeCLIReason(stderr, err))
	}
	var raw statusJSON
	if err := json.Unmarshal(out, &raw); err != nil {
		return nodeStatus{}, errors.New("tailscale status returned invalid JSON")
	}
	node := nodeStatus{backend: mapBackend(raw.BackendState), funnel: FunnelUnknown}
	if raw.Self != nil {
		node.dns = normalizeDNS(raw.Self.DNSName)
		node.certs = certCovers(raw.CertDomains, node.dns)
		node.funnel = funnelCapability(raw.Self.Capabilities, raw.Self.CapMap)
	}
	return node, nil
}
