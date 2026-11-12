// Package p2psettings owns the private, persistent web override of [p2p].
// It never starts a PeerConnection or port mapping itself; callers honor its
// fail-closed result.
package p2psettings

import (
	"errors"
	"fmt"
	"slices"

	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/settingsstore"
	"github.com/rainchan/webmux/internal/statefile"
)

var (
	ErrUnavailable = errors.New("p2p settings storage unavailable")
	ErrInvalid     = errors.New("invalid p2p settings")
)

// Pointer fields distinguish complete files from truncated or missing values.
type storedFile struct {
	V           int       `json:"v"`
	Enabled     *bool     `json:"enabled"`
	PortMapping *bool     `json:"portMapping"`
	STUNServers *[]string `json:"stunServers"`
}

func (storedFile) UnavailableErr() error { return ErrUnavailable }

var spec = settingsstore.Spec[config.P2PConfig, storedFile]{
	ID:     statefile.P2PSettings,
	Decode: decodeStored,
	Normalize: func(_, next config.P2PConfig) (config.P2PConfig, error) {
		return Prepare(next)
	},
	Encode: func(c config.P2PConfig) storedFile {
		enabled, portMapping := c.Enabled, c.PortMapping
		servers := slices.Clone(c.STUNServers)
		if servers == nil {
			servers = []string{}
		}
		return storedFile{
			V:           1,
			Enabled:     &enabled,
			PortMapping: &portMapping,
			STUNServers: &servers,
		}
	},
	// A damaged store fails closed: disable P2P upgrades and port mapping.
	FailClosed: func(c *config.P2PConfig) {
		c.Enabled = false
		c.PortMapping = false
	},
	Closed: "p2p settings service is closed",
}

// Service holds an exclusive lock for its lifetime. A fault disables writes
// and turns P2P off without changing the original file.
type Service = settingsstore.Store[config.P2PConfig, storedFile]

// Open loads the override for stateDir. defaults is the validated TOML value.
func Open(stateDir string, defaults config.P2PConfig) *Service {
	return settingsstore.Open(&spec, stateDir, defaults)
}

// Inspect reads the on-disk override without locking or creating files.
func Inspect(dir string, defaults config.P2PConfig) (config.P2PConfig, bool, error) {
	return settingsstore.Inspect(&spec, dir, defaults)
}

func decodeStored(f storedFile, _ config.P2PConfig) (config.P2PConfig, error) {
	if f.V != 1 || f.Enabled == nil || f.PortMapping == nil || f.STUNServers == nil {
		return config.P2PConfig{}, errors.New("incomplete p2p settings")
	}
	return config.P2PConfig{
		Enabled:     *f.Enabled,
		PortMapping: *f.PortMapping,
		STUNServers: *f.STUNServers,
	}.Validated()
}

// Prepare validates a proposed [p2p] configuration update.
func Prepare(next config.P2PConfig) (config.P2PConfig, error) {
	out, err := next.Validated()
	if err != nil {
		return config.P2PConfig{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return out, nil
}
