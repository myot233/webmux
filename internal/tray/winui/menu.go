//go:build windows

package winui

import (
	"fyne.io/systray"
)

// Pool sizes of the dynamic submenus. fyne.io/systray cannot rebuild a menu
// cheaply, so every dynamic entry is pre-created and shown or hidden.
const (
	remotePool    = 8
	forwardPool   = 8
	infoLinkPool  = 8
	candidatePool = 12
	distroPool    = 16
)

// item caches what fyne.io/systray shows, because updating a hidden item
// would insert it again.
type item struct {
	mi      *systray.MenuItem
	title   string
	enabled bool
	checked bool
	shown   bool
}

func newItem(mi *systray.MenuItem, title string) *item {
	return &item{mi: mi, title: title, enabled: true, shown: true}
}

func (it *item) set(title string, enabled, checked bool) {
	if !it.shown {
		it.mi.Show()
		it.shown = true
	}
	if title != it.title {
		it.mi.SetTitle(title)
		it.title = title
	}
	if enabled != it.enabled {
		if enabled {
			it.mi.Enable()
		} else {
			it.mi.Disable()
		}
		it.enabled = enabled
	}
	if checked != it.checked {
		if checked {
			it.mi.Check()
		} else {
			it.mi.Uncheck()
		}
		it.checked = checked
	}
}

func (it *item) hide() {
	if it.shown {
		it.mi.Hide()
		it.shown = false
	}
}

// remoteLink is one entry of the remote links.
type remoteLink struct {
	URL, Label string
}

// candidate is one entry of the forwarding addresses.
type candidate struct {
	Address string
	Title   string
	Enabled bool
	Checked bool
}

// menu holds every item; only App.render changes them.
type menu struct {
	status1, status2, hint1, hint2 *item

	open, copyLinks, copyLocal, remoteWarn *item
	remoteLinks, forwardLinks              []*item
	copyPublic, copyToken                  *item
	tunnel, tunnelInfo, tunnelSettings     *item
	pushInfo, pushGuide                    *item

	remote, fwHeadline, fwBlocked, fwInit *item
	fwLinks, fwCandidates                 []*item

	start, stop, restart, install, download *item

	logs, logService, logTray *item
	openConfig, initConfig    *item
	distro, distroDefault     *item
	distros                   []*item
	mode, modeAuto, modeTray  *item
	modeSystemd               *item
	editSettings              *item
	autostart                 *item
	quit                      *item
}

// bindings are what the dynamic items currently stand for.
type bindings struct {
	remote     []remoteLink // fills remoteLinks then forwardLinks, in order
	reported   int          // remote[:reported] fill remoteLinks
	candidates []candidate
	distros    []string
}

func add(title string) *item { return newItem(systray.AddMenuItem(title, ""), title) }

func addSub(parent *item, title string) *item {
	return newItem(parent.mi.AddSubMenuItem(title, ""), title)
}

func pool(parent *item, n int) []*item {
	items := make([]*item, n)
	for i := range items {
		items[i] = addSub(parent, "")
		items[i].hide()
	}
	return items
}

// buildMenu creates the items in display order: fyne.io/systray orders
// items by creation, so hidden pool items reappear in place.
func buildMenu() *menu {
	m := &menu{}
	m.status1 = add("WebMux")
	m.status2 = add("")
	m.hint1 = add("")
	m.hint2 = add("")
	for _, it := range []*item{m.status1, m.status2, m.hint1, m.hint2} {
		it.mi.Disable()
		it.enabled = false
	}
	m.hint1.hide()
	m.hint2.hide()
	systray.AddSeparator()

	m.open = add("打开 WebMux")
	// Every copyable link lives in one submenu: local, then the exposure
	// warning and remote links, then the public tunnel link.
	m.copyLinks = add("复制链接")
	m.copyLocal = addSub(m.copyLinks, "本机链接")
	m.remoteWarn = addSub(m.copyLinks, exposedWarning)
	m.remoteWarn.mi.Disable()
	m.remoteWarn.enabled = false
	m.remoteWarn.hide()
	m.remoteLinks = pool(m.copyLinks, remotePool)
	m.forwardLinks = pool(m.copyLinks, forwardPool)
	m.copyPublic = addSub(m.copyLinks, "公网链接")
	m.copyPublic.hide()
	m.copyLinks.hide()
	m.copyToken = add("复制 Token")
	m.copyToken.hide()
	m.tunnel = add("公网隧道（Cloudflare）")
	m.tunnelInfo = add("")
	m.tunnelInfo.mi.Disable()
	m.tunnelInfo.enabled = false
	m.tunnelSettings = add("打开公网访问设置…")
	m.pushInfo = add("")
	m.pushInfo.mi.Disable()
	m.pushInfo.enabled = false
	m.pushGuide = add("手机推送设置说明…")
	for _, it := range []*item{m.tunnel, m.tunnelInfo, m.tunnelSettings, m.pushInfo, m.pushGuide} {
		it.hide()
	}
	m.remote = add("远程访问")
	m.fwHeadline = addSub(m.remote, "")
	m.fwBlocked = addSub(m.remote, "")
	m.fwInit = addSub(m.remote, "初始化配置（生成 Token）")
	m.fwLinks = pool(m.remote, infoLinkPool)
	m.remote.mi.AddSeparator()
	m.fwCandidates = pool(m.remote, candidatePool)
	systray.AddSeparator()

	m.start = add("启动")
	m.stop = add("停止")
	m.restart = add("重启")
	m.install = add("安装 WSL 中的 webmux")
	m.install.hide()
	m.download = add("下载托盘")
	m.download.hide()
	systray.AddSeparator()

	m.logs = add("打开日志")
	m.logService = addSub(m.logs, "打开 WebMux 日志")
	m.logTray = addSub(m.logs, "打开托盘日志")
	m.openConfig = add("打开 WebMux 配置文件")
	m.initConfig = add("初始化配置（生成 Token）")
	m.initConfig.hide()
	m.distro = add("发行版")
	m.distroDefault = addSub(m.distro, "系统默认")
	m.distros = pool(m.distro, distroPool)
	m.mode = add("启动方式")
	m.modeAuto = addSub(m.mode, "自动")
	m.modeTray = addSub(m.mode, "托盘托管")
	m.modeSystemd = addSub(m.mode, "systemd 用户服务")
	m.editSettings = add("编辑托盘设置")
	systray.AddSeparator()

	m.autostart = add("登录时自动启动")
	systray.AddSeparator()

	m.quit = add("退出")
	return m
}
