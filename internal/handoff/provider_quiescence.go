package handoff

import "context"

func (s *Service) quiesceProvider(ctx context.Context, id string) error {
	return s.options.Hybrid.QuiesceProvider(ctx, id, func() (map[string]string, error) {
		rec, err := s.options.Hybrid.Record(id)
		if err != nil {
			return nil, err
		}
		config, err := s.options.Launch.Get(rec.ConfigID)
		if err != nil {
			return nil, err
		}
		return config.Env, nil
	})
}
