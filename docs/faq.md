# 常见问题与获取帮助

遇到问题时先在本页找答案，再按“排障入口”定位到对应主题的诊断步骤；仍无法解决时按 [获取帮助](#获取帮助) 反馈。功能说明见 [使用指南](usage.md)，文档全集见 [文档中心](README.md)。

## 常见问题

<details>
<summary><b>重启服务后会话去哪了？</b></summary>

会话、PTY 和缓冲只保存在内存中，服务重启即结束所有会话。这是有意的设计边界：WebMux 不使用数据库、不做守护进程分离，也不与 tmux 互通。只有显式登记的状态文件（仓库清单、启动配置、偏好等，见 [持久化清单](persistence.md)）会持久保存，会话本身不会。

例外是 [混合会话](hybrid-sessions.md)：WebMux 记录它与 agent 会话的对应关系，重启后会自动恢复仍在运行的会话；手动归档的会话可在首页所属工作树的“已归档”中恢复，对话本身由 agent 保存。

</details>

<details>
<summary><b>聊天模式（快速开始）和工作台模式有什么区别？怎么选？</b></summary>

两者只是侧栏与主区域的导航组织方式不同，底层会话完全互通，可随时在侧栏左上角切换（当前浏览器会记住上一次的选择）：

- **工作台模式（首次打开默认）**：侧栏按“仓库 → 工作树 → 会话”树状分组，在侧栏选择「全部仓库」即可在主区域查看所有工作树卡片与终端实时预览；适合同时管理多个仓库、长期终端会话与多窗格分屏。
- **聊天模式**：切到聊天模式后点击侧栏顶部「新聊天」进入 **快速开始（`/new`）**，选好仓库、工作树方式（默认从基准分支新建独立 worktree，也可选主工作区或已有工作树）和 agent 后直接输入任务，WebMux 会启动聊天并进入会话；适合围绕单次任务高频发起 AI 对话。

</details>

<details>
<summary><b>创建会话时找不到 claude / codex？</b></summary>

systemd 和 launchd 不会自动加载你的交互 shell 配置（如 `~/.bashrc` 或 `~/.zshrc` 里由 nvm、bun、mise、cargo 配置的 `PATH`）。

最简单的解决方式（按优先级选择）：

1. 在“设置 → 启动配置”中编辑对应配置，勾选 **“加载 shell 启动文件”**（可选 Bash 或 Zsh，新安装的预置配置已默认开启 Bash）。
2. 若使用一次性自定义命令，可显式写成 `/usr/bin/env bash -ic 'exec opencode "$@"' opencode`，或把程序目录加入服务的 `PATH`，参见 [agent 的 PATH](deployment/services.md#agent-的-path)。

</details>

<details>
<summary><b>两台设备同时打开同一会话，布局互相影响？</b></summary>

两端共享同一个 PTY，但只有控制端决定终端尺寸：先打开的设备控制，后打开的设备观察（按原网格显示、可平移）。在观察端输入或点击“接管控制”即可切换。

</details>

<details>
<summary><b>粘贴 / 复制没有权限？</b></summary>

HTTP 不是浏览器安全上下文，Clipboard API 不可用。WebMux 会回退到文本框手动粘贴 / 复制；也可以通过 `tailscale serve` 或 Caddy 提供 HTTPS，见 [HTTPS](deployment/remote-access.md#https)。

</details>

<details>
<summary><b>重连后首行有少量乱码？</b></summary>

缓冲按字节环形存储，最旧的数据可能从多字节字符或转义序列中间开始，属于设计允许范围。

</details>

<details>
<summary><b>忘了 Token，或者浏览器提示需要登录？</b></summary>

首次启动生成的 token 持久化在配置文件中，可用 `webmux status --json --reveal-token` 查看，或用 `webmux config path` 找到配置文件。详见 [配置](deployment/configuration.md)。

</details>

<details>
<summary><b>升级后会话都没了？</b></summary>

升级需要重启服务，重启会结束所有终端会话。升级前先让 agent 告一段落，见 [升级](deployment/install-and-upgrade.md#升级)。

</details>

## 排障入口

- 安装、升级与服务状态：[持久化升级与诊断](deployment/install-and-upgrade.md#持久化升级与诊断)、[服务管理命令](deployment/services.md#服务管理命令)。
- 平台行为差异（Linux / macOS / WSL）：[平台差异](deployment/services.md#平台差异)。
- Windows 托盘与 WSL：[localhost 转发诊断](deployment/windows-wsl.md#localhost-转发诊断)、[Token 与剪贴板](deployment/windows-wsl.md#token-与剪贴板)。
- 公网入口：[Cloudflare Tunnel 故障排查](cloudflare-tunnel.md#9-故障排查)、[Tailscale 手工移除与排错](tailscale.md#手工移除与排错)。
- 手机推送收不到：[手机推送（Web Push）](deployment/pwa-and-push.md#手机推送web-push)。
- 混合会话无法切换到聊天：[切换按钮不可用的原因](hybrid-sessions.md#切换按钮不可用的原因)、[启动命令兼容要求](hybrid-sessions/agents.md#启动命令兼容要求)、[注意事项](hybrid-sessions.md#注意事项)。
- agent 没有触发“需要你介入”提醒：[需要你介入的提醒](usage/attention.md)。

## 获取帮助

- **提问或报告缺陷**：在 [GitHub Issues](https://github.com/mzdluo123/webmux/issues) 新建 issue，附上 `webmux version`、操作系统、浏览器与复现步骤。
- **不要公开安全漏洞**：按 [安全策略](../SECURITY.md#报告漏洞) 私下报告。
- **日志**：用户服务可用 `webmux service logs -f` 查看；日志不包含终端输入输出，但请在粘贴前检查是否含有路径等隐私信息。
- **想参与修复**：从 [参与贡献](../CONTRIBUTING.md) 开始。
