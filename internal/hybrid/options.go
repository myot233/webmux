package hybrid

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/rainchan/webmux/internal/pathutil"
	"github.com/rainchan/webmux/internal/statefile"
)

const (
	optionsVersion = 1
	// maxAgentOptions bounds the remembered launch configs; the least
	// recently updated entry goes first. Deleting a launch config forgets
	// its entry, so the bound only matters for configs deleted elsewhere.
	maxAgentOptions = 256
	// The cache is display-only, so one config's report is cut to a budget
	// instead of letting a long model list fill the file: at most
	// maxEntryOptions options sharing about maxEntryBytes of choice text,
	// and bounded names and descriptions. Each option keeps its selected
	// choice.
	maxEntryOptions   = 32
	maxEntryBytes     = 16 << 10
	maxNameRunes      = 128
	maxDescriptionRun = 256
)

// storedAgentOptions is agent-options.json: per launch config, the agent
// options its latest chat session reported, with their last values.
type storedAgentOptions struct {
	V       int                `json:"v"`
	Configs []agentOptionEntry `json:"configs"`
}

type agentOptionEntry struct {
	ConfigID  string         `json:"configId"`
	Options   []storedOption `json:"options"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

type storedOption struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Category string         `json:"category,omitempty"`
	Type     string         `json:"type"`
	Value    string         `json:"value,omitempty"`
	Enabled  *bool          `json:"enabled,omitempty"`
	Choices  []storedChoice `json:"options"`
}

type storedChoice struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func validateAgentOptions(f storedAgentOptions) error {
	if f.V != optionsVersion || f.Configs == nil {
		return errors.New("文件损坏或版本不受支持")
	}
	if len(f.Configs) > maxAgentOptions {
		return fmt.Errorf("记录超过 %d 条", maxAgentOptions)
	}
	seen := map[string]bool{}
	for i, e := range f.Configs {
		if e.ConfigID == "" || seen[e.ConfigID] || e.Options == nil {
			return fmt.Errorf("第 %d 条记录: 启动配置无效、重复或缺少配置项", i+1)
		}
		seen[e.ConfigID] = true
		for _, o := range e.Options {
			if o.ID == "" || o.Choices == nil {
				return fmt.Errorf("第 %d 条记录: 配置项无效", i+1)
			}
		}
	}
	return nil
}

// agentOptions owns <state_dir>/agent-options.json, a display cache of the
// agent options per launch config: nothing restores or switches from it.
// A lock conflict, corrupt or unreadable file leaves it unavailable (it
// reports nothing and is left untouched) while hybrid sessions keep
// working.
type agentOptions struct {
	writeMu sync.Mutex
	mu      sync.Mutex
	path    string
	store   *statefile.Handle[storedAgentOptions]
	broken  error
	list    []agentOptionEntry
	dirty   bool
	pending *time.Timer
	closed  bool
	log     *slog.Logger
}

func openAgentOptions(stateDir string) *agentOptions {
	o := &agentOptions{log: slog.New(slog.DiscardHandler)}
	dir, err := pathutil.Expand(stateDir)
	if err == nil {
		dir, err = filepath.Abs(dir)
	}
	if err != nil {
		o.path = filepath.Join(stateDir, statefile.Describe(statefile.AgentOptions).Path)
		o.broken = fmt.Errorf("状态目录无效: %w", err)
		return o
	}
	var f storedAgentOptions
	o.store, f, err = statefile.Open[storedAgentOptions](dir, statefile.AgentOptions, validateAgentOptions)
	o.path = filepath.Join(dir, statefile.Describe(statefile.AgentOptions).Path)
	switch {
	case statefile.IsKind(err, statefile.Missing):
		o.list = []agentOptionEntry{}
	case err != nil:
		o.broken = err
	default:
		o.list = f.Configs
	}
	return o
}

func (o *agentOptions) unavailableLocked() error {
	if o.broken != nil {
		return fmt.Errorf("%w: %s（%s）", ErrUnavailable, o.broken, o.path)
	}
	if o.closed {
		return fmt.Errorf("%w: 服务已关闭", ErrUnavailable)
	}
	return nil
}

func (o *agentOptions) unavailable() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.unavailableLocked()
}

// get returns the options remembered for configID, empty when none are.
func (o *agentOptions) get(configID string) ([]ConfigOption, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.unavailableLocked(); err != nil {
		return []ConfigOption{}, err
	}
	i := slices.IndexFunc(o.list, func(e agentOptionEntry) bool { return e.ConfigID == configID })
	if i < 0 {
		return []ConfigOption{}, nil
	}
	return loadOptions(o.list[i].Options), nil
}

// remember keeps options as configID's latest report. An unchanged report
// writes nothing; a change is written coalesced, like record updates.
func (o *agentOptions) remember(configID string, options []ConfigOption, now time.Time) {
	if configID == "" {
		return
	}
	stored := storeOptions(options)
	o.writeMu.Lock()
	defer o.writeMu.Unlock()
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.unavailableLocked() != nil {
		return
	}
	i := slices.IndexFunc(o.list, func(e agentOptionEntry) bool { return e.ConfigID == configID })
	if i >= 0 && reflect.DeepEqual(o.list[i].Options, stored) {
		return
	}
	next := slices.Clone(o.list)
	if i >= 0 {
		next = slices.Delete(next, i, i+1)
	}
	next = append(next, agentOptionEntry{ConfigID: configID, Options: stored, UpdatedAt: now.UTC()})
	if extra := len(next) - maxAgentOptions; extra > 0 {
		next = next[extra:]
	}
	// Oldest first: drop reports until the file fits its byte limit.
	for len(next) > 1 && !o.store.Fits(storedAgentOptions{V: optionsVersion, Configs: next}) {
		next = next[1:]
	}
	o.list = next
	o.dirty = true
	o.scheduleLocked()
}

// forget drops configID's options and writes the file now: the launch
// config is gone.
func (o *agentOptions) forget(configID string) error {
	o.writeMu.Lock()
	defer o.writeMu.Unlock()
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.unavailableLocked(); err != nil {
		return err
	}
	i := slices.IndexFunc(o.list, func(e agentOptionEntry) bool { return e.ConfigID == configID })
	if i < 0 {
		return nil
	}
	next := slices.Delete(slices.Clone(o.list), i, i+1)
	if err := o.commitLocked(next); err != nil {
		return err
	}
	o.list = next
	return nil
}

// setLogger sets where failed coalesced writes are reported.
func (o *agentOptions) setLogger(log *slog.Logger) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.log = log
}

func (o *agentOptions) scheduleLocked() {
	o.scheduleAfterLocked(persistDelay)
}

func (o *agentOptions) scheduleAfterLocked(delay time.Duration) {
	if o.pending != nil || !o.dirty {
		return
	}
	o.pending = time.AfterFunc(delay, func() {
		o.writeMu.Lock()
		defer o.writeMu.Unlock()
		o.mu.Lock()
		defer o.mu.Unlock()
		o.pending = nil
		if o.unavailableLocked() != nil {
			return
		}
		if err := o.commitLocked(o.list); err != nil {
			o.log.Error("hybrid agent options write failed; changes are not persisted", "error", err)
		}
	})
}

// commitLocked commits one write transaction with mu released during I/O.
func (o *agentOptions) commitLocked(list []agentOptionEntry) error {
	if o.pending != nil {
		o.pending.Stop()
		o.pending = nil
	}
	if list == nil {
		list = []agentOptionEntry{}
	}
	wasDirty := o.dirty
	o.dirty = true
	o.mu.Unlock()
	err := o.store.Commit(storedAgentOptions{V: optionsVersion, Configs: list})
	o.mu.Lock()
	if err != nil {
		o.dirty = wasDirty
		if wasDirty && retryable(err) {
			o.scheduleAfterLocked(retryDelay)
		}
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	o.dirty = false
	return nil
}

// close writes any pending change and releases the file lock.
func (o *agentOptions) close() {
	o.writeMu.Lock()
	defer o.writeMu.Unlock()
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return
	}
	if o.pending != nil {
		o.pending.Stop()
		o.pending = nil
	}
	if o.dirty && o.broken == nil {
		if err := o.commitLocked(o.list); err != nil {
			o.log.Error("hybrid agent options final write failed; changes are not persisted", "error", err)
		}
	}
	o.closed = true
	o.mu.Unlock()
	o.store.Close()
}

// storeOptions converts a report for the cache within the entry budget.
func storeOptions(in []ConfigOption) []storedOption {
	n := min(len(in), maxEntryOptions)
	out := make([]storedOption, 0, n)
	// Each option gets an equal share, so one long model list cannot
	// starve the others.
	share := maxEntryBytes / max(n, 1)
	for _, c := range in {
		if len(out) >= maxEntryOptions {
			break
		}
		used := 0
		s := storedOption{ID: c.ID, Name: clipRunes(c.Name, maxNameRunes), Category: c.Category, Type: c.Type, Value: c.Value, Choices: []storedChoice{}}
		used += len(s.ID) + len(s.Name) + len(s.Category) + len(s.Type) + len(s.Value)
		if c.Enabled != nil {
			enabled := *c.Enabled
			s.Enabled = &enabled
		}
		for _, choice := range c.Options {
			selected := choice.Value == c.Value
			if used >= share && !selected {
				continue
			}
			sc := storedChoice{Value: choice.Value, Name: clipRunes(choice.Name, maxNameRunes), Description: clipRunes(choice.Description, maxDescriptionRun)}
			used += len(sc.Value) + len(sc.Name) + len(sc.Description)
			s.Choices = append(s.Choices, sc)
		}
		out = append(out, s)
	}
	return out
}

// clipRunes cuts s to at most n runes.
func clipRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func loadOptions(in []storedOption) []ConfigOption {
	out := make([]ConfigOption, 0, len(in))
	for _, s := range in {
		c := ConfigOption{ID: s.ID, Name: s.Name, Category: s.Category, Type: s.Type, Value: s.Value, Options: make([]ConfigChoice, 0, len(s.Choices))}
		if s.Enabled != nil {
			enabled := *s.Enabled
			c.Enabled = &enabled
		}
		for _, choice := range s.Choices {
			c.Options = append(c.Options, ConfigChoice(choice))
		}
		out = append(out, c)
	}
	return out
}

// AgentOptions returns the agent options the latest chat session of launch
// config configID reported, with their last values; empty when no chat
// session of it has reported any. An unreadable agent-options.json is
// ErrUnavailable, with empty options.
func (s *Service) AgentOptions(configID string) ([]ConfigOption, error) {
	return s.options.get(configID)
}

// ForgetAgentOptions drops the options remembered for a deleted launch
// config.
func (s *Service) ForgetAgentOptions(configID string) error {
	return s.options.forget(configID)
}

// logAgentOptions reports an unusable agent-options.json once at startup.
func logAgentOptions(log *slog.Logger, o *agentOptions) {
	if err := o.unavailable(); err != nil {
		log.Warn("agent options unavailable", "error", err)
	}
}
