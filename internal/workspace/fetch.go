package workspace

import (
	"context"
	"log/slog"
	"time"
)

// fetchRemoteTimeout bounds the fetch of one remote by default.
const fetchRemoteTimeout = 60 * time.Second

// fetchRemotesTimeout bounds a whole FetchRemotes run; remotes it leaves
// unfetched are reported as timed out.
const fetchRemotesTimeout = 3 * time.Minute

// FetchResult is the outcome of fetching one remote; Message is empty when
// Status is "ok".
type FetchResult struct {
	Remote  string `json:"remote"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// FetchRemotesResult lists each remote's outcome and the repository as it
// is after the fetch.
type FetchRemotesResult struct {
	Results    []FetchResult `json:"results"`
	Repository Repository    `json:"repository"`
}

// fetchFlight is a running FetchRemotes shared by every caller that asks
// meanwhile. Unlike a snapshot it is not cancelled when its callers leave:
// an interrupted fetch only makes the user retry.
type fetchFlight struct {
	done   chan struct{}
	result FetchRemotesResult
	err    error
}

// FetchRemotes fetches every remote of the repository in name order with
// pruning, then rereads the repository. One remote's failure does not stop
// the others. It holds no repository gate, so worktree operations and
// snapshots go on meanwhile; a caller that leaves does not stop it.
func (s *Service) FetchRemotes(ctx context.Context, repositoryID string) (FetchRemotesResult, error) {
	r, err := s.repository(repositoryID)
	if err != nil {
		return FetchRemotesResult{}, err
	}
	r.fetchMu.Lock()
	flight := r.fetching
	if flight == nil {
		flight = &fetchFlight{done: make(chan struct{})}
		r.fetching = flight
		runCtx := context.WithoutCancel(ctx)
		go func() {
			flight.result, flight.err = s.fetchRemotes(runCtx, r)
			r.fetchMu.Lock()
			r.fetching = nil
			r.fetchMu.Unlock()
			close(flight.done)
		}()
	}
	r.fetchMu.Unlock()
	select {
	case <-ctx.Done():
		return FetchRemotesResult{}, ctx.Err()
	case <-flight.done:
		return flight.result, flight.err
	}
}

func (s *Service) fetchRemotes(ctx context.Context, r *repositoryState) (FetchRemotesResult, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchRemotesTimeout)
	defer cancel()
	anchor := r.entry.AnchorPath
	remotes, err := s.remoteNames(ctx, r)
	if err != nil {
		return FetchRemotesResult{}, err
	}
	if len(remotes) == 0 {
		return FetchRemotesResult{}, problemf(ErrConflict, "仓库没有配置远程，无法获取")
	}
	timeout := s.fetchRemoteTimeout
	if timeout <= 0 {
		timeout = fetchRemoteTimeout
	}
	results := make([]FetchResult, 0, len(remotes))
	for _, remote := range remotes {
		result := FetchResult{Remote: remote, Status: remoteOK}
		var failure *RemoteFailure
		if ctx.Err() != nil {
			failure = &RemoteFailure{Status: remoteTimeout, Message: remoteMessage(remoteTimeoutText, "")}
		} else {
			remoteCtx, remoteCancel := context.WithTimeout(ctx, timeout)
			_, failure = s.remoteGit(remoteCtx, anchor, "fetch", "--prune", "--recurse-submodules=no", "--end-of-options", remote)
			remoteCancel()
		}
		if failure != nil {
			result.Status, result.Message = failure.Status, failure.Message
			// URLs and Git output stay out of the log: they can hold credentials.
			slog.Warn("remote fetch failed", "repository", r.entry.ID, "remote", remote, "status", failure.Status)
		}
		results = append(results, result)
	}
	return FetchRemotesResult{Results: results, Repository: s.fetchedRepository(r)}, nil
}

// remoteNames lists the configured remotes, sorted by name.
func (s *Service) remoteNames(ctx context.Context, r *repositoryState) ([]string, error) {
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	if err := s.verifyIdentity(readCtx, r); err != nil {
		return nil, err
	}
	data, err := s.git(readCtx, r.entry.AnchorPath, "remote", "-v")
	if err != nil {
		return nil, err
	}
	remotes := parseRemotes(string(data))
	names := make([]string, len(remotes))
	for i, remote := range remotes {
		names[i] = remote.Name
	}
	return names, nil
}

// fetchedRepository rereads the repository after a fetch. It does not join
// a shared snapshot, which may have started before the fetch ended.
func (s *Service) fetchedRepository(r *repositoryState) Repository {
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	if err := acquire(ctx, r); err != nil {
		return s.unavailableRepository(r, err)
	}
	defer release(r)
	return s.readRepository(ctx, r)
}
