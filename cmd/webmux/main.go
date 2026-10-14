package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/rainchan/webmux/internal/api"
	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/workspace"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("webmux", version)
		return
	}
	if err := run(); err != nil {
		slog.Error("webmux failed", "error", err)
		os.Exit(1)
	}
}
func loadConfig(args []string) (config.Config, error) {
	flags := flag.NewFlagSet("webmux", flag.ContinueOnError)
	path := flags.String("config", config.DefaultPath(), "TOML configuration path")
	listen := flags.String("listen", "", "HTTP listen address")
	token := flags.String("token", "", "optional shared token")
	if err := flags.Parse(args); err != nil {
		return config.Config{}, err
	}
	explicit := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	c, err := config.Load(*path, explicit["config"])
	if err != nil {
		return c, err
	}
	if explicit["listen"] {
		c.Listen = *listen
	}
	if explicit["token"] {
		c.Token = *token
	}
	if err = c.Validate(); err != nil {
		return c, err
	}
	return c, nil
}

func run() error {
	c, err := loadConfig(os.Args[1:])
	if err != nil {
		return err
	}
	host, _, _ := net.SplitHostPort(c.Listen)
	if (host == "0.0.0.0" || host == "::" || host == "") && c.Token == "" {
		slog.Warn("SHELL EXPOSED: listening on all interfaces without token; isolate with Tailscale/WireGuard")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return err
	}
	m := session.NewManager(c.SessionOptions())
	workspaces := workspace.New(c.StateDir, m, c.DefaultCwd)
	defer workspaces.Close()
	server := &http.Server{
		Handler:           api.New(c, m, workspaces),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       60 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	ended := make(chan error, 1)
	go func() { ended <- server.Serve(listener) }()
	slog.Info("webmux listening", "address", listener.Addr(), "version", version)
	select {
	case <-ctx.Done():
	case err = <-ended:
	}
	stop() // Cancel HTTP and hijacked WebSocket handlers before joining sessions.
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m.Shutdown(shutdown)
	if shutdownErr := server.Shutdown(shutdown); shutdownErr != nil {
		_ = server.Close()
		slog.Warn("forced HTTP shutdown", "error", shutdownErr)
	}
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
