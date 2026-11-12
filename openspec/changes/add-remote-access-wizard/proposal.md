# Proposal

## Why

WebMux 想让用户“在手机上接管服务器上的 agent”，但目前把手机连上来要自己拼几步：

- 设置页里有两张互相独立的卡片：“公网访问（Cloudflare Tunnel）”和“Tailscale（Serve / Funnel）”，各自带一个管理页。用户得先自己判断该选哪一个。
- Named Tunnel 要先在 Cloudflare 控制台创建隧道、复制 Token、填主机名（见 `docs/cloudflare-tunnel.md`）；Tailscale 要先确认 HTTPS 证书、端口和 provider。设置页只呈现状态和表单，不告诉用户“下一步该做什么”。
- 配好之后，手机上还要手动把地址和 Token 拼成链接再传过去。现在只有“复制带 Token 链接”按钮，没有二维码，手机扫不了。
- Quick Tunnel 零配置，但每次启动地址会变，且不支持 Web Push。用户往往配完才发现收不到锁屏推送。

结果是：功能都在，但“从零到手机上打开”这条路径没有被设计成一条路径。这个变更只在 Web 端补上一个引导：按用户的情况推荐方式，复用现有配置能力完成设置，最后给出手机可直接扫码打开的链接。

## What Changes

- 新增 Web 端“远程访问向导”页面 `/settings/remote-access`，设置页新增入口卡片，两张现有访问卡片也提供“用向导配置”的入口。
- 向导分四步，每一步都能返回上一步，也可以随时退出到现有的详细设置页：
  1. **选择方式**：读取 `GET /api/tailscale`、`GET /api/tunnel` 和 `GET /api/config`，根据检测结果给出推荐，并列出三种方式的差异（地址是否固定、谁能访问、支持 Web Push、需要什么前提）。
  2. **完成配置**：嵌入现有的 Tailscale 与 Tunnel 配置表单，不新增服务端配置接口；对不满足的前提（Token 不足 43 字符、未检测到 Tailscale、WSL NAT 等）直接说明原因和下一步。
  3. **等待就绪**：轮询状态，显示 `starting`、`ready`、`failed` 与原因，失败时可返回上一步修改。
  4. **手机访问**：显示手机访问链接、二维码、复制按钮；稳定的 HTTPS 入口额外指向 Web Push 设置，Quick Tunnel 明确提示“地址会变、不支持推送”。
- 带 Token 的链接和二维码默认隐藏，用户点击后才显示，一段时间后自动收起；Token 仍只通过 `#token=` 片段携带，不进入页面可见文本、日志或任何新的持久化位置。
- 新增二维码生成能力。采用体积很小、无依赖的纯前端库，按需动态加载，不进入首屏包。
- 新增 Storybook 原型 `Settings/Remote access wizard`，覆盖各步骤与主要状态。

**不在范围内：** CLI 命令（`webmux remote` 等）、官方托管中继、新的服务端接口或持久化字段、Windows 托盘界面、修改登录 Token 的能力。

## Capabilities

### New Capabilities

- `remote-access-wizard`：向导的入口、方式推荐、分步流程、嵌入配置、就绪等待、手机访问链接与二维码、Token 展示边界。

### Modified Capabilities

无。向导只组合 `runtime-tunnel-control`、`tailscale-web-settings` 与 `access-link` 已有的行为和接口，不改变它们的要求。

## Impact

- 前端：
  - 新增页面 `RemoteAccessWizardPage` 和组件（步骤导航、方式卡片、手机访问面板），新增 hook 汇总三个状态来源。
  - `web/src/hooks/shared/route.ts` 新增 `settings-remote-access` 路由，`Settings.svelte` 与 `AccessSummaryCard.svelte` 增加入口。
  - 抽出 `TunnelManager`、`TailscaleSettingsPage` 中的“复制带 Token 链接”逻辑为共享函数，避免三处各拼一份。
  - 新增依赖：一个二维码生成库（动态加载）。需要确认它在 `web/dist/embed.zip` 中的增量，以及 `knip`、Oxlint 与依赖审计通过。
- 后端：无改动，不新增接口，不改 `internal/contract`、`internal/wire` 或 wire 快照。
- 测试：新增 Vitest 单元与浏览器组件测试、Storybook 页面检查、Playwright E2E（用模拟 API 走完三条路径）。
- 文档：按 `openspec/config.yaml` 的约定，不为这个变更单独安排文档任务。
