# WebMux User Stories

本文从用户目的出发，描述 WebMux 当前各项功能用来做什么，并给出可观察的验收要点。适用于理解产品、评审变更和检查功能覆盖。

基线核对日期：2026-10-23（之后各次局部核对见下文）。范围为主分支 `5195154` 的实现，另含 US-N07–US-N08 所述的设置页公网访问说明与运行时隧道状态；OpenSpec 中只有规划、没有实现的内容不视为现有能力。验收要点描述行为契约，不代表本次已重新执行全部测试。

本次局部核对（2026-10-23）：PR #98 的暂存 UI 调整对应 US-F03、US-F09；#99 的设置页隧道运行时状态对应 US-N08；PR #103 的启动配置对应 US-B01–US-B03、US-B08；未重新核对其余功能组。

本次局部核对（2026-10-24）：#108 的代码工作区图片预览与下载对应新增 US-I15，并更新 US-I11 的受限说明；`compact-terminal-chrome` 的紧凑顶栏与代码区控件对应新增 US-H08，并更新 US-D03、US-D07、US-D08、US-I01、US-I08、US-I09、US-I13、US-I15；未重新核对其余功能组。

本次局部核对（2026-10-24）：`enforce-controller-service-layering` 的分层架构测试与永久线上快照对应新增 US-L05；后端重构不改变其余故事描述的行为，未重新核对其余功能组。

本次局部核对（2026-10-25）：`frontend-architecture-guard` 的前端架构守卫对应新增 US-L06；合并前补齐重复代码基线的收紧检查，实际克隆烟测确认新增重复被拒绝且不写入基线、已解决指纹可删除，`check:arch` 通过；未重新核对其余功能组。

本次局部核对（2026-10-25）：#117 后续加固更新 US-L05（R5/R7/R9、独立快照及字段级归一化）、US-L04（真实 PTY/WebSocket 分配基准）及 US-D02（非法内部消息只断开连接）；已运行 Go race 全套、六组独立快照、生产协议烟测和前后版本分配基准；未重新核对其余功能组，未进行本次前端浏览器或真机验收。

本次局部核对（2026-10-25）：移除内置启动预设，统一使用保存的自定义配置或显式临时命令，对应 US-B01–US-B03、US-B08、US-K04；原内置预设行为由自定义启动配置替代，故事 ID 保持不变。Open Code 的显式 Bash 命令保留 `.bashrc` 环境与 argv 边界；不代表已重新核对其余功能组。

本次局部核对（2026-10-25）：新建会话以 Tab 组织启动位置和命令来源，保存的启动配置使用同时展示名称与命令的下拉选择器，对应 US-B01–US-B03、US-B06；不改变启动配置存储和会话创建协议，未重新核对其余功能组。

本次局部核对（2026-10-25）：新安装首次创建 `launch-configs.json` 时写入 Shell、Claude Code、Codex CLI、Gemini CLI、Open Code、omp 预设，对应 US-B01、US-B08、US-K04；已运行 `internal/launch` 单元测试与真实二进制空状态目录烟测，未重新核对其余功能组。

本次局部核对（2026-10-25）：Pullfrog workflow 显式选择 OpenCode Go 的 DeepSeek V4.1 Flash，并固定 `effort: high` 推理等级，对应 US-L07；本次仅核对配置与密钥名称，不代表已发起远程模型调用，未重新核对其余功能组。

本次局部核对（2026-10-25）：PR #124/#125 的本地实现对应新增 US-D11，更新 US-D03、US-M08、US-N01–US-N08、US-O09、US-O11 及覆盖索引。

- 已执行：全量 Go race、前端单元、定向浏览器及桌面/Pixel 7 E2E；真实隔离浏览器完成查找、只写凭据、键盘操作与亮暗/宽窄布局验收。
- 范围限制：隧道使用 fake cloudflared，不连接真实 Cloudflare；未做真机或原生 Windows 托盘交互检查。
- 一致性：133 个故事 ID 唯一，索引引用与 143 条来源路径有效；不代表重新核对其余功能组。

