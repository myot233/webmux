package tray

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rainchan/webmux/internal/update"
)

// Timing of the tray's own update checks; see windows-tray-self-update.
const (
	UpdateFirstCheck = 2 * time.Minute
	UpdateInterval   = 24 * time.Hour
	// UpdateIdlePoll is how often a downloaded installer waits for the
	// tray to become idle.
	UpdateIdlePoll = 5 * time.Second
	// UpdateInstallWait bounds how long a running installer keeps the
	// updater busy when it has not closed this tray.
	UpdateInstallWait = 15 * time.Minute

	updateCleanRetry   = 10 * time.Second
	updateCleanRetries = 30
)

// ReleaseClient reads signed releases; update.Client implements it.
type ReleaseClient interface {
	Latest(ctx context.Context) (update.Manifest, string, error)
	Fetch(ctx context.Context, m update.Manifest, asset, target string, mode os.FileMode) (string, error)
}

// SelfUpdateOptions configure a SelfUpdater.
type SelfUpdateOptions struct {
	// Version is the tray version; Arch the Windows architecture of the
	// installer ("amd64", "arm64").
	Version, Arch string
	// Release is update.IsRelease(): only release builds check.
	Release bool
	// Installed is true for a tray running from the directory the
	// installer registered; any other tray is portable and never
	// downloads or runs an installer.
	Installed bool
	// Dir holds the downloaded installer (%LOCALAPPDATA%\WebMux\updates).
	Dir string
	// Client returns the release client of a channel; each check uses
	// the channel current at its start.
	Client func(update.Channel) ReleaseClient
	// Channel reads Settings.Channel at the time of each decision: the
	// channel, and why the stored value was not recognised. nil means
	// stable.
	Channel func() (update.Channel, error)
	// SetChannel saves Settings.UpdateChannel when the user picks one.
	SetChannel func(update.Channel)
	Clock      Clock
	// AutoUpdate reads Settings.AutoUpdate at the time of each decision.
	AutoUpdate func() bool
	// Idle reports that no server install, start, stop or restart runs and
	// no dialog is open, so the installer may close the tray.
	Idle func() bool
	// RecordAttempt saves Settings.UpdateAttempt before an installer
	// starts; the installer does not run when it fails.
	RecordAttempt func(UpdateAttempt) error
	// Launch starts the installer silently and detached; wait reports its
	// exit code once it exits.
	Launch func(path string) (wait func() (int, error), err error)
	// Publish is called after every change of View.
	Publish func()
	Logf    func(format string, args ...any)
	// Updated is the version announced once as "已更新到", from
	// RecordVersion; "" announces nothing.
	Updated string
	// FailedAttempt is the failed install RecordVersion found. It is
	// announced once, and checks the user did not ask to install skip its
	// release until UpdateInterval after the attempt.
	FailedAttempt UpdateAttempt
}

// SelfUpdateView is what the menu shows about the tray's own update.
type SelfUpdateView struct {
	Release, Installed bool
	Channel            update.Channel
	// Busy is true while a check, download or install runs.
	Busy bool
	// Latest is a newer release found by the latest check, or "".
	Latest string
	// Status is the status line, or "".
	Status string
}

// SelfUpdater checks for newer tray releases and, for an installed tray,
// downloads the signed installer and runs it silently once the tray is
// idle. The installer then closes this tray as with "仅退出托盘".
type SelfUpdater struct {
	o      SelfUpdateOptions
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// channelMu orders channel switches against starting an installer;
	// see launch. It is taken before mu and the Options callbacks' locks.
	channelMu sync.Mutex

	mu     sync.Mutex
	busy   bool
	latest string
	status string
	// queued is a check the user's channel switch asked for while another
	// operation ran; it starts once that ends. queuedInstall installs too.
	queued, queuedInstall bool
	// notice is shown until the menu opened twice: once to show it.
	notice     string
	noticeSeen bool
	// servers are newer WSL versions that already triggered a check.
	servers map[string]bool
}

