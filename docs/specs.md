# OpenSpec 规格索引

`openspec/specs/` 下每个目录是一个 capability，`spec.md` 用 SHALL/MUST 描述它的现行需求。本页按主题列出全部 capability，让读者从 [文档中心](README.md) 沿链接找到每一份规格。

- 这里只收录已归档生效的主规格；在途变更位于 `openspec/changes/<变更>/`，由 OpenSpec 工作流管理，归档后并入对应 capability。
- 新增、拆分或改名 capability 时，在同一变更中更新本页；`internal/docguard` 会把没有链接到的规格报告为不可达。
- 规格描述“应当怎样”；功能用途与验收要点见 [User Stories](user-stories.md)，工作流见 [参与贡献](../CONTRIBUTING.md#openspec-工作流可选)。

## 会话与终端

- [terminal-control](../openspec/specs/terminal-control/spec.md)：多设备同时打开同一会话时的唯一控制端与接管
- [terminal-stream-resume](../openspec/specs/terminal-stream-resume/spec.md)：按输出 offset 与运行代际增量续接终端输出
- [terminal-buffer-find](../openspec/specs/terminal-buffer-find/spec.md)：在保留的终端输出中查找、跳转与复制
- [terminal-header](../openspec/specs/terminal-header/spec.md)：终端页顶栏在桌面与手机上的信息与操作收纳
- [terminal-input-composer](../openspec/specs/terminal-input-composer/spec.md)：统一的草稿模式输入入口
- [terminal-draft-persistence](../openspec/specs/terminal-draft-persistence/spec.md)：草稿在刷新、关页与认证失效后的本机恢复
- [terminal-draft-scratch](../openspec/specs/terminal-draft-scratch/spec.md)：每个会话的草稿暂存区
- [terminal-submission-delivery](../openspec/specs/terminal-submission-delivery/spec.md)：草稿与快捷指令可确认、不重复的送达语义
- [terminal-quick-commands](../openspec/specs/terminal-quick-commands/spec.md)：在设置中保存并一键发送常用命令
- [terminal-session-cli](../openspec/specs/terminal-session-cli/spec.md)：`webmux run|attach|ls|open` 本机终端附着
- [session-launch-environment](../openspec/specs/session-launch-environment/spec.md)：会话启动时的进程环境约定
- [launch-configs](../openspec/specs/launch-configs/spec.md)：服务器持久化的启动配置（名称 + argv + 环境变量）
- [session-artifacts](../openspec/specs/session-artifacts/spec.md)：会话产物目录
- [terminal-keyboard-size-hold](../openspec/specs/terminal-keyboard-size-hold/spec.md)：触屏唤起软键盘时暂时保持 PTY 行列不变
- [terminal-touch-gamepad](../openspec/specs/terminal-touch-gamepad/spec.md)：触屏与调试可用的半透明浮动摇杆、高频虚拟键及底栏去重
- [terminal-notice-overlay](../openspec/specs/terminal-notice-overlay/spec.md)：终端顶部状态提示悬浮显示，不触发 resize
- [websocket-compression](../openspec/specs/websocket-compression/spec.md)：手机与平板默认启用、可按设备覆盖的 WebSocket 压缩

## 应用外壳与导航

- [app-shell](../openspec/specs/app-shell/spec.md)：首页、快速开始页与会话页共用的侧栏布局与模式切换
- [chat-sidebar](../openspec/specs/chat-sidebar/spec.md)：聊天模式侧栏中的聊天历史
- [workbench-navigation](../openspec/specs/workbench-navigation/spec.md)：工作台侧栏的“仓库 → worktree → 会话”导航树
- [quick-start-chat](../openspec/specs/quick-start-chat/spec.md)：一次发送即在新 worktree 中启动聊天会话
- [new-session-dialog](../openspec/specs/new-session-dialog/spec.md)：汇总式单页的新建会话对话框
- [workspace-return-cache](../openspec/specs/workspace-return-cache/spec.md)：返回首页时先显示缓存列表再后台刷新

## Shell 工具（`webmux` 命令）

- [agent-shell-guide](../openspec/specs/agent-shell-guide/spec.md)：按 llms.txt 约定提供 shell 工具手册与 `{{webmux}}` 说明
- [shell-cli-help](../openspec/specs/shell-cli-help/spec.md)：完整、可机器读取的命令帮助与 shell 补全
- [shell-session-launch](../openspec/specs/shell-session-launch/spec.md)：从 shell 启动会话与 `chat new`
- [shell-session-control](../openspec/specs/shell-session-control/spec.md)：从 shell 控制已有会话的输入、等待、输出与事件
- [shell-chat-control](../openspec/specs/shell-chat-control/spec.md)：从 shell 驱动混合会话的聊天端
- [shell-workspace-control](../openspec/specs/shell-workspace-control/spec.md)：从 shell 管理 worktree、仓库、启动配置与端口转发
- [session-metadata](../openspec/specs/session-metadata/spec.md)：会话完整背景、创建者与标签

## Agent 状态与提醒

- [agent-attention](../openspec/specs/agent-attention/spec.md)：依据 BEL、OSC 9/777 等信号判断 agent 需要介入
- [program-status](../openspec/specs/program-status/spec.md)：识别 OSC 7501 Program Status Protocol 上报的运行状态
- [attention-push](../openspec/specs/attention-push/spec.md)：离开页面或锁屏后通过 Web Push 推送注意力事件
- [push-notification-client](../openspec/specs/push-notification-client/spec.md)：在本设备开启、关闭和测试推送

## 混合会话（ACP）

- [hybrid-sessions](../openspec/specs/hybrid-sessions/spec.md)：同一段对话在聊天视图与原生 TUI 之间切换
- [hybrid-agent-profiles](../openspec/specs/hybrid-agent-profiles/spec.md)：内置 agent 档案，负责识别、启动、恢复与中断
- [hybrid-agent-specifics](../openspec/specs/hybrid-agent-specifics/spec.md)：Claude Code、Codex、Antigravity CLI 各自的环境、配置推导、命令与进程边界
- [hybrid-session-recovery](../openspec/specs/hybrid-session-recovery/spec.md)：服务重启后恢复混合会话
- [agent-session-identity](../openspec/specs/agent-session-identity/spec.md)：识别 TUI 端当前打开的 agent 会话
- [acp-agent-connection](../openspec/specs/acp-agent-connection/spec.md)：作为 ACP 客户端与 agent 子进程交互的协议边界
- [acp-connector-availability](../openspec/specs/acp-connector-availability/spec.md)：ACP 连接器（如 codex-acp）的可用性与安装提示
- [chat-view](../openspec/specs/chat-view/spec.md)：ACP 输出的对话界面
- [chat-prompt-queue](../openspec/specs/chat-prompt-queue/spec.md)：回合进行中的多设备共享消息队列

## 代码工作区

- [terminal-diff-review](../openspec/specs/terminal-diff-review/spec.md)：在终端旁浏览工作树、完整文件与 Diff
- [terminal-code-references](../openspec/specs/terminal-code-references/spec.md)：把文件、行或 Diff 片段引用插入终端
- [code-image-preview](../openspec/specs/code-image-preview/spec.md)：只读查看与对比图片
- [code-markdown-preview](../openspec/specs/code-markdown-preview/spec.md)：Markdown 预览、目录与文档内跳转
- [code-document-preview](../openspec/specs/code-document-preview/spec.md)：文档类文件预览
- [code-popout-window](../openspec/specs/code-popout-window/spec.md)：代码工作区弹出为独立窗口
- [code-tree-drop-upload](../openspec/specs/code-tree-drop-upload/spec.md)：拖放文件到资源管理器上传

## 仓库与工作树

- [repository-registry](../openspec/specs/repository-registry/spec.md)：独立于会话的持久仓库清单
- [workspace-overview](../openspec/specs/workspace-overview/spec.md)：按仓库分组的会话总览
- [workspace-sessions](../openspec/specs/workspace-sessions/spec.md)：会话与仓库、工作树的关联
- [worktree-lifecycle](../openspec/specs/worktree-lifecycle/spec.md)：发现、创建与清理 Git worktree
- [worktree-batch-management](../openspec/specs/worktree-batch-management/spec.md)：单仓库工作树全量清单、状态筛选与批量清理
- [repository-details](../openspec/specs/repository-details/spec.md)：仓库面板分区、工作树上游同步详情、分支与远程清单及凭据脱敏
- [repository-maintenance](../openspec/specs/repository-maintenance/spec.md)：获取全部远程与清理目录已不存在的失效工作树登记
- [worktree-branch-switch](../openspec/specs/worktree-branch-switch/spec.md)：已有工作树原地切换分支的目标、阻止条件、会话确认与“在新工作树打开”
- [branch-operations](../openspec/specs/branch-operations/spec.md)：快进到上游、推送与发布、删除与批量清理本地分支

## 配置、状态与同步

- [cli-config](../openspec/specs/cli-config/spec.md)：客户端定位、检查与初始化配置
- [effective-configuration](../openspec/specs/effective-configuration/spec.md)：区分已配置值与运行中生效值
- [server-settings-sync](../openspec/specs/server-settings-sync/spec.md)：主题、预览等偏好的跨设备同步
- [state-persistence](../openspec/specs/state-persistence/spec.md)：状态目录文件的统一存储、权限与有界解码
- [persistence-governance](../openspec/specs/persistence-governance/spec.md)：持久化清单、守卫与故障测试

## 服务运行与分发

- [cli-server-control](../openspec/specs/cli-server-control/spec.md)：`webmux start|stop|status` 等服务控制命令
- [server-local-api](../openspec/specs/server-local-api/spec.md)：同一用户本机客户端使用的本地 API
- [server-runtime-record](../openspec/specs/server-runtime-record/spec.md)：可校验的本机运行时记录
- [access-link](../openspec/specs/access-link/spec.md)：携带 Token 的访问链接
- [release-distribution](../openspec/specs/release-distribution/spec.md)：一次标签发布产出全部平台二进制与校验和
- [linux-packages](../openspec/specs/linux-packages/spec.md)：deb 与 rpm 安装包
- [signed-release-updates](../openspec/specs/signed-release-updates/spec.md)：发布签名、最新版本发现与只升级不降级的共用更新规则
- [linux-package-auto-update](../openspec/specs/linux-package-auto-update/spec.md)：deb / rpm 安装的系统级定时自动更新
- [server-update-notice](../openspec/specs/server-update-notice/spec.md)：可执行文件被替换后提示“重启后生效”

## 远程访问

- [tunnel-exposure](../openspec/specs/tunnel-exposure/spec.md)：通过 Cloudflare Tunnel 以 HTTPS 暴露到公网
- [runtime-tunnel-control](../openspec/specs/runtime-tunnel-control/spec.md)：运行期间开启或关闭公网隧道
- [web-tunnel-settings](../openspec/specs/web-tunnel-settings/spec.md)：网页设置中的 Cloudflare Tunnel 管理
- [tailscale-exposure](../openspec/specs/tailscale-exposure/spec.md)：Tailscale Serve / Funnel 自动证书 HTTPS
- [tailscale-web-settings](../openspec/specs/tailscale-web-settings/spec.md)：网页设置中的 Tailscale 状态与开关

## Windows（WSL）

- [windows-console-client](../openspec/specs/windows-console-client/spec.md)：PowerShell / Windows Terminal 控制台客户端
- [windows-installer](../openspec/specs/windows-installer/spec.md)：按用户安装的托盘安装程序
- [windows-tray-host](../openspec/specs/windows-tray-host/spec.md)：常驻通知区域的托盘程序
- [windows-tray-access](../openspec/specs/windows-tray-access/spec.md)：从托盘打开或复制访问链接
- [windows-tray-forwarding](../openspec/specs/windows-tray-forwarding/spec.md)：WSL 网络不可达时的托盘端口转发
- [windows-tray-push-guidance](../openspec/specs/windows-tray-push-guidance/spec.md)：托盘中的手机推送前置条件提示
- [windows-tray-server-compat](../openspec/specs/windows-tray-server-compat/spec.md)：托盘与 WSL 中 webmux 的版本兼容判断
- [windows-tray-tunnel](../openspec/specs/windows-tray-tunnel/spec.md)：托盘一键开关公网隧道
- [windows-tray-wsl-install](../openspec/specs/windows-tray-wsl-install/spec.md)：由托盘在 WSL 中安装同版本 webmux
- [windows-tray-self-update](../openspec/specs/windows-tray-self-update/spec.md)：安装版托盘下载签名安装程序并静默更新自身
- [wsl-service-lifecycle](../openspec/specs/wsl-service-lifecycle/spec.md)：托盘在 WSL 中启动、监控与重启服务

## 工程架构

- [controller-service-layering](../openspec/specs/controller-service-layering/spec.md)：后端 Controller 与 Service 分层
- [frontend-architecture-guard](../openspec/specs/frontend-architecture-guard/spec.md)：前端架构检查入口