本次局部核对（2026-10-25）：代码阅读的语法高亮改为按文件名与扩展名从 CodeMirror language-data 按需加载，并新增 US-I16 Markdown 预览（源码/预览切换、DOMPurify 清洗、图片与外部资源不加载、相对链接不导航）；已运行 svelte-check、前端单元与浏览器测试、`check:arch` 和生产构建；未重新核对其余功能组。

本次局部核对（2026-10-26）：`add-program-status-signals` 的 `OSC 7501` 程序状态识别、服务端探测应答、信号来源分层与状态合成对应更新 US-C05、US-C06、US-K04。

- 已运行：Go race 全套与 `attention.spec.ts` 桌面/Pixel 7 E2E。
- 隔离环境中以真实 Claude Code 2.1.310、Codex CLI 0.164.0、omp 18.12.6 抓取 PTY 字节，确认探测与上报格式（见变更目录 `baseline.md`）。
- 限制：未进行真机验收，未重新核对其余功能组。

本次局部核对（2026-10-25）：#127 的截断回放修复对应 US-D03、US-D04、US-D07、US-D09。

- 通过：定向 Go race、14 个终端回放浏览器用例、9 个连接生命周期用例、类型检查、lint、前端架构守卫和生产构建。
- 隔离实例使用 4 KiB 缓冲与真实 opencode v2.1.4，验证重开页面、观察端附着及菜单重新同步，确认仅初始化模式并重新绘制。
- 已核对亮暗与宽窄布局的提示。
- 限制：不代表完整 VT 状态恢复、真机验收或其余故事重新验证。

本次局部核对（2026-10-25）：#130 / #128 的 Kitty 图形协议对应新增 US-D12，并更新 US-B08 的 omp 首次预置环境。

- 审查修复：解码/画布/放置预算、异步解码失效、备用屏幕隔离、清屏和完整回放顺序。
- 通过：类型、lint、前端架构守卫、协议与全量前端测试、生产构建及 Go race 全套。
- 隔离真实后端：官方 kitten icat 0.49.2 与真实交互式 omp 18.12.6 显示样例 PNG。
- 默认 omp 环境没有 Kitty 输出；预置配置注入 `PI_FORCE_IMAGE_PROTOCOL=kitty` 后发出 PNG 传输与放置，浏览器像素逐色匹配。
- 限制：使用只含图片工具结果的恢复会话，无模型调用；不是图片生成或 provider 验证，未做真机或其余功能组重新验收。

追加局部核对（2026-10-26）：#130 并发追加的性能改动复核对应 US-D12。

- 复现并修复：Base64 空白导致像素长度错误、跨解码块的中间填充被误接受，以及完整同步后旧异步查询迟到答复。
- 通过：原生解码与 `atob` 回退烟测，268 个单元与 228 个浏览器用例、Go race 全套、架构/格式/生产构建及 45 项 OpenSpec 严格校验。
- `negotiate-terminal-graphics-capabilities` 仅规划启动能力协商，19 项实施任务均未勾选，不作为已实现能力。

追加局部核对（2026-10-26）：Android 系统返回键收起键盘后，轻点或滑动终端偶发重新弹出键盘的修复对应 US-H02。

- 修复方式：
  - 视口高度用于区分键盘开合与浏览器栏/横竖屏变化。
  - 触摸或笔手势在确认键盘已收起时清除残留输入焦点，并抑制触摸派生的兼容 `mousedown` 重新聚焦。
  - 键盘按钮在检测到真实收起后可重新打开。
- 通过：268 个单元用例、231 个浏览器用例、211 个 Storybook 用例、桌面与 Pixel 7 软键 E2E、svelte-check、Oxlint、前端架构守卫和格式检查。
- 限制：未进行真机 Android IME 验收，未重新核对其余功能组。