// NewSelfUpdater returns an updater; Start runs its schedule.
func NewSelfUpdater(o SelfUpdateOptions) *SelfUpdater {
	if o.Clock == nil {
		o.Clock = RealClock
	}
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	if o.Publish == nil {
		o.Publish = func() {}
	}
	if o.AutoUpdate == nil {
		o.AutoUpdate = func() bool { return false }
	}
	if o.Channel == nil {
		o.Channel = func() (update.Channel, error) { return update.ChannelStable, nil }
	}
	if o.SetChannel == nil {
		o.SetChannel = func(update.Channel) {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	u := &SelfUpdater{o: o, ctx: ctx, cancel: cancel, servers: map[string]bool{}}
	if o.Updated != "" {
		u.notice = "已更新到 " + o.Updated
		o.Logf("tray updated to %s", o.Updated)
	}
	if f := o.FailedAttempt; !f.IsZero() {
		u.notice = "更新到 " + f.Version + " 失败，已恢复原托盘"
		o.Logf("tray update to %s started at %s did not complete; not installing it automatically before %s",
			f.Version, f.At.Format(time.RFC3339), f.At.Add(UpdateInterval).Format(time.RFC3339))
	}
	return u
}

// Start removes earlier downloads, then checks UpdateFirstCheck after the
// start and every UpdateInterval while automatic updates are on.
func (u *SelfUpdater) Start() {
	u.wg.Go(u.schedule)
}

// Close stops the schedule and any check or download, and waits for them.
func (u *SelfUpdater) Close() {
	u.cancel()
	u.wg.Wait()
}

// Check checks now, from the menu; it does not depend on AutoUpdate.
func (u *SelfUpdater) Check() { u.spawn(true, false) }

// Install checks now and installs a newer release on an installed tray,
// whether or not automatic updates are on.
func (u *SelfUpdater) Install() { u.spawn(true, true) }

// SelectChannel saves the channel the user picked from "更新通道" and
// checks it at once. Picking the current channel only rewrites a stored
// value that was not recognised.
func (u *SelfUpdater) SelectChannel(ch update.Channel) {
	if !u.o.Release {
		return
	}
	u.channelMu.Lock()
	defer u.channelMu.Unlock()
	current, warn := u.o.Channel()
	if ch == current {
		if warn != nil {
			u.o.SetChannel(ch)
		}
		return
	}
	u.o.Logf("update: channel %s selected", ch)
	u.o.SetChannel(ch)
	u.switched(false)
}

// SwitchToDev serves "切换到开发版通道并更新托盘": a newer development
// build in WSL is never offered on the stable channel, so the tray moves
// to the dev channel and installs as if the user picked "更新到".
func (u *SelfUpdater) SwitchToDev() {
	if !u.o.Release {
		return
	}
	u.channelMu.Lock()
	defer u.channelMu.Unlock()
	u.o.Logf("update: switching to the dev channel to follow WSL webmux")
	u.o.SetChannel(update.ChannelDev)
	u.switched(true)
}

// ChannelChanged reports a channel changed outside the menu (tray.json was
// edited): what the previous channel offered is forgotten and the new one
// checked, as after SelectChannel.
func (u *SelfUpdater) ChannelChanged() {
	if !u.o.Release {
		return
	}
	u.channelMu.Lock()
	defer u.channelMu.Unlock()
	u.o.Logf("update: channel changed to %s in the settings file", u.channel())
	u.switched(false)
}

// switched forgets what the previous channel offered and checks the new
// one: now, or once the running operation ends. That operation installs
// nothing it found on the previous channel. The last switch decides
// whether the queued check installs.
func (u *SelfUpdater) switched(install bool) {
	u.mu.Lock()
	u.latest, u.status = "", ""
	now := !u.busy && u.ctx.Err() == nil
	if now {
		u.busy = true
	} else {
		u.queued, u.queuedInstall = true, install
	}
	u.mu.Unlock()
	u.o.Publish()
	if now {
		u.wg.Go(func() { u.work(true, install) })
	}
}

// channel is the channel in effect; an unrecognised stored value is
// stable.
func (u *SelfUpdater) channel() update.Channel {
	ch, _ := u.o.Channel()
	return ch
}

// Observe checks once per newer WSL webmux version when automatic updates
// are on, so an installed tray catches up with its server. A version counts
// only once its check started, so one that arrives while another operation
// runs is checked on a later snapshot. A development build is never offered
// on the stable channel, so it triggers no check there.
func (u *SelfUpdater) Observe(snap Snapshot) {
	version := snap.Installed.Version
	if _, ok := update.ParseVersion(version); !ok || !snap.Compat.Newer() || !u.o.Release || !u.o.Installed || !u.o.AutoUpdate() {
		return
	}
	if update.IsDevVersion(version) && u.channel() != update.ChannelDev {
		return
	}
	u.start(false, false, func() bool {
		if u.servers[version] {
			return false
		}
		u.servers[version] = true
		u.o.Logf("update: WSL webmux %s is newer than tray %s; checking", version, u.o.Version)
		return true
	})
}

// MenuOpened dismisses a notice the user has already seen in an earlier
// opening of the menu.
func (u *SelfUpdater) MenuOpened() {
	u.mu.Lock()
	changed := false
	if u.notice != "" {
		if u.noticeSeen {
			u.notice, u.noticeSeen, changed = "", false, true
		} else {
			u.noticeSeen = true
		}
	}
	u.mu.Unlock()
	if changed {
		u.o.Publish()
	}
}

// View returns the current state for the menu.
func (u *SelfUpdater) View() SelfUpdateView {
	ch := u.channel()
	u.mu.Lock()
	defer u.mu.Unlock()
	var lines []string
	for _, line := range []string{u.notice, u.status} {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return SelfUpdateView{
		Release:   u.o.Release,
		Installed: u.o.Installed,
		Channel:   ch,
		Busy:      u.busy,
		Latest:    u.latest,
		Status:    strings.Join(lines, "；"),
	}
}

func (u *SelfUpdater) schedule() {
	next := u.o.Clock.After(UpdateFirstCheck)
	// The installer that started this tray is still finishing, so its
	// file may resist the first removals.
	var retry <-chan time.Time
	retries := 0
	if !u.tidy() {
		retry = u.o.Clock.After(updateCleanRetry)
	}
	for {
		select {
		case <-u.ctx.Done():
			return
		case <-retry:
			retry = nil
			if retries++; !u.tidy() && retries < updateCleanRetries {
				retry = u.o.Clock.After(updateCleanRetry)
			}
			continue
		case <-next:
		}
		next = u.o.Clock.After(UpdateInterval)
		if u.o.Release && u.o.AutoUpdate() {
			u.run(false, false)
		} else {
			u.tidy()
		}
	}
}

// tidy removes earlier downloads unless an update runs, and reports
// whether none remain.
func (u *SelfUpdater) tidy() bool {
	if !u.begin(nil) {
		return true
	}
	defer u.end()
	return u.clean()
}

func (u *SelfUpdater) spawn(manual, install bool) { u.start(manual, install, nil) }

// start claims the updater and runs one check in the background; claim,
// when set, runs under the lock once nothing else holds the updater and
// may refuse the check.
func (u *SelfUpdater) start(manual, install bool, claim func() bool) {
	if !u.o.Release || u.ctx.Err() != nil || !u.begin(claim) {
		return
	}
	u.wg.Go(func() { u.work(manual, install) })
}

// begin claims the updater; it fails while another operation runs, which
// includes an installer that has not exited, or when claim refuses.
func (u *SelfUpdater) begin(claim func() bool) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.busy || (claim != nil && !claim()) {
		return false
	}
	u.busy = true
	return true
}

// end releases the updater, or hands it to a check a channel switch
// queued meanwhile.
func (u *SelfUpdater) end() {
	u.mu.Lock()
	next, install := u.queued && u.ctx.Err() == nil, u.queuedInstall
	u.queued, u.queuedInstall = false, false
	u.busy = next
	u.mu.Unlock()
	u.o.Publish()
	if next {
		u.wg.Go(func() { u.work(true, install) })
	}
}

// onChannel applies edit unless the channel changed from ch, and reports
// whether it did. A switch saves the channel before it clears the state
// under the same lock, so nothing found on the previous channel survives.
func (u *SelfUpdater) onChannel(ch update.Channel, edit func()) bool {
	u.mu.Lock()
	ok := u.channel() == ch
	if ok {
		edit()
	}
	u.mu.Unlock()
	if !ok {
		u.o.Logf("update: channel changed from %s; dropping this check", ch)
		return false
	}
	u.o.Publish()
	return true
}

func (u *SelfUpdater) set(edit func()) {
	u.mu.Lock()
	edit()
	u.mu.Unlock()
	u.o.Publish()
}

func (u *SelfUpdater) setStatus(status string) {
	u.set(func() { u.status = status })
}

// run is one check unless another operation runs.
func (u *SelfUpdater) run(manual, install bool) {
	if u.begin(nil) {
		u.work(manual, install)
	}
}

// work is one claimed check, and for an installed tray the download and
// install; the updater stays busy until the installer exits.
func (u *SelfUpdater) work(manual, install bool) {
	u.o.Publish()
	u.update(manual, install)
	u.end()
}

// clean removes earlier downloads and reports whether none remain; an
// installer that still runs keeps its file until a later attempt.
func (u *SelfUpdater) clean() bool {
	if u.o.Dir == "" {
		return true
	}
	entries, err := os.ReadDir(u.o.Dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true
		}
		u.o.Logf("update cleanup: %v", err)
		return false
	}
	clean := true
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(u.o.Dir, e.Name())); err != nil {
			u.o.Logf("update cleanup: %v", err)
			clean = false
		}
	}
	return clean
}
