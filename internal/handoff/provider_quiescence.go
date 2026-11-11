package handoff

import "context"

func (s *Service) quiesceProvider(ctx context.Context, id string) error {
	rec, err := s.options.Hybrid.Record(id)
	if err != nil {
		return err
	}
	config, err := s.options.Launch.Get(rec.ConfigID)
	if err != nil {
		return err
	}
	return s.options.Hybrid.QuiesceProvider(ctx, id, config.Env)
}
