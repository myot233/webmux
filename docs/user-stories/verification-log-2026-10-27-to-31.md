# 核对记录：2026-10-27 至 2026-10-31

本文收录 2026-10-27 至 2026-10-31 各次局部核对的记录：对应的故事、已运行的验证、范围限制与未核对的部分。

条目由[用户故事索引](../user-stories.md)原文迁入，按日期升序排列，措辞保持当时的口径；故事的当前内容以各分组文档为准。

其他时间段见[verification-log-2026-10-23-to-26.md](verification-log-2026-10-23-to-26.md)、[verification-log-2026-11.md](verification-log-2026-11.md)。

本次局部核对（2026-10-27）：推送与站内关注文案统一并补充下一步提示，对应新增 US-C12 并更新 US-C06。

- 推送正文改为“短标签 · 下一步”，`done` 在列表与推送中统一为“本轮完成”；服务 worker 兜底文案由 Go 用例锁定同步。
- 通过：`internal/push`、`internal/api`、`internal/wire` Go race、`go vet`、Vitest Service Worker 与推送卡片用例、`attention.spec.ts` / `push.spec.ts` 桌面与 Pixel 7 E2E、`svelte-check`、Oxlint、前端架构守卫、格式检查与文档守卫。
- 一致性：148 个故事 ID 唯一；未重新核对其余功能组。

本次局部核对（2026-10-27）：#161 的 PWA 第一批对应新增 US-H09，并更新 US-D03、US-H06、US-K10、US-C12。

- 已实现：健康终端连接回到前台改为探测保留；安装应用分区、非安全上下文提示、manifest `id`/`lang` 与图标尺寸检查；推送授权只请求一次。
- 不支持：离线启动和子路径部署。
- 通过：Go `internal/api` 测试、Vitest 单元与浏览器全套、Playwright E2E 全套（本地桌面与 Pixel 7 Chromium）、`svelte-check`、Oxlint、前端架构守卫与格式检查；149 个故事 ID 唯一。
- 尚未验收：Android Chrome、桌面 Chrome/Edge、iOS Safari 真机安装入口。

本次局部核对（2026-10-27）：#162 的浏览器标签页运行指示对应新增 US-C13。

- 标题与 favicon 由 `web/src/platform/browser-chrome.ts` 统一管理，终端页与首页按 `status === "running"` 接线，静态页面清除残留动画。
- 动画使用本地生成的约 2 fps 帧；`prefers-reduced-motion: reduce` 时图标保持静态运行帧、标题仍保留标记。

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

本次局部核对（2026-10-28）：会话卡片改为在当前标签页打开终端，US-C11 退役并由新增 US-C14 取代，覆盖索引同步；依据 `web/src/components/sessions/SessionRow.svelte`、`web/test/browser/SessionRow.test.ts` 与 `web/test/e2e/session-links.spec.ts`。

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

本次局部核对（2026-10-29）：代码质量扫描修复对应 US-K09（`allowed_origins` 条目须为浏览器 Origin 的精确小写形式、`listen` 端口无效时启动即失败），删除会话的非“不存在”失败不再报 404；其余为内部并发、测试与无障碍修正，不改变故事行为。当前 168 个故事 ID 唯一；未重新核对其余故事能力。

本次补充核对（2026-10-29）：软键栏 Shift 修饰键对应 US-H01，点按为一次性、长按约 0.5 秒锁定并在再次点按后解除，支持 Shift+Tab 与带 Shift 的方向、Home/End、翻页序列；粘贴内容不应用修饰键。已运行前端类型检查、lint、架构与文档守卫、定向 Vitest 及 softkeys Playwright 用例；未重新核对其余故事。

本次局部核对（2026-10-30）：`enhance-markdown-preview` 对应 US-I16。完整回归结果以 PR 验证记录为准，未重新核验其余故事。

- 能力补充：工作区内相对链接与目录定位、标题锚点与“目录”、相对图片按需加载与失败原因、代码块语法高亮。
- 边界：明确聊天消息保持原有惰性渲染。
- 新增验证：路径规范化与 slug 单元测试、预览与目录浏览器测试，以及桌面/Pixel 7 E2E 用例。

本次局部核对（2026-10-30）：`add-code-document-preview` 对应新增 US-I17。完整回归结果以 PR 验证记录为准，未重新核验其余故事。

- 新增能力：HTML 静态预览、CSV/TSV 表格、JSON 树与 PDF 预览。
- 更新 US-I11 中 PDF 与二进制内容的边界。
- 新增验证：Go 识别与原始字节响应头测试、解析器单元测试、格式预览与 PDF 浏览器测试，以及桌面/Pixel 7 E2E 用例。

本次局部核对（2026-10-31）：当时新安装只预置 Shell、Claude Code、Codex CLI、Open Code、omp，Gemini 图标与已有配置仍可用，对应 US-B01、US-B08。该默认列表已由 2026-11-01 更新取代。

