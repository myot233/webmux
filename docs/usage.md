# 使用指南

WebMux 把服务器上的 CLI agent 或 Shell 会话跑在后台，你可以随时从电脑或手机的浏览器接管。关闭标签页或锁屏不会结束会话；服务重启才会。

## 第一次使用

1. **想直接把任务交给 agent**：点左上角切到**聊天模式**，选好仓库、工作树方式（默认从基准分支新建 worktree，也可选已有工作树）与 agent，写下任务并发送。
   服务端会按所选方式启动聊天 agent 并发出第一条消息。见 [聊天模式](usage/chat.md)。
2. **想管理多个终端或查看全局状态**：切到**工作台模式**，点右上角「新建会话」，选择目录与启动配置（如 Claude Code、Codex、Shell）进入终端。
   见 [会话与工作台](usage/sessions.md)。
3. **随时审阅与接管**：会话页顶栏的“聊天 | 终端”切换混合会话的视图，「代码」在旁边打开文件树与 Git Diff。

## 你想做什么

| 我想……                                         | 看这里                                               |
| ---------------------------------------------- | ---------------------------------------------------- |
| 把任务交给 agent、在手机上看回复               | [聊天模式](usage/chat.md)                            |
| 在聊天与原生 TUI 之间切换同一段对话            | [混合会话](hybrid-sessions.md)                       |
| 新建会话、看懂会话状态、调整预览与设置         | [会话与工作台](usage/sessions.md)                    |
| 知道 agent 什么时候在等我                      | [需要你介入的提醒](usage/attention.md)               |
| 用终端：回放、多设备控制、查找、分屏、草稿     | [终端使用](usage/terminal.md)                        |
| 在终端旁看文件、审 Diff、把行号发给 agent      | [代码工作区](usage/code.md)                          |
| 管理仓库、新建或删除 worktree、释放空间        | [仓库与 worktree](usage/worktrees.md)                |
| 在本机终端里 `run` / `attach`，或在 Windows 用 | [从本机终端使用](usage/command-line.md)              |
| 让脚本或 agent 控制 WebMux                     | [用脚本和 agent 控制 WebMux](usage/shell-control.md) |
| 在手机上输入、用软键栏和手势                   | [移动端使用](usage/mobile.md)                        |
| 预览服务器上的开发服务器                       | [端口转发预览](usage/port-forwarding.md)             |
| 让会话行显示 shell 当前目录                    | [Shell 目录报告](usage/shell-directory.md)           |
| 接入某个 CLI agent                             | [CLI agent 接入](cli-agents.md)                      |

## 公网访问设置

从“设置 → 公网访问”进入 `/settings/tunnel`，选择 Quick Tunnel 或填写 Named Tunnel 的凭据与公共主机名，保存后启用。配置与开关保存在服务端，跨设备与服务重启保留；无需为了改隧道配置重启或结束终端会话。

凭据只写不读，留空保留已配置凭据，切换 Quick 会删除 Named 凭据。开启前须确认公开 shell 的风险，WebMux 登录 Token 至少 43 字符；它与 Cloudflare 凭据不同。公网来源上关闭或修改配置会断开当前页面，请保留本机/VPN 入口。完整步骤与故障恢复见 [配置指南](cloudflare-tunnel.md)。