追加局部核对（2026-10-26）：#139（issue #133）的启动配置级输出预览对应 US-B08、US-C02、US-C07。

- 启动配置新增可选的 `preview: {lines, skip}`（`launch-configs.json` 版本 2）。当时兼容读取版本 1，此兼容行为已被 #146 的 v2-only 破坏性升级取代。
- 会话信息新增可选 `configId`，首页按配置解析预览与卡片高度，全局设置保留为兜底。
- 通过：`internal/launch`、`internal/workspace`、`internal/wire`、`internal/session` 与现有的 API 快照测试，269 个前端单元与 231 个浏览器用例、类型检查、lint、前端架构守卫和生产构建。
- 桌面与 Pixel 7 模拟的定向 E2E 使用隔离后端验证配置启动、`configId` 回传和 5 行/跳过 2 行的卡片预览。
- 限制：未做真机验收，未重新核对其余功能组。

本次局部核对（2026-10-25）：#126 的按需文件树对应 US-I03，保留 US-I01 的可见根路径。

- 合并审查补齐：当前文件高亮、搜索与折叠竞态、导航状态保留、分页目录定位和陈旧检查结果处理。
- 通过：19 个定向浏览器用例、类型检查、lint、前端架构守卫与生产构建；真实 Storybook 组件核对亮暗、桌面与手机布局及高亮，并更新截图。
- 限制：未进行真机验收或重新核对其余故事；#120 只规划设置同步，不视为已实现能力。

本次局部核对（2026-10-25 至 2026-10-26）：#131 / #129 的首次启动自动生成配置对应 US-A03、US-K02、US-K03、US-K05、US-N03。

- 合并审查修正：隧道模式仍会把持久化 Token 放入启动链接和日志的安全说明。
- 通过：真实生产前端构建后 Go race 全套及相关 go vet。
- 隔离真实 CLI/HTTP 烟测确认：首次 0600/0700 配置、持久 Token 认证与状态查询、重启不改文件、临时参数不写回、显式缺失文件不创建默认配置。
- 限制：未做真机验收或重新核对其余功能组。

本次局部核对（2026-10-26）：启动配置图标对应 US-B01、US-B08。

- 行为：图标随配置持久保存，默认 agent 使用本地打包的 LobeHub SVG，旧配置按命令自动匹配且不改写文件。
- 通过：46 个定向单元、33 个浏览器用例、4 个桌面/Pixel 7 E2E、`internal/launch` / `internal/api` / `internal/wire` Go race、相关 go vet、类型、lint、架构守卫、全仓格式检查与生产构建；真实隔离实例核对浅色/深色默认图标列表及浅色图标编辑器。
- 一致性：136 个故事 ID 唯一、索引引用与来源路径有效；未重新核对其余功能组或进行真机验收。

本次局部核对（2026-10-26）：#132 的会话列表终端链接改为在新标签页打开，对应新增 US-C11。主屏幕应用没有浏览器标签页且不共享本机 Token，仍使用应用内导航。

- 通过：定向浏览器用例、桌面新标签页与受影响的续接/草稿/工作树/软键 E2E、两项 4x CPU 性能用例、`svelte-check`、lint、格式检查与前端架构守卫。
- 限制：未做真机主屏幕应用验收，未重新核对其余功能组。

本次局部核对（2026-10-26）：#134 将终端字体下限从 12px 降到 8px，对应 US-D08、US-H04；同步更新三处钳制点、字体滑块测试与设置同步规划中的范围。定向终端渲染浏览器用例（15 项）、移动端软键 E2E、类型检查、lint 与格式检查通过；未重新核对其余功能组。

追加局部核对（2026-10-26）：仅处理 #138、#133、#132，对应 US-D12、US-B08、US-C02、US-C07、US-C11。Kitty 图片不再被局部文本擦除或 xterm 文本 marker 的销毁误删；共享字节的真实 PTY 程序验证顶部裁剪与文字重绘，浏览器覆盖页边距内滚动、历史显示/截断、尺寸变化与 reset。

