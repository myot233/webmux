package handoff

import (
	"context"
	"errors"

	"github.com/rainchan/webmux/internal/launch"
)

func (s *Service) quiesceProvider(ctx context.Context, id string) error {
	return s.options.Hybrid.QuiesceProvider(ctx, id, func() (map[string]string, error) {
		rec, err := s.options.Hybrid.Record(id)
		if err != nil {
			return nil, err
		}
		config, err := s.options.Launch.Get(rec.ConfigID)
		// Live actors retain their own launch provenance and never call this
		// fallback. Archived providers can still be checked in the service
		// environment after their launch config is deleted.
		if errors.Is(err, launch.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return config.Env, nil
	})
}
