# Portable handoff 核对记录

本页记录 2026-11-11 起交接增强层的实际验证。故事 ID 保持不变。
身份、Activity、历史接口、预览、inline 和同一对话切换的先前依据见[基础层记录](verification-log-2026-11-06.md)。

## Handoff 07：Codex 原生历史（US-Q12、US-Q20）

- 范围：可选 HistoryInjector、独立 stdio app-server 客户端、目标持有的注入 receipt、历史与用户 prompt 分别记账；预览不启动 agent 探测能力。
- 协议：核对 codex-acp 2.5.0/2.6.0 与 Codex 0.166/0.167 的固定源码；原生帧省略版本字段，角色保持 user/input_text 与 assistant/output_text。
- 实际 stdio：fake Codex 共享 ACP/native 原生存储，核对 coverage、每项角色、顺序和完整正文；用户新请求独立发送，load/restart 不重复 Activity 或 prompt。
- 故障：注入保存后退出保持 unknown，无 inline 回退；显式 load 的完整有序证明只确认历史，原用户命令单独继续，不重复注入。
- 边界：旧版本与明确 MethodNotFound 回退 inline；64 KiB 用户请求可独立于原生历史，inline 回退超限时拒绝且不截短。
- 存储：claim 后崩溃、错误原生 ID、部分证明、新命令绕过、后续用户失败和来源删除的 Go race 测试通过。
- 传输：无版本字段、紧随 ACK 的 EOF、阻塞写取消、超长帧拒绝、错误正文不泄漏的 Go race 测试通过。
- 集成：`make check`、全仓 `go test -race -p 4 ./...`（隔离 XDG/CODEX 环境）、268 项 wire 前端测试通过。
- same-thread 对账：load 使来源过期时保留历史事实和原用户草稿，旧命令拒绝发送；重新预览由用户决定。

## Handoff 08：只读历史 MCP（US-Q20）

- 范围：固定 argv scope 的 `mcp-serve`，经本机 HTTP API 读取冻结来源；数据库仍由服务端独占生命周期。
- 协议：采用 MCP initialize 时代协议，支持 2024-11-05 至 2025-11-25 的四个固定版本；现代 discovery 明确返回 MethodNotFound 以便客户端选择 legacy。
- 实际链路：活 SQLite → Unix HTTP API → CLI stdio MCP → 独立 fake provider，取回 45 项历史，分页无重复；长中文/emoji 按字节 offset 完整重建。
- 活跃复用：普通目标首次接入 MCP，以及连续两次 handoff 刷新冻结 source point；保留原 runtime/native ID，经真实 stdio 读到当前 scope。
- scope 与生命周期：非法 cursor/limit、跨 Conversation、未来项拒绝；new/load/resume 配置与关闭后的 helper 清理通过。
- 门禁：`make check`、隔离环境全仓 Go race、237 项 wire-contract 前端测试通过。补齐集成用例 Manager 关闭后，MCP+既有 CLI 联合重复 2 次通过；既有 CLI 单独重复 5 次通过。

## Handoff 09：浏览器交接与位置（US-Q20、US-Q05）