已启动会话的预览在重新加载配置后应用修改、清空与删除后的回退；新标签页操作时原列表持续接收输出和退出状态，主屏幕模式保留 Token。

278 个单元、244 个浏览器、211 个 Storybook 用例及 Linux E2E 170 项通过（20 项按现有平台条件跳过），Go race 全套、类型、lint、架构与生产构建通过。137 个故事 ID 唯一，索引引用和来源路径有效。

截图来自隔离实例；未运行 macOS 原生套件或 iOS/Android 真机验收，未重新核对其余功能组。

本次局部核对（2026-10-26）：#141 的文档与 spec 治理对应新增 US-L08；新增 `internal/docguard` 守卫与 `docs/doc-governance.md`，拆分 `docs/protocol.md` 和 `docs/user-stories.md`。`go test ./internal/docguard`、`go test ./internal/archtest` 与 gofmt/Oxfmt 格式检查通过；未重新核对其余功能组。

本次局部核对（2026-10-26）：#144 的 Tailscale Serve/Funnel 托管对应新增 US-P01–US-P06，并更新 US-K10 的设置分区入口。

- 通过：fake CLI 单元、`internal/tailscale`/`internal/config`/`internal/api`/`internal/wire`/`cmd/webmux` Go race、`internal/archtest`、`go vet`、新增 Tailscale 浏览器用例、`svelte-check`、Oxlint、前端架构守卫、生产构建、格式检查与 OpenSpec 严格校验。
- 尚未验收：真实 Tailscale Serve/Funnel、WSL Windows mirrored 主机、真机与亮暗/桌面/手机截图；CI 不连接真实 Tailscale。

本次局部核对（2026-10-26）：Tailscale 设置改为网页可编辑并立即生效，对应新增 US-P07，并更新 US-P06 的过期记录与状态文件恢复要点。

Go race 全套、`go vet`、架构与文档守卫、278 个前端单元与 254 个浏览器用例、`svelte-check`、Oxlint、前端架构守卫、格式检查、Cloudflare 隧道设置 E2E（共享确认框重构）与 OpenSpec 严格校验通过。

WSL2 mirrored + 真实 Windows Tailscale 上经网页与 API 验证：切换端口（8444→8445→自动 8443→8446）先移除旧入口再应用、关闭与恢复配置文件设置移除本实例入口、网页设置跨重启生效、未确认 Funnel 400、未知字段 400、异源 403、`auto` 不代管 Windows；用户自建的 443 入口始终未被修改。

未在真实环境开启公网 Funnel，未做 Linux/macOS 原生节点与真机验收。当时 145 个故事 ID 唯一，索引引用与来源路径有效。

本次局部核对（2026-10-26）：#148 的 Go 质量基线与 lint/CI 门禁对应 US-L02、US-L05。

- 同批变化：推送存储失败日志对应 US-K08；终端附着输入可取消并在返回前等待，对应 US-M03；服务定义预览输出失败对应 US-K06。
- 必填响应数组/映射统一为 `[]`/`{}`，nullable 字段保留 `null`。
- review bot 后续修正 Unix read 的 EINTR 重试，避免信号中断导致误分离，同属 US-M03。
- 限制：仅核对上述变化，未重新核对其余故事；验证记录见 PR。

本次局部核对（2026-10-26）：#149 的前端代码质量基线对应新增并扩展 US-L09。

- 规范入口：`docs/frontend-style.md`、`AGENTS.md`、`CONTRIBUTING.md`。
- 已落地：Oxlint 类型感知 lint、Svelte 推荐规则、严格索引、测试目录类型检查、请求体验证、Go 强类型枚举、wire 快照契约和五类失败路径回归；并补主题首屏、减少动效、标签键盘、路由焦点与图标提示。
- 限制：未重新核对其余功能组。