- 已运行 `internal/launch` 单元测试与真实二进制空状态目录烟测，未重新核对其余功能组。

本次局部核对（2026-10-31）：`add-session-artifacts-folder` 的会话临时文件夹对应新增 US-G06、US-I18，并更新 US-E05；覆盖索引同步。完整回归结果以 PR 验证记录为准，未重新核验其余故事。

- 新增能力：按实例与会话隔离的 `$TMPDIR` 临时文件夹、代码栏“临时文件夹”根、⚡ 菜单固定提示词项与可编辑模板。
- 行为变化：上传改存本会话 `uploads/`，同名追加序号；会话删除（混合会话删除记录）后清理，聊天附件只接受本会话上传。
- 新增验证：`internal/artifacts`、代码视图与 API 的 Go race 测试，提示词与菜单、代码面板、设置的单元/浏览器测试，以及桌面/Pixel 7 E2E 用例。

本次局部核对（2026-10-31）：混合会话聊天视图缺少快捷指令入口，对应更新 US-G02。聊天视图在顶栏显示 ⚡，指令与临时文件夹提示词只插入消息输入框，不自动发送；新增桌面/Pixel 7 E2E 用例（`web/test/e2e/hybrid.spec.ts`）。未重新核验其余故事。

本次局部核对（2026-10-31）：基于上游 `1cdda765` 补齐九款 CLI agent 的首次启动配置与品牌图标，对应 US-B01、US-B08。

- 通过：53 项启动配置单元、21 项浏览器组件、6 项桌面/Pixel 7 E2E，以及相关 Go race、类型、lint、架构、文档治理与生产构建检查。
- API race 初轮并行执行时已有突发输出用例触发慢客户端断连，单独重跑 API 包通过。
- 隔离生产实例以替身 CLI 验证九款配置的命令、参数边界与退出码，核对桌面/393px 手机的浅色和深色截图；没有使用真实账号或调用模型。
- 核对了 174 个故事 ID 的唯一性、两份索引引用及 B 组来源路径；未重新验收其余功能组或真实手机键盘。

本次局部核对（2026-10-31）：统一品牌标志对应 US-A01、US-H09。

- 通过：生产构建、应用与 Storybook 类型检查、lint、前端架构守卫、两项工作区 UI E2E、Manifest/API 定向 Go race 及文档治理。
- 截图：真实隔离后端，1280×900 桌面和 390×900 窄屏，包含明暗主题的前后对比；样本为三个测试 Shell 会话，无私有终端输出。
- 一致性：174 个故事 ID 唯一，两份索引与 A/H 组来源路径有效；192/512 图标尺寸匹配声明，maskable 图形位于安全圆内。
- 限制：未做真机安装或系统裁切验收，未重新核对其余功能组。

本次局部核对（2026-10-31）：终端实时输出不再为攒批固定等待后续字节，对应 US-D01。
逐字回显延迟测试在修复前复现约 51 ms 等待，修复后三轮约 0.03 ms（本机 PTY 到 WebSocket）。
连续输出回归覆盖 14 万字节的完整性、顺序、64 KiB 实时帧上限和退出顺序。
未重新核验其余故事能力。

本次局部核对（2026-10-31）：#215 独立代码窗口中修改视图的行号 Shift 范围选择与重新选择失效，对应更新 US-I13。
diff 库的行号按压只在主页面 `document` 上等待 pointerup，现由 `CodeDiffReader` 在独立窗口中转发；`code-popout.spec.ts` 新增用例修复前失败、修复后通过。
未重新核验其余故事能力。

本次局部核对（2026-10-31）：#216 首页未保存显示方式时恢复默认输出预览，显式选择的紧凑列表仍在当前浏览器记忆，对应更新 US-C02。
`repository-scope.spec.ts` 改为断言默认预览、切换紧凑后刷新仍保持紧凑；未重新核验其余故事能力。

本次局部核对（2026-10-31）：圆角统一为设计系统刻度并由 `arch:test` 检查，对应更新 US-L06；聊天卡片、仓库侧栏、紧凑列表面板和空状态由 12px 改为内容容器 8px，复选框、滑块、滚动条与键帽不再使用直角或固定像素。
新规则在改动前的源码上列出 18 处违规，改动后 `check:arch` 通过；未重新核验其余故事能力。

本次局部核对（2026-10-31）：US-D01 的高吞吐输出在 CPU 紧张时不再误断开。持续输出产生大量小块读取（约 340 字节），写协程被调度延后时 256 条队列先满，以 4408 断开（macOS 与 Ubuntu CI 均失败）。
终端输出队列改按积压字节限制（8 MiB，最多 1024 条）；`TestTerminalBurstPreservesOutputBeforeExit` 在 `-race -cpu 1`/`-cpu 2` 下由 23/100、9/200 失败变为 0/300，与全量 Go race 并行时 0/150；新增字节预算单元测试。测试客户端读取上限提高到 1 MiB（输出帧最大 64 KiB）。未重新核验其余故事能力。