- 同一对话换 Agent 弹窗与新聊天页面共用预览、独立任务和服务端 revision；不读取 ChatModel 或拼接 quickStartMemory。
- API：新增按来源/模式恢复草稿，任务单独 PATCH 不改变 native 资格；冻结来源工作位置与目标位置分开。
- 桌面和 iPhone SE：20 项 E2E 通过，覆盖关闭/刷新/重启、A→B→A、三种位置、并发冲突、超预算、分片、unknown 与明确重试。
- 审查回归：恢复新工作树草稿保留已保存分支和精确基准，仓库 HEAD 移动后仍从原提交创建；发送前发现未审阅 revision 则不发送，进入冲突流程。桌面/手机 4 项针对性复验通过。
- 轮询回归：延迟旧 GET 不覆盖成功 autosave；另一设备的投递结果不清空本设备更新的用户任务，桌面/手机 4 项针对性 E2E 通过。
- 最终 UI 复验：38 项桌面/手机 handoff E2E 通过，发送与保存关闭先锁定编辑再等待 PATCH；目标记录删除、来源查询暂时失败不清空本设备未发送内容。
- 初始化竞争：分支列表加载前禁用新分支输入，避免建议名与输入拼接。延迟列表的桌面/手机 E2E 和 59 项工作树/新会话浏览器测试通过。
- 输入框/键盘缩小聊天视口时，位置未变的排队 scroll 事件不取消底部跟随；用户主动上滚仍暂停跟随。新增确定性事件顺序在修改前失败，修改后聊天/视口 34 项与完整 712 项 browser 通过，`make check` 通过。
- 聊天滚动补充浅深色及桌面/平板/手机前后截图；正常速度[手机多行输入与收起](../images/handoff/composer-follow.gif)保持最新回复可见。截图为正常交互，事件竞争由上述回归测试验证。
- 滚动修复后，桌面/手机的聊天↔终端往返、长历史重放与文档滚动 E2E 19 项通过，3 项按既有布局条件跳过。
- 关闭后焦点回到会话菜单，桌面/手机复验通过。补充桌面/手机 4 项 E2E 通过：新回合只标 stale，删除来源后清除本设备恢复缓存。
- Go race 覆盖全仓；随机来源路径引发的 wire 快照差异修复后 API 包复验通过，conversation/handoff/threadread/archtest 再验通过。
- 前端 786 项单元、712 项真实 Chromium 浏览器测试通过；`make check`、前端架构守卫和测试类型检查通过。
- 真实应用检查浅/深色及 1280×900、768×800、390×844、375×667、320×568、667×375；根页面无横向溢出，打开的菜单、Select 和弹窗均在视口内。
- [交接弹窗](../images/handoff/preview-light-desktop.png)、[手机新聊天](../images/handoff/fork-dark-phone.png)和前后截图使用隔离 fake agent 数据；版本提示反映 fake agent 的版本。
- 正常速度 GIF：[编辑、保存关闭与切换](../images/handoff/switch-flow.gif)，[位置选择、历史分片与新工作树投递](../images/handoff/fork-flow.gif)。
- Chromium 模拟未代替真实手机键盘/触屏；没有真机或原生 macOS 运行验证。鼠标 hover 媒体条件为 true、coarse 为 false，未核对真实触屏常驻与硬件悬停。

## Handoff 10：迁移、恢复与数据边界（US-Q05、US-Q12、US-Q20、US-K04）

- 全局 `history_capture=false` 保留正常聊天和身份/未决投递保护，拒绝新正文捕获与历史/交接/MCP；重新启用不回填关闭区间的原生 replay。
- 来源 forget 后，目标保留最小 dispatch 凭据与占用成本；unknown 继续阻止新命令，精确目标 replay 可对账，不恢复来源操作。
- JSON 与 SQLite 的发布/删除意图标记区分资源创建窗口和已删除运行记录；资源准备期间 forget 在关闭 writer 之前拒绝竞争。
- release/rebuild 标记与配置删除保留历史/草稿；forget、MaxRecords 淘汰与未开始 fork 草稿清理通过，原生目标事实按目标生命周期保留。
- 子进程在成功状态提交前退出后仍为 unknown；Activity 提交失败、磁盘满/只读模拟、重复 replay、旧 Run 后台 compaction 与 full 策略去重通过。
- 修复 fake provider 在 UTF-8 字符中间切 chunk；实际 stdio 测试用中文/emoji 核对正常聊天，关闭捕获也不损坏回复。
- 隔离环境全仓 Go race 通过，包含完整 `internal/archtest`；786 项 Vitest unit、712 项 Chromium browser、75 项 Storybook 通过。
- 完整桌面/手机 E2E 439 项通过，47 项按既有平台/布局条件跳过；`make check` 与新增文档后的独立 docs/arch 守卫通过。
- 组合输入视口 fixture 排除原生聚焦滚动和状态换行引起的 scroll anchoring，保留精确 50px 与结束后 0px 的断言；修正后 81 个 browser 文件全部通过。
- 1 万 Activity、500 次 selector、1 MiB 分块流、btrfs/tmpfs 对照、纯 Go Linux/Darwin 构建的真实数据见[故障矩阵与性能报告](../protocol/handoff-validation.md)。

## 核对范围与限制

07/08 层无可见 UI；09 层的界面证据和局限记录在上方。
测试使用隔离临时状态与 fake provider，未连接真实 provider 账号、未发出计费模型请求。
版本矩阵是上游协议核实；未宣称在原生 macOS 或真实移动设备上运行本层测试。
集成门禁和后续增强的实际结果在完成后追加，不把未运行项目记为通过。