本次局部核对（2026-10-26）：#146 的持久化清单守卫对应新增 US-L10。完整 `internal/archtest` 与 `internal/docguard` race 测试、OpenSpec 严格校验及本次文件格式检查通过；146 个故事 ID 唯一，核对范围内 204 个本地链接目标存在。该记录描述最初 draft 阶段；后续完整实施核对见下。

本次追加实施（2026-10-26）：#146 与 #119 更新 US-C07、US-C09、US-D08、US-G01–US-G04、US-H04、US-I05、US-J03、US-K03、US-K08、US-K10、US-L10、US-P06。

- 已接线：统一 JSON 存储、磁盘/活进程配置诊断和完整偏好同步；旧浏览器偏好不迁移。
- 已通过：新增双客户端/重启/损坏文件 E2E 与配置只读脱敏测试。
- 一致性：147 个故事 ID 唯一，215 个本地链接目标存在；主分支已占用 US-L09，本分支未合入的持久化故事分配 US-L10。
- 限制：完整回归结果以 PR 验证记录为准，不代表重验全部历史故事。

本次局部核对（2026-10-27）：推送与站内关注文案统一并补充下一步提示，对应新增 US-C12 并更新 US-C06。

- 推送正文改为“短标签 · 下一步”，`done` 在列表与推送中统一为“本轮完成”；服务 worker 兜底文案由 Go 用例锁定同步。
- 通过：`internal/push`、`internal/api`、`internal/wire` Go race、`go vet`、Vitest Service Worker 与推送卡片用例、`attention.spec.ts` / `push.spec.ts` 桌面与 Pixel 7 E2E、`svelte-check`、Oxlint、前端架构守卫、格式检查与文档守卫。
- 一致性：148 个故事 ID 唯一；未重新核对其余功能组。

本次局部核对（2026-10-27）：#161 的 PWA 第一批对应新增 US-H09，并更新 US-D03、US-H06、US-K10、US-C12。

- 已实现：健康终端连接回到前台改为探测保留；安装应用分区、非安全上下文提示、manifest `id`/`lang` 与图标尺寸检查；推送授权只请求一次。
- 不支持：离线启动和子路径部署。
- 通过：Go `internal/api` 测试、Vitest 单元与浏览器全套、Playwright E2E 全套（本地桌面与 Pixel 7 Chromium）、`svelte-check`、Oxlint、前端架构守卫与格式检查；149 个故事 ID 唯一。
- 尚未验收：Android Chrome、桌面 Chrome/Edge、iOS Safari 真机安装入口。

本次局部核对（2026-10-27）：#162 的浏览器标签页运行指示对应新增 US-C13。标题与 favicon 由 `web/src/browser-chrome.ts` 统一管理，终端页与首页按 `status === "running"` 接线，静态页面清除残留动画；动画使用本地生成的约 2 fps 帧，`prefers-reduced-motion: reduce` 时图标保持静态运行帧、标题仍保留标记。

`svelte-check`（含 `check:test`）、Oxlint、前端架构守卫、定向单元与浏览器用例、`browser-chrome.spec.ts` / `attention.spec.ts` 桌面与 Pixel 7 E2E、`go test ./internal/docguard` 与格式检查通过；149 个故事 ID 唯一。

浏览器对动态 favicon 的支持不一致，标题标记为保底；未做真机验收，未重新核对其余功能组。

本次局部核对（2026-10-27）：#164 新增 `webmux start` 后台启动，对应新增 US-K12，并更新 K 组索引与覆盖索引。`go test ./cmd/webmux`、`go test ./internal/client ./internal/runtime`、`go vet ./cmd/webmux` 与文档守卫通过；152 个故事 ID 唯一；未重新核对其余功能组。

本次局部核对（2026-10-27）：#163 的一键安装脚本对应新增 US-K11。安装入口使用 Release 资产、jsDelivr 与 gh-proxy 镜像，不再依赖 `raw.githubusercontent.com`。

