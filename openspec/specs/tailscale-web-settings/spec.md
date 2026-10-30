# tailscale-web-settings Specification

## Purpose

让部署者在网页设置页查看 Tailscale 检测与运行状态、了解 Serve/Funnel 的风险，并在网页中修改持久化的 Tailscale 设置；托管入口、对账与命令执行边界由 `tailscale-exposure` 规定。

## Requirements

### Requirement: 设置页状态与风险说明

设置页 SHALL 提供 Tailscale 卡片与 `/settings/tailscale` 详情页，展示检测状态、provider、节点 DNS 名、HTTPS 就绪、Funnel 可用性、配置模式、端口、公开 URL、运行状态与阻塞原因。

页面 SHALL 明确区分 `Serve`（仅 tailnet 内设备可访问）与 `Funnel`（公网 HTTPS，任何拿到链接与 Token 的人都能执行命令），并在 Funnel 模式下持续显示公网风险。

页面 SHALL 说明配置来源（配置文件或网页设置），并提供模式、端口与 provider 的编辑与“恢复配置文件设置”（见“网页修改 Tailscale 设置”）。

页面在各状态下的行为：

- 失败状态：SHALL 说明修复原因后重新保存即可再次应用。
- 刷新检测：只更新状态，不触发写命令。
- WSL NAT：SHALL 显示三种替代方案。
- Windows provider：SHALL 显示显式选择与 operator 权限提示。
- 入口冲突：SHALL 只显示 host、port、是否公开，不显示其他目标路径。

页面 SHALL 提供刷新检测与复制 URL。带 Token 链接复用现有剪贴板与 `localStorage` 约定，Token 不得进入页面可见文本或新的持久化位置。

#### Scenario: 检测到但未配置

- **WHEN** 设置页检测到运行中的 Tailscale，当前模式为 `off`
- **THEN** 页面显示节点 DNS、HTTPS 就绪，并可选择 Serve 或 Funnel 后保存；登录 Token 不足 43 字符时说明原因且服务端拒绝开启

#### Scenario: WSL NAT 指引

- **WHEN** 页面检测到 WSL NAT 与 Windows Tailscale
- **THEN** 页面说明不能托管，列出 mirrored（会结束会话）、WSL 内安装 Tailscale 与托盘转发三种方案，不提供自动修改 `.wslconfig` 的按钮

#### Scenario: Funnel 风险

- **WHEN** `mode = "funnel"` 且状态为 `ready`
- **THEN** 页面显著显示公网 HTTPS 风险、端口与公网 URL，不把 Funnel 描述成“仅 tailnet”

#### Scenario: 失败时终端仍可用

- **WHEN** Tailscale 检测或启动失败
- **THEN** 卡片/详情页显示原因，其他设置与终端功能继续工作，不阻塞页面其他请求

#### Scenario: 入口冲突

- **WHEN** 目标端口已有不属于本实例的入口
- **THEN** 页面显示该 host、port 与公开状态，说明不会覆盖，并给出人工处理提示；不显示其他应用的目标路径

### Requirement: 网页修改 Tailscale 设置

系统 SHALL 提供需要认证与 Host/Origin 检查的 `PUT /api/tailscale/settings`（`mode`、`port`、`provider`、可选 `confirmFunnel`）与 `DELETE /api/tailscale/settings`。

网页设置 SHALL 按与 TOML 相同的规则校验，以 0600、原子替换与独立 flock 保存在 `<state_dir>/tailscale-settings.json`，并在之后的启动中优先于 TOML 的 `mode`、`port`、`provider`；CLI 路径 `command` MUST 只来自 TOML。

设置文件损坏、不可读或锁冲突时，系统 MUST 保留原文件、拒绝网页写入，并把有效模式视为 `off`，不得回退到 TOML 开启入口。

保存成功后系统 SHALL 立即生效、无需重启：先撤销 Tailscale 来源，若记录中的入口不再符合新设置，先按所有权规则移除该精确目标，再应用新设置；移除失败时 MUST 保持 `failed`、保留记录且不应用新设置。

响应 SHALL 返回当前视图，应用中为 `starting`（202）。保存请求的校验：

- 从非 Funnel 切到 Funnel MUST 携带 `confirmFunnel: true`，否则返回 400 且不保存。
- `provider = "windows"` 仅在 WSL 中有效。
- 登录 Token 少于 43 字符或监听地址无法托管时 MUST 返回 409 且不保存。

恢复配置文件设置 SHALL 删除网页覆盖并关闭当前入口，本进程不按 TOML 自动开启，下次启动按 TOML 生效。

WebMux 会话内的进程 MUST NOT 通过本机 API 修改 Tailscale 设置。

#### Scenario: 网页切换端口

- **WHEN** Serve 在 8443 就绪，用户在网页把端口改为 10000 并保存
- **THEN** 系统先移除 8443 的本实例入口，再在 10000 应用并回读确认，期间状态为 `starting`，无需重启且会话不受影响

#### Scenario: 开启 Funnel 需要确认

- **WHEN** 用户在网页选择 Funnel 并保存
- **THEN** 页面先显示公网风险确认；未确认的请求返回 400 且不保存，确认后才保存并公开

#### Scenario: 弱 Token 拒绝开启

- **WHEN** 登录 Token 少于 43 字符，用户尝试在网页开启 Serve 或 Funnel
- **THEN** 服务端返回 409 且不保存设置，页面显示需要设置强 Token 并重启的原因

#### Scenario: 恢复配置文件设置

- **WHEN** 用户确认恢复配置文件设置
- **THEN** 系统删除网页设置并关闭本实例入口；下次启动按 TOML 的 `[tailscale]` 决定是否创建入口
