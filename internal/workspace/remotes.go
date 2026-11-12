package workspace

import (
	"context"
	"net/url"
	"sort"
	"strings"
)

// redactedSecret replaces credentials in remote URLs.
const redactedSecret = "***"

// Remotes lists the repository's configured remotes with credentials
// redacted. Raw URLs never leave this function, not even in errors or logs.
func (s *Service) Remotes(ctx context.Context, id string) (Remotes, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	r, err := s.repository(id)
	if err != nil {
		return Remotes{}, err
	}
	if err := acquire(ctx, r); err != nil {
		return Remotes{}, err
	}
	defer release(r)
	if err := s.verifyIdentity(ctx, r); err != nil {
		return Remotes{}, err
	}
	data, err := s.git(ctx, r.entry.AnchorPath, "remote", "-v")
	if err != nil {
		return Remotes{}, err
	}
	return Remotes{Remotes: parseRemotes(string(data))}, nil
}

// parseRemotes reads `git remote -v` lines "name<TAB>url (fetch|push)",
// keeping the first URL of each direction, and sorts remotes by name.
func parseRemotes(data string) []Remote {
	index := map[string]int{}
	out := []Remote{}
	for _, line := range strings.Split(data, "\n") {
		name, rest, ok := strings.Cut(line, "\t")
		if !ok || name == "" {
			continue
		}
		cut := strings.LastIndex(rest, " (")
		if cut < 0 {
			continue
		}
		raw, direction := rest[:cut], rest[cut:]
		i, known := index[name]
		if !known {
			i = len(out)
			index[name] = i
			out = append(out, Remote{Name: name})
		}
		switch direction {
		case " (fetch)":
			if out[i].FetchURL == "" {
				out[i].FetchURL = redactURL(raw)
			}
		case " (push)":
			if out[i].PushURL == "" {
				out[i].PushURL = redactURL(raw)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// redactURL hides credentials in a remote URL. A password is replaced in any
// scheme; an http(s) username alone is taken for a token and replaced too.
// ssh:// and scp-style user@host:path keep their user names. A URL with a
// scheme that net/url cannot parse loses its whole userinfo instead of being
// echoed.
func redactURL(raw string) string {
	schemeEnd := strings.Index(raw, "://")
	if schemeEnd < 0 {
		return raw
	}
	authorityStart := schemeEnd + len("://")
	authorityEnd := len(raw)
	if end := strings.IndexAny(raw[authorityStart:], "/?#"); end >= 0 {
		authorityEnd = authorityStart + end
	}
	at := strings.LastIndex(raw[authorityStart:authorityEnd], "@")
	if at < 0 {
		return raw
	}
	at += authorityStart
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User == nil {
		return raw[:authorityStart] + raw[at+1:]
	}
	user := raw[authorityStart:at]
	if _, ok := parsed.User.Password(); ok {
		name, _, _ := strings.Cut(user, ":")
		return raw[:authorityStart] + name + ":" + redactedSecret + raw[at:]
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return raw[:authorityStart] + redactedSecret + raw[at:]
	}
	return raw
}