- 覆盖：`sh -n` 与本地 HTTP fixture 覆盖 Linux/macOS 架构映射、Rosetta、固定版本、校验失败保留旧文件、镜像回退和不支持平台；Release 配置附带 `install.sh`。
- 限制：未运行真实 macOS 安装或发布流程，未重新核对其余功能组；149 个故事 ID 唯一。

本次局部核对（2026-10-27）：#154 的通用 HTTPS 推送来源更新 US-C12、US-N08。推送来源从 Named Tunnel 与回环放宽为通过 Host/Origin 校验的 HTTPS 入口（含托管 Tailscale Serve/Funnel），HTTPS 锚点按 authority 规范化并保留非 443 端口；Quick Tunnel、非回环 HTTP 与本机套接字仍不支持。

`go test ./internal/api`、`internal/archtest`、`internal/docguard`、`internal/wire`、`internal/push` 与 `go vet ./...`、`svelte-check`（含 `check:test`）、Oxlint、PushSettings 浏览器用例、格式检查通过；149 个故事 ID 未变，未重新核对其余功能组。

本次局部核对（2026-10-28）：`add-acp-hybrid-sessions` 的混合会话对应新增 US-Q01–US-Q13，并更新 US-B01、US-B08、US-C02、US-D09、US-D10、US-M01、US-M03、US-M05、US-L10。

- 核对依据：实现与本变更新增的 Go race、Vitest 单元/浏览器与 Playwright 测试，以及 `baseline.md` 中对真实 OpenCode 2.1.4/2.2.0 与 omp 18.13.1 的 ACP、TUI 与参数实测。
- 限制：未进行真机验收，未重新核对其余功能组。165 个故事 ID 唯一。

本次局部核对（2026-10-28）：#181 的聊天视图权限模式（每次询问 / 自动批准，默认自动批准）更新 US-Q06，覆盖索引同步。核对依据为 `internal/hybrid` 与 `internal/api` 的 Go race 测试、线上快照与 wire 契约用例，以及 ChatView 浏览器用例；未用真实 agent 验收，未重新核对其余功能组。165 个故事 ID 未变。

本次局部核对（2026-10-28）：会话卡片改为在当前标签页打开终端，US-C11 退役并由新增 US-C14 取代，覆盖索引同步；依据 `web/src/components/SessionRow.svelte`、`web/test/browser/SessionRow.test.ts` 与 `web/test/e2e/session-links.spec.ts`。

本次局部核对（2026-10-28）：#194 的聊天端文件上传对应新增 US-Q14，并同步覆盖索引。核对依据为新增的 `web/test/browser/ChatUpload.test.ts` 与 `svelte-check`；未用真实 agent 验收，未重新核对其余功能组。

本次局部核对（2026-10-29）：#196 的聊天附件对应新增 US-Q15，并把 US-Q14 的上传结果从“插入路径”改为“待发送附件 chip”，覆盖索引同步。

- 依据：`internal/hybrid/attachments.go`、`internal/pathutil/pathutil.go`、`internal/acp/update.go`、fakeagent 的附件记录，`internal/hybrid`、`internal/pathutil` 与 `internal/wire` 的定向用例，以及 `web/test/browser/ChatUpload.test.ts`、`ChatView.test.ts`。
- 限制：未用真实 agent 验收，未重新核对其余功能组。167 个故事 ID 唯一。

本次局部核对（2026-10-29）：终端分屏对应新增 US-D13、US-D14，并更新 US-H08 的入口。

- 新增测试：布局单元与桌面/手机 Chromium 分屏用例，覆盖独立输入、嵌套布局、拖动、刷新、既有会话与聊天、关闭窗格和创建失败。
- 复审补充：US-D13、US-E03 的后台弹窗与待处理复制规则；修正 US-H08 的宽屏分屏入口；新增后台交互和宽屏触控快捷指令回归。
- 限制：只核对本次变化，未重新核验其他故事。

