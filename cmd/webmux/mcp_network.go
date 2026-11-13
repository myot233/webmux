package main

import (
	"context"
	"net"
	"net/http"

	"github.com/rainchan/webmux/internal/agentguide"
	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/localclient"
	"github.com/rainchan/webmux/internal/mcpcontrol"
	"github.com/rainchan/webmux/internal/mcphttp"
)

// networkMCP uses the owner's authenticated TCP API, not the local socket:
// the server's process identity must never stand in for a remote caller.
func networkMCP(ctx context.Context, cfg config.Config, address net.Addr) (http.Handler, func(), error) {
	client, err := localclient.NewTCP(localOrigin(address), cfg.Token)
	if err != nil {
		return nil, nil, err
	}
	control := mcpcontrol.Client{API: client, GuideContext: agentguide.Context{Transport: "mcp-http", Locality: "remote"}}
	registry, err := control.Registry("2025-03-26", "2025-06-18", "2025-11-25")
	if err != nil {
		client.Close()
		return nil, nil, err
	}
	handler := mcphttp.New(ctx, registry)
	return handler, func() { handler.Close(); client.Close() }, nil
}
