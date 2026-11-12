# WebMux 文档中心

这里是所有文档的入口，按“你想做什么”分组。每份文档都能从本页或 [AGENTS.md](../AGENTS.md) 沿链接找到，`make docs-check` 会拒绝无法到达的文档与失效链接（规则见 [文档治理](doc-governance.md)）。

**初次接触 WebMux？推荐按这三步上手：**

1. **一分钟看懂定位与功能**：先读首页 [README](../README.md) 或 [项目介绍](overview.md)，了解持久终端、混合会话（聊天 ↔ 终端）与代码工作区。
2. **安装并连通手机**：按 [安装与升级](deployment/install-and-upgrade.md) 启动服务，再用 [远程访问与 HTTPS](deployment/remote-access.md) 在手机上扫码或打开访问链接。
3. **开始第一次对话**：阅读 [使用指南](usage.md) 的上手步骤，用“快速开始”或“新建会话”启动你的第一个 CLI agent。

## 了解 WebMux

- [项目介绍](overview.md)：定位、设计理念、全部特性、混合会话概览与技术栈。
- [User Stories](user-stories.md)：每项功能的用户目的与验收要点，适合评审和查找“某个功能能做什么”。[连接与会话](user-stories/access-and-sessions.md)覆盖登录、启动和工作区。[草稿用户故事](user-stories/terminal/drafts.md)收录草稿、暂存及发送确认。
- [常见问题与获取帮助](faq.md)：常见疑问、排障入口与反馈渠道。

## 安装与部署

- [部署指南](deployment.md)：总索引。
  - [安装与升级](deployment/install-and-upgrade.md)：一键脚本、deb / rpm、手动下载与升级。
  - [自动更新](deployment/updates.md)：后台定时检查新版本、设置页一键更新与回退策略。
  - [配置](deployment/configuration.md)：命令行参数、配置项与状态目录。
  - [服务管理与平台差异](deployment/services.md)：systemd / launchd、agent 的 PATH。
  - [远程访问与 HTTPS](deployment/remote-access.md)：手机访问、访问链接、HTTPS。
  - [安装为应用与手机推送](deployment/pwa-and-push.md)：PWA 与 Web Push。
  - [Windows（WSL）部署](deployment/windows-wsl.md)：托盘、控制台客户端与故障诊断。
- [Cloudflare Named Tunnel 配置指南](cloudflare-tunnel.md)：固定的 HTTPS 公网入口。
- [Tailscale Serve / Funnel](tailscale.md)：自动证书的 tailnet / 公网入口。
- [安全策略](../SECURITY.md)：安全模型、支持的版本与漏洞报告。

## 日常使用

- [使用指南](usage.md)：上手步骤与“你想做什么”导航。
  - [聊天模式](usage/chat.md)：快速开始、侧栏聊天、归档恢复与释放空间、仓库初始化指令。
  - [会话与工作台](usage/sessions.md)：侧栏、会话列表与状态、新建会话、启动配置、设置与跨设备偏好。
  - [终端使用](usage/terminal.md)：回放与控制权、查找、分屏、复制粘贴、终端图形、文件上传与草稿模式。
  - [代码工作区](usage/code.md)：文件浏览、Git Diff、多格式预览、行号引用与独立窗口。
  - [从本机终端使用](usage/command-line.md)：`run` / `attach`、Windows 终端与后台启动。
  - [仓库与 worktree](usage/worktrees.md)：仓库登记、新建工作树与起点选择、释放空间、删除与重建、推送检查。
  - [移动端使用](usage/mobile.md)：软键栏、虚拟键盘、触摸手势、锁定尺寸与全屏。
  - [需要你介入的提醒](usage/attention.md)：agent 状态识别、徽章与声音提醒、`OSC 7501` 协议及旧版钩子。
  - [端口转发预览](usage/port-forwarding.md)：把服务器上的开发服务器经 WebMux 转发到浏览器，终端端口提示与兼容选项。
  - [Shell 目录报告](usage/shell-directory.md)：可选的 OSC 7 目录报告与 bash / zsh 配置片段。
  - [用脚本和 agent 控制 WebMux](usage/shell-control.md)：连接正确实例、会话元数据、父会话与标签、命令帮助。
- [混合会话](hybrid-sessions.md)：OpenCode / omp / Codex CLI / Claude Code / Antigravity CLI / Qoder / QoderCN / Kimi 在聊天视图与终端间切换。
  - [支持的 agent 与连接器](hybrid-sessions/agents.md)：已验证版本、启动命令兼容要求、会话识别与 codex-acp / claude-agent-acp 安装。
- [CLI agent 接入](cli-agents.md)：预置的十一款 CLI agent 启动配置与提醒。

## 开发与架构

- [参与贡献](../CONTRIBUTING.md)：开发环境、常用命令、测试、PR 与 OpenSpec 工作流。
- [架构设计](architecture.md)：会话模型、缓冲与回放、状态探测、分层规则 R0–R11。
- [API 与协议](protocol.md)：REST、WebSocket 帧与错误码（按主题分页）。
  - [交接预览](protocol/http-handoff.md)：整项历史选择、预算、revision 与 stale。
  - [只读历史 MCP](protocol/handoff-mcp.md)：目标 scope、分页恢复、可用性和生命周期。
  - [Codex 原生历史](protocol/handoff-native.md)：已核实的版本、角色、独立用户 turn 与 unknown 恢复。
  - [持久对话历史](protocol/http-conversation.md)：Activity 分页、长文本余文、完整性和 portable projection。
- [配置与持久化清单](persistence.md)：状态目录中每类文件的格式与治理。
- [OpenSpec 规格索引](specs.md)：按主题列出 `openspec/specs/` 下的全部 capability。
- 代码规范：[Go](go-style.md)、[前端](frontend-style.md)、[前端架构守卫](frontend-architecture.md)。
- 界面：[设计系统](design-system.md)、[Storybook 与防溢出测试](storybook.md)。

## 性能与质量

- [布局稳定性与 4× CPU 回归](performance.md)：性能测试的测量方式与阈值。
- [页面性能与连接稳定性验证](page-performance.md)：页面加载与重连的实测记录。
- [内嵌二进制体积](binary-size.md)：发布二进制的体积对比。
- [CI 耗时优化与验收](ci-performance.md)：CI 墙钟时间的优化记录。
- [产品能力 baseline](../baseline/README.md)：可复现评测、统计口径与量化结果。

## 官网与推广

- [官网建设指南](website.md)：定位、静态站点生成器选型、页面清单、视觉规范、交互与 SEO 规则、站点功能与验收清单。

## 文档与 Agent 规则

- [文档治理](doc-governance.md)：文档规模、链接可达性与 README 预算。
- [Agent 规则](agent-rules.md)：从 `AGENTS.md` 迁出的仓库地图、约定、测试与 User Stories 维护规则。