本次局部核对（2026-10-29）：worktree 起始提交标题对应 US-J04。默认提交和分支候选项展示提交标题，创建仍使用原引用或精确 OID，并保留手动输入。

- 已核对：J 组来源、故事 ID 与两份索引。
- 通过：定向 Go race、wire 契约、5 项表单浏览器测试、16 项桌面/Pixel 7 worktree E2E；亮暗/宽窄截图来自隔离真实后端。
- 限制：未重新核验其余故事能力。

本次局部核对（2026-10-29）：仓库侧栏、范围记忆、默认紧凑列表与详情披露对应 US-C01–US-C04、US-C07–US-C08、US-J02；保留聊天恢复、Shell 目录线索和异常状态的直接展示。验证结果以本 PR 检查记录为准，未重新核对其余故事。

本次局部核对（2026-10-29）：终端快捷键帮助对应新增 US-D15；键位按当前主分支核对，包括复制、终端查找、分屏、代码阅读与布局调整。定向桌面/手机 E2E、类型与前端架构检查通过；完整回归结果以 PR 验证记录为准，未重新核验其余故事能力。

本次局部核对（2026-10-30）：`enhance-markdown-preview` 对应 US-I16。完整回归结果以 PR 验证记录为准，未重新核验其余故事。

- 能力补充：工作区内相对链接与目录定位、标题锚点与“目录”、相对图片按需加载与失败原因、代码块语法高亮。
- 边界：明确聊天消息保持原有惰性渲染。
- 新增验证：路径规范化与 slug 单元测试、预览与目录浏览器测试，以及桌面/Pixel 7 E2E 用例。

本次局部核对（2026-10-30）：`add-code-document-preview` 对应新增 US-I17。完整回归结果以 PR 验证记录为准，未重新核验其余故事。

- 新增能力：HTML 静态预览、CSV/TSV 表格、JSON 树与 PDF 预览。
- 更新 US-I11 中 PDF 与二进制内容的边界。
- 新增验证：Go 识别与原始字节响应头测试、解析器单元测试、格式预览与 PDF 浏览器测试，以及桌面/Pixel 7 E2E 用例。

本次局部核对（2026-10-31）：`add-session-artifacts-folder` 的会话临时文件夹对应新增 US-G06、US-I18，并更新 US-E05；覆盖索引同步。完整回归结果以 PR 验证记录为准，未重新核验其余故事。

- 新增能力：按实例与会话隔离的 `$TMPDIR` 临时文件夹、代码栏“临时文件夹”根、⚡ 菜单固定提示词项与可编辑模板。
- 行为变化：上传改存本会话 `uploads/`，同名追加序号；会话删除（混合会话删除记录）后清理，聊天附件只接受本会话上传。
- 新增验证：`internal/artifacts`、代码视图与 API 的 Go race 测试，提示词与菜单、代码面板、设置的单元/浏览器测试，以及桌面/Pixel 7 E2E 用例。

## 阅读约定与产品边界

- **使用者**：通过桌面、手机或平板浏览器使用服务器上的 Shell、CLI agent 和其他终端程序的人。
- **部署者**：安装、配置和维护自己的 WebMux 服务的人，通常也是使用者。
- **贡献者**：开发、测试和发布 WebMux 的人；相关故事单独列在 L 节。
- WebMux 面向单用户，多设备连接仍共享同一服务用户的权限，不存在团队账号、成员角色或设备间文件隔离。
- **会话**是服务端持有的 PTY 与进程；**工作树**是磁盘上的 Git worktree。浏览器断开不会结束会话，关闭会话不会删除工作树。
- 会话、终端缓冲和发送确认记录不是跨服务重启的持久数据（混合会话只持久化与 agent 会话的对应关系，见 Q 节）；仓库清单持久保存在服务端，草稿、暂存和显示偏好保存在当前浏览器。
- 本文不承诺进程沙箱、公网访问安全、文件编辑、自动 clone/fetch、自动安装 agent 或跨服务重启恢复终端。
- 故事编号保持稳定。新增功能应补充故事；改变行为时同步修改验收要点与相关测试，不因排序调整而重新编号。

