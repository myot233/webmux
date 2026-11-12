# Tasks

> 开始前先读 `webmux-ui` 技能与 `docs/design-system.md`、`docs/frontend-style.md`；所有新增 UI 遵循其设计系统与前端守卫。

## 1. 路由与入口

- [ ] 1.1 在 `web/src/hooks/shared/route.ts` 新增 `settings-remote-access` 路由（`/settings/remote-access`），补充 `route` 单元测试：解析、不影响 `/settings/tunnel` 与 `/settings/tailscale`。用 `pnpm test`（Vitest 单元）验证。
- [ ] 1.2 在 `App.svelte` 注册页面，并确认页面需要认证、与现有设置页一样走设置导航。
- [ ] 1.3 在 `Settings.svelte` 的 `TunnelSettings` 之前加入向导入口卡片；在 `AccessSummaryCard` 增加可选的“用向导配置”次要链接，两张现有卡片传入。更新对应组件测试。

## 2. 状态汇总与推荐

- [ ] 2.1 新增 hook `useRemoteAccessWizard`（`web/src/hooks/`），聚合 `GET /config`、`GET /tunnel`、`GET /tailscale`，单个来源失败时只标记该来源失败。用 Vitest 单元测试覆盖三种来源的成功、失败、加载中组合。
- [ ] 2.2 实现推荐函数（纯函数）：已有 `ready` 入口 > Tailscale `running` 且 `httpsReady` > 已保存 Named 凭据 > Quick。表驱动测试覆盖规格里的每个场景，并断言永远不会推荐 Funnel。
- [ ] 2.3 实现步骤状态机（选择方式、完成配置、等待就绪、手机访问）：前进、后退、退出、已有可用入口时跳到第 4 步。测试非法跳转被拒绝。

## 3. 配置步骤（复用现有组件）

- [ ] 3.1 检查 `TunnelConfiguration.svelte` 与 `TailscaleConfiguration.svelte` 能否在向导里复用；不能则拆出“表单 + 保存结果回调”的展示部分，让详细页与向导共用，详细页行为不变。用现有详细页的组件测试回归。
- [ ] 3.2 第 2 步嵌入对应表单：
  - Tailscale Serve：模式固定为 `serve`；
  - Named：输入隧道 Token 与主机名，就地给出获取步骤；
  - Quick：无输入项。
- [ ] 3.3 前提不满足时直接说明并禁用保存，不发写请求：`canEnable = false`、登录 Token 不足 43 字符、`storageAvailable = false`、WSL NAT 等。组件测试断言无网络写调用。
- [ ] 3.4 公网入口开启前的风险确认（Named、Quick），未确认时开启按钮不可用；Tailscale Serve 显示“仅 tailnet”说明且不要求确认。
- [ ] 3.5 服务端 400/409 错误显示在对应字段旁并保留已填内容。

## 4. 等待就绪

- [ ] 4.1 实现第 3 步状态展示与轮询：`starting` 轮询、`ready` 自动前进、`failed` 显示原因并提供“返回修改”与“重试”，不自动重试。轮询间隔作为常量，测试中注入计时器，不依赖真实 sleep。
- [ ] 4.2 页面卸载时用 `AbortController` 取消轮询，测试断言离开后不再有读取请求。

## 5. 手机访问与二维码

- [ ] 5.1 选定二维码库并加入依赖（候选 `qrcode-generator`）。记录 `web/dist/embed.zip` 的体积增量，通过 `knip`、Oxlint 与依赖审计；增量不可接受则改为内置最小实现。
- [ ] 5.2 抽出共享的“带 Token 链接”函数，替换 `TunnelManager` 与 `TailscaleSettingsPage` 里各自的实现，行为不变，现有测试通过。
- [ ] 5.3 实现手机访问面板：
  - 公开地址与一键复制；
  - 点击“显示带登录的链接”后动态 `import()` 二维码库并渲染 SVG；
  - 60 秒或离开步骤后清空；
  - 触屏设备默认不渲染二维码并显示复制与 Web Share；
  - 复制失败时的手动复制输入框；
  - 空 Token 实例只显示公开地址。
- [ ] 5.4 单元与浏览器组件测试：
  - 扫码内容解码后等于 `<publicUrl>#token=<编码后的 Token>`；
  - Token 不出现在 DOM 文本、`aria-label`、`title`、`localStorage`、`sessionStorage`、`document.cookie` 中；
  - 60 秒（注入计时器）后元素被移除；
  - 页面源码与网络请求中没有外部二维码服务。
- [ ] 5.5 按入口显示说明：Quick 的“地址会变、不支持 Web Push”、Tailscale 的“手机需登录同一个 tailnet”、Named 的风险提示；稳定 HTTPS 入口显示前往 Web Push 设置的链接，Quick 不显示。

## 6. 可访问性与响应式

- [ ] 6.1 步骤指示暴露当前步骤，切换后焦点移到新步骤标题；状态变化用 `role="status"`/`role="alert"` 播报；二维码带不含 Token 的文字替代。
- [ ] 6.2 360 px 与桌面宽度布局：卡片纵向排列、无横向滚动、触摸目标达标。

## 7. 原型与端到端

- [ ] 7.1 新增 Storybook 原型 `Settings/Remote access wizard`，覆盖：推荐 Tailscale、仅 Quick 可用、已有可用入口、某来源读取失败、登录 Token 不足、等待就绪、`failed`、手机访问（桌面二维码与手机复制两种）、Quick 限制说明。用 `make` 中的 Storybook 检查验证。
- [ ] 7.2 新增 Playwright E2E（模拟 API，不使用真实终端会话，也不启动真实 cloudflared 或 Tailscale）：
  - Quick 全流程：选择、确认风险、开启、等待、显示二维码；
  - Tailscale Serve 全流程；
  - Named 凭据校验失败后修正；
  - 键盘走完全流程。
- [ ] 7.3 运行 `make test-fast` 与相关 Playwright 用例；合并前运行 `make test`。
