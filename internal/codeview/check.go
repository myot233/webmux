package codeview

import (
	"context"
	"time"
)

func (s *Service) Check(ctx context.Context, id string, req CheckRequest) (Check, error) {
	r := Check{Snapshots: map[string]string{}, Directories: map[string]string{}, Changes: "unchanged", Checked: time.Now().UTC()}
	if len(req.Snapshots) > 40 || len(req.Directories) > 30 {
		return r, problem(400, "检查数量超过限制")
	}
	b, e := s.root(ctx, id, req.Context)
	if e != nil {
		return r, e
	}
	release, e := s.acquire(ctx)
	if e != nil {
		return r, e
	}
	defer release()
	var current *Changes
	if b.Diff && (req.Changes != "" || len(req.Snapshots) > 0) {
		v, err := b.changes(ctx)
		if err == nil {
			current = &v
		} else {
			r.Changes = "unknown"
		}
	}
	if req.Changes != "" && current != nil && req.Changes != current.Version {
		r.Changes = "updated"
	}
	for _, sid := range req.Snapshots {
		v, err := s.get(id, sid)
		if err != nil {
			if Status(err) == 400 {
				return r, err
			}
			r.Snapshots[sid] = "expired"
			continue
		}
		if v.context != req.Context {
			return r, problem(400, "快照上下文不匹配")
		}
		state := "unchanged"
		if v.file != nil {
			d := v.file.Document
			if d.Source != "empty" {
				next, e := b.sourceFile(ctx, d.rawPath, d.Source)
				if e != nil {
					state = "unknown"
					if Status(e) == 404 {
						state = "missing"
					}
				} else if next.Version != d.Version || next.Kind != d.Kind {
					state = "updated"
				}
			}
		} else {
			if current == nil {
				state = "unknown"
			} else {
				var found *Change
				for _, c := range current.Entries {
					if c.ID == v.diff.Change.ID {
						copy := c
						found = &copy
						break
					}
				}
				if found == nil {
					state = "updated"
				} else if found.Group != "staged" && v.diff.New.Source == "worktree" {
					next, e := b.read(found.NewPath)
					if e != nil {
						state = "unknown"
						if Status(e) == 404 {
							state = "missing"
						}
					} else if next.Version != v.diff.New.Version {
						state = "updated"
					}
				}
			}
		}
		r.Snapshots[sid] = state
	}
	for nid, version := range req.Directories {
		p, e := nodePath(nid)
		if e != nil {
			return r, e
		}
		next, e := b.listing(ctx, p, req.IncludeIgnored)
		state := "unchanged"
		if e != nil {
			state = "unknown"
		} else if next.Version != version {
			state = "updated"
		}
		r.Directories[nid] = state
	}
	return r, nil
}
