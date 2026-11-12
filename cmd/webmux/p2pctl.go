package main

import (
	"context"
	"sync"

	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/p2p"
	"github.com/rainchan/webmux/internal/p2psettings"
	"github.com/rainchan/webmux/internal/wire"
)

// p2pController bridges the persistent [p2p] settings store with the running
// p2p.Service.
type p2pController struct {
	store *p2psettings.Service
	svc   *p2p.Service

	mu  sync.Mutex
	cfg config.P2PConfig
	web bool
}

func newP2PController(store *p2psettings.Service, svc *p2p.Service) *p2pController {
	cfg, web := store.Effective()
	return &p2pController{
		store: store,
		svc:   svc,
		cfg:   cfg,
		web:   web,
	}
}

func (c *p2pController) Enabled() bool {
	return c.svc != nil && c.svc.Enabled()
}

func (c *p2pController) Status() contract.P2PStatus {
	c.mu.Lock()
	cfg, web := c.cfg, c.web
	c.mu.Unlock()
	source := "config"
	if web {
		source = "web"
	}
	return wire.P2PStatus(cfg, source, c.store.Unavailable(), c.svc.Status())
}

func (c *p2pController) UpdateSettings(in contract.P2PSettingsUpdate) (contract.P2PStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	next, err := p2psettings.Prepare(config.P2PConfig{
		Enabled:     in.Enabled,
		PortMapping: in.PortMapping,
		STUNServers: in.STUNServers,
	})
	if err != nil {
		return contract.P2PStatus{}, err
	}
	if err := c.store.Save(next); err != nil {
		return contract.P2PStatus{}, err
	}
	c.cfg, c.web = c.store.Effective()
	c.svc.Reconfigure(p2p.Config{
		Enabled:     c.cfg.Enabled,
		PortMapping: c.cfg.PortMapping,
		STUNServers: c.cfg.STUNServers,
	})
	source := "config"
	if c.web {
		source = "web"
	}
	return wire.P2PStatus(c.cfg, source, c.store.Unavailable(), c.svc.Status()), nil
}

func (c *p2pController) ResetSettings() (contract.P2PStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	defaults, err := c.store.Reset()
	if err != nil {
		return contract.P2PStatus{}, err
	}
	c.cfg, c.web = defaults, false
	c.svc.Reconfigure(p2p.Config{
		Enabled:     defaults.Enabled,
		PortMapping: defaults.PortMapping,
		STUNServers: defaults.STUNServers,
	})
	return wire.P2PStatus(c.cfg, "config", c.store.Unavailable(), c.svc.Status()), nil
}

func (c *p2pController) Signal(ctx context.Context, req contract.P2PSignalRequest) (contract.P2PSignalResponse, error) {
	out, err := c.svc.Signal(ctx, wire.P2PSignalInput(req))
	if err != nil {
		return contract.P2PSignalResponse{}, err
	}
	return wire.P2PSignalResponse(out), nil
}