## 功能索引

| 分组 | 功能范围                               | 故事编号      |
| ---- | -------------------------------------- | ------------- |
| A    | 连接、认证与访问边界                   | US-A01–US-A06 |
| B    | 创建会话与选择启动位置                 | US-B01–US-B08 |
| C    | 会话总览、搜索与提醒                   | US-C01–US-C14 |
| D    | 终端操作、分屏、续接、多设备与生命周期 | US-D01–US-D15 |
| E    | 剪贴板与文件上传                       | US-E01–US-E07 |
| F    | 草稿编写、保存与发送确认               | US-F01–US-F09 |
| G    | 快捷指令                               | US-G01–US-G06 |
| H    | 手机、平板与触摸操作                   | US-H01–US-H09 |
| I    | 只读代码工作区、Diff 与引用            | US-I01–US-I18 |
| J    | 仓库登记与 worktree 管理               | US-J01–US-J11 |
| K    | 配置、运行与服务管理                   | US-K01–US-K12 |
| L    | 开发、验证与发布支持                   | US-L01–US-L10 |
| M    | 原生终端、CLI 与本机控制面             | US-M01–US-M08 |
| N    | Cloudflare 公网隧道                    | US-N01–US-N08 |
| O    | Windows/WSL 托盘管理                   | US-O01–US-O11 |
| P    | Tailscale Serve / Funnel               | US-P01–US-P07 |
| Q    | 混合会话（聊天 ↔ 终端）                | US-Q01–US-Q15 |

## 分组文档

各分组的完整故事、验收要点与依据位于以下文件；故事编号与上方索引保持一致。

| 文件                                                             | 分组               |
| ---------------------------------------------------------------- | ------------------ |
| [连接、启动与会话总览](user-stories/access-and-sessions.md)      | A–C                |
| [终端、输入与移动端](user-stories/terminal.md)                   | D–H                |
| [代码工作区与工作树](user-stories/code-and-worktrees.md)         | I–J                |
| [配置、运行与贡献者工具](user-stories/operations-and-tooling.md) | K–L                |
| [原生客户端、隧道与托盘](user-stories/clients-network-tray.md)   | M–P                |
| [混合会话](user-stories/hybrid-sessions.md)                      | Q                  |
| [功能覆盖核对与后续维护](user-stories/coverage.md)               | 覆盖索引与维护约定 |

本次局部核对（2026-10-29）：代码质量扫描修复对应 US-K09（`allowed_origins` 条目须为浏览器 Origin 的精确小写形式、`listen` 端口无效时启动即失败），删除会话的非“不存在”失败不再报 404；其余为内部并发、测试与无障碍修正，不改变故事行为。当前 168 个故事 ID 唯一；未重新核对其余故事能力。

本次补充核对（2026-10-29）：软键栏 Shift 修饰键对应 US-H01，点按为一次性、长按约 0.5 秒锁定并在再次点按后解除，支持 Shift+Tab 与带 Shift 的方向、Home/End、翻页序列；粘贴内容不应用修饰键。已运行前端类型检查、lint、架构与文档守卫、定向 Vitest 及 softkeys Playwright 用例；未重新核对其余故事。

本次局部核对（2026-10-31）：终端实时输出不再为攒批固定等待后续字节，对应 US-D01。
逐字回显延迟测试在修复前复现约 51 ms 等待，修复后三轮约 0.03 ms（本机 PTY 到 WebSocket）。
连续输出回归覆盖 14 万字节的完整性、顺序、64 KiB 实时帧上限和退出顺序。
未重新核验其余故事能力。
