package tray

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rainchan/webmux/internal/update"
)

func (u *SelfUpdater) update(manual, install bool) {
	ctx := u.ctx
	u.clean()
	ch := u.channel()
	client := u.o.Client(ch)
	m, host, err := client.Latest(ctx)
	if err != nil {
		// The check of a channel the user left reports nothing; the new
		// channel's check follows.
		if u.channel() == ch {
			u.fail("check", err)
		}
		return
	}
	if !update.Newer(m.Version, u.o.Version) {
		// Leaving the dev channel never downgrades: the dev build stays
		// until a release passes it.
		ahead := update.AheadOfStable(ch, u.o.Version, m.Version)
		u.o.Logf("update check (%s): tray %s is current; latest %s from %s", ch, u.o.Version, m.Version, host)
		u.onChannel(ch, func() {
			u.latest, u.status = "", ahead
			if manual && ahead == "" {
				u.notice, u.noticeSeen = "托盘已是最新版本 "+u.o.Version, false
			}
		})
		return
	}
	u.o.Logf("update check (%s): tray %s, latest %s from %s", ch, u.o.Version, m.Version, host)
	if !u.onChannel(ch, func() { u.latest, u.status = m.Version, "有新版本 "+m.Version }) {
		return
	}
	if !u.o.Installed || (!install && !u.o.AutoUpdate()) {
		return
	}
	if f := u.o.FailedAttempt; !install && f.Version == m.Version && u.o.Clock.Now().Before(f.At.Add(UpdateInterval)) {
		u.o.Logf("update install: %s failed to install at %s; not retrying it automatically yet", m.Version, f.At.Format(time.RFC3339))
		return
	}
	asset := update.AssetName(m.Version, "webmux-setup-windows-"+u.o.Arch+".exe")
	target := filepath.Join(u.o.Dir, asset)
	if !u.onChannel(ch, func() { u.status = "正在下载托盘 " + m.Version + "…" }) {
		return
	}
	if err := os.MkdirAll(u.o.Dir, 0o700); err != nil {
		u.fail("download", fmt.Errorf("下载目录：%w", err))
		return
	}
	host, err = client.Fetch(ctx, m, asset, target, 0o700)
	if err != nil {
		u.fail("download", fmt.Errorf("下载 %s %s：%w", asset, m.Version, err))
		return
	}
	u.o.Logf("update download: %s %s from %s", asset, m.Version, host)
	for {
		// The user may turn automatic updates off or switch the channel
		// while this waits.
		if !install && !u.o.AutoUpdate() {
			u.o.Logf("update install: automatic updates turned off; not installing %s", m.Version)
			u.onChannel(ch, func() { u.status = "有新版本 " + m.Version })
			return
		}
		if u.o.Idle() {
			break
		}
		if !u.onChannel(ch, func() { u.status = "托盘 " + m.Version + " 已下载，当前操作完成后安装" }) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-u.o.Clock.After(UpdateIdlePoll):
		}
	}
	wait, ok := u.launch(ch, m.Version, target)
	if !ok {
		return
	}
	u.o.Logf("update install: started %s /S for %s", target, m.Version)
	u.setStatus("正在安装托盘 " + m.Version + "…")
	u.awaitInstaller(wait)
}

// launch records the attempt and starts the installer unless the channel
// changed from ch. channelMu spans the check, the record and the start,
// and every switch holds it from saving the channel to clearing the state,
// so a switch lands either before the check (nothing starts) or after the
// start (the installer of the channel in effect when it started runs).
// RecordAttempt writes a local file and Launch only starts a process; the
// lock is released before waiting for the installer.
func (u *SelfUpdater) launch(ch update.Channel, version, target string) (wait func() (int, error), ok bool) {
	u.channelMu.Lock()
	defer u.channelMu.Unlock()
	if !u.onChannel(ch, func() {}) {
		return nil, false
	}
	if err := u.o.RecordAttempt(UpdateAttempt{Version: version, At: u.o.Clock.Now()}); err != nil {
		u.fail("install", fmt.Errorf("无法记录更新尝试：%w", err))
		return nil, false
	}
	wait, err := u.o.Launch(target)
	if err != nil {
		u.clearAttempt()
		u.fail("install", fmt.Errorf("无法运行安装程序：%w", err))
		return nil, false
	}
	return wait, true
}

// awaitInstaller returns when the tray closes, when the installer exits
// or after UpdateInstallWait. A successful installer closes this tray
// before it exits, so an exit seen here means the update failed without
// closing the tray: exit code 2 or 3 (deploy/windows/webmux.nsi), and the
// attempt is cleared, as no tray was replaced. A failure after closing it
// (code 4) is never seen here; the installer restarts the old tray, which
// finds Settings.UpdateAttempt still recorded. A timeout or an unobservable
// exit keeps the attempt: the installer may still close the tray.
func (u *SelfUpdater) awaitInstaller(wait func() (int, error)) {
	type exit struct {
		code int
		err  error
	}
	// The wait cannot be cancelled; a tray that closes leaves it behind.
	exited := make(chan exit, 1)
	go func() {
		code, err := wait()
		exited <- exit{code, err}
	}()
	select {
	case <-u.ctx.Done():
	case e := <-exited:
		if e.err != nil {
			u.fail("install", fmt.Errorf("等待安装程序：%w", e.err))
			return
		}
		u.clearAttempt()
		u.fail("install", fmt.Errorf("安装程序退出（代码 %d）", e.code))
	case <-u.o.Clock.After(UpdateInstallWait):
		u.fail("install", fmt.Errorf("安装程序 %d 分钟内未完成", int(UpdateInstallWait/time.Minute)))
	}
}

// clearAttempt removes the recorded attempt of an installer that ended
// while this tray still runs, so the next start reports no failure.
func (u *SelfUpdater) clearAttempt() {
	if u.ctx.Err() != nil {
		return
	}
	if err := u.o.RecordAttempt(UpdateAttempt{}); err != nil {
		u.o.Logf("update install: clear recorded attempt: %v", err)
	}
}

func (u *SelfUpdater) fail(stage string, err error) {
	if u.ctx.Err() != nil {
		return
	}
	u.o.Logf("update %s failed: %v", stage, err)
	u.setStatus("更新失败：" + err.Error())
}
