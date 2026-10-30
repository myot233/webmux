## Context

- 上传：没有按会话归属，也不随会话删除或启动清扫。
  - `internal/files.StoreUpload` 每次 `os.MkdirTemp("", "webmux-upload-")`；`Upload.Discard` 用 `os.RemoveAll` 删除整个目录。
  - `internal/pathutil.UploadPath` 只认 `$TMPDIR/webmux-upload-*/<name>`；唯一调用方是 `internal/hybrid/attachments.go` 的 `resolveAttachments`、`readAttachment`。
- 代码栏：`internal/codeview.Service` 以会话 ID 为键缓存一个 `binding`（根 = 启动目录或其 Git toplevel），每次请求经 `withRoot` 重新校验并比对 `contextId`；1 s `sweep` 轮询 `manager.Get` 丢弃已删除会话的绑定。`open` 以 `O_NOFOLLOW` 逐段打开，不跟随符号链接。
- 会话删除：`Manager.Remove` 发布 `removed` 事件（`Manager.Subscribe()` 返回 `ListSubscriber`，慢订阅者被断开）。混合会话关闭时 `internal/hybrid` 也调用 `Manager.Remove`，但记录保留为可恢复；`DELETE /api/hybrid-sessions/{id}` → `hybrid.RemoveRecord` 才真正删除。恢复以原会话 ID 重建会话。
- 设置：`internal/settings` 的 `Values`/`apply`/`validateValues`/`normalize`/`cloneValues`，`internal/contract/settings.go`，`internal/wire/settings.go` 的 `shapes`，前端 `SettingsValuesSchema` 与 `settings-intent.ts` 需逐一加键。
- 快捷指令菜单：`QuickCommandMenu.svelte` 由 `SessionTerminal.svelte` 的 `quickMenu` snippet 渲染于顶栏或软键栏；`runQuickCommand` 走 `insertText`（草稿或 PTY 粘贴）或 `t.submit`（带回车）。聊天视图不渲染该菜单。
- 层规则（`docs/architecture.md` R0–R10）：Service 可以依赖其他 Service；Controller 不得直接操作文件系统；新 `internal/` 包需登记到 `internal/archtest/layers_test.go`；新的 `os.RemoveAll`/`os.OpenFile` 入口需登记到 `internal/archtest/persistence_inventory.json`。

## Goals / Non-Goals

**Goals:**

- 一个按实例与会话隔离、权限私有、可预测路径的交互物临时文件夹，生命周期跟随会话删除（混合会话跟随记录删除）。
- 上传统一落到该文件夹 `uploads/`，聊天附件校验收紧到本会话。
- 代码栏以第二种根只读浏览该文件夹，复用现有树、搜索、预览、变化检测与安全打开逻辑。
- ⚡ 菜单固定项按模板插入提示词，模板可在设置中编辑并跨设备同步。

**Non-Goals:**

- 聊天视图（混合会话聊天端）的快捷入口与斜杠命令。
- 向会话环境注入 `WEBMUX_ARTIFACTS_DIR` 等变量；在代码栏内写入、删除、重命名文件。
- 配额、过期时间、跨服务重启保留普通会话文件夹（普通会话本身不跨重启）。
- 迁移已有 `webmux-upload-*` 目录：它们留给系统临时目录清理机制。

## Decisions

### D1 新服务包 `internal/artifacts`

拥有实例根目录、路径解析、确保、删除、清扫与上传路径校验。`artifacts.New(Options{TempDir, StateDir, Manager, Retain func(id string) bool, Logger})`，由 `cmd/webmux` 构造并注入 `files`、`codeview`、`hybrid` 与 `api.Deps`。

- 备选：并入 `internal/files`。`files` 是无状态的路径补全/上传工具，不订阅会话事件；把生命周期放进去会让它依赖 `session.Manager` 且职责混杂。独立包更清楚，`files` 只接收“写到哪个目录”。
- 移除 `pathutil.UploadDirPrefix` 与 `pathutil.UploadPath`（无其他调用方），替换为 `artifacts.Service.UploadPath(sessionID, path)`。

### D2 路径：`<canonical TMPDIR>/webmux-artifacts-<uid>-<hash8(canonical state_dir)>/<sessionID>`

- 实例摘要隔离多个实例（E2E worker 共用 `TMPDIR` 但各自有 `state_dir`），使启动清扫只碰自己的根；路径稳定，因此可恢复混合会话跨重启仍能找到文件夹。
- 根与会话目录以 `Mkdir(0700)` 创建；已存在时 `Lstat` 校验目录、非符号链接、`Stat_t.Uid == euid`、`mode&0077 == 0`，否则返回 `ErrUnavailable`（`api/errors.go` 映射 503）。会话 ID 先按会话 ID 字符集校验，防止路径注入。
- 备选：每进程 `MkdirTemp` 随机根。清扫简单，但混合会话恢复后丢失文件夹，且需额外记录上次路径；放弃。
- 备选：放在 `state_dir` 下。用户明确选择系统临时目录。

### D3 生命周期：事件 + 对账

- `artifacts.Service` 订阅 `Manager.Subscribe()`：收到 `removed` 时，若 `Retain(id)` 为假则删除该会话目录。收到 `Slow`（被断开）时重新订阅并执行一次对账。
- 对账 = 列出根目录直接子项，删除“不在 `Manager` 中且 `Retain` 为假”的目录；只删除名称合法的目录，`RemoveAll` 前 `Lstat` 确认不是符号链接。
- `Retain` 由 `cmd` 注入：混合记录存在（任意状态）返回真；混合记录服务不可用时返回真（保守）。启动时在 `hybrid` 打开之后、`server.Serve` 之前调用 `Sweep()`；若混合记录不可读则跳过整次清扫并告警。
- 混合记录删除：`internal/api` 的 `removeHybridRecord` 在 `RemoveRecord` 成功后调用 `s.Artifacts.Remove(id)`（Controller 只调用注入实例方法，符合 R5）。
- 备选：只在 `api.delete` 里挂钩。会漏掉 hybrid 内部或将来其他调用 `Manager.Remove` 的路径；事件订阅覆盖全部路径。

### D4 上传布局

`files.StoreUpload(dir, filename, src, limit)` 接收目标目录（由 `api.upload` 先调用 `Artifacts.UploadsDir(id)` 确保）；在 `uploads/` 内用 `O_CREAT|O_EXCL|O_NOFOLLOW` 0600 创建 `name`、`name (2).ext`…，直到成功（上限 1000 次后 `ErrInternal`）。`Upload.Discard` 改为只删除该文件（`os.Remove`）。

- 备选：`uploads/<random>/<name>`。避免改名，但在代码栏中多一层无意义目录，提示词也更难描述；采用序号后缀。

### D5 代码栏第二种根

- `codeview` 绑定键改为 `(sessionID, rootKind)`；`Context(ctx, id, root)` 新增 `root` 参数（`workspace` | `artifacts`）。其余操作仍以 `(id, contextId)` 定位：`withRoot` 在该会话的两种绑定中按 `contextId` 查找，路由与服务签名不必为每个接口加 `root`。
- 临时文件夹根绑定：`Root` 为会话文件夹规范路径，`Kind "artifacts"`、`Browse true`、`Diff/Ignore false`，不运行任何 git 命令；`validate` 只做根目录 identity 校验。目录不存在时返回 `ErrNotFound`（客户端先调确保接口）。
- `Changes`/`Diff` 对 artifacts 上下文返回 `ErrUnavailable`；`Locate` 仅在该根内解析。
- `sweep` 同步丢弃已删除会话的两种绑定。
- 前端：
  - `CodePanel.svelte` 新增 `root` 状态；切换时重置 `context/listing/rootListing/nav`，`nav` 强制为 `files`。
  - `CodeNavigator.svelte` 顶部用 `Tabs` 呈现“工作区 | 临时文件夹”，与“文件/修改”同一组件，不新造控件；artifacts 根隐藏“修改”。
  - 标签键已包含 `contextId`；标签模型额外记录 `rootKind`，用于“临时文件夹”标注。
- 备选：在工作区树中挂一个虚拟节点。会把系统临时目录混入 Git 工作区语义（修改、ignore、定位基点），放弃。

### D6 提示词模板与插入

- 内置模板只在前端定义（`web/src/artifactPrompt.ts`，与 `quickCommands.ts` 预置指令同一惯例）；服务端只校验自定义字符串（非空、≤4096 UTF-16、含 `{dir}`、规范化换行）。`null` 与字符串的区分同 `quickCommands`。
- `{dir}` 用 `replaceAll` 原样替换为确保接口返回的路径，不做 shell 引号（目标是自然语言提示，不是 shell 参数）。
- `SessionTerminal.svelte` 新增 `insertArtifactPrompt()`：在入口处 `drafts.begin(draft)` 捕获操作并绑定会话，`await` 确保接口后用同一 operation 调用 `insertText`；从不调用 `t.submit`，因此不会回车。失败通过现有终端提示区显示。
- `QuickCommandMenu.svelte` 新增 `artifact()` 回调 prop，固定项受与指令条目相同的 `blocked` 规则约束。
- 设置页：在快捷指令卡片内追加“临时文件夹提示词”区块（满足 `server-settings-sync` 现有“两张卡片”约定）；`settings-intent.ts` 对 `artifactPrompt` 采用整体覆盖重放。

### D7 契约

- `contract.ArtifactsFolder{Path}`（`POST /api/sessions/{id}/artifacts` 200）；`CodeContext.RootKind` 增加 `artifacts`；`SettingsValues.ArtifactPrompt *string`。
- 前端 `ArtifactsFolderSchema`、`CodeContextSchema.rootKind` 枚举、`SettingsValuesSchema.artifactPrompt` 同步；重录 `internal/api/testdata/wire`（上传、设置、新增 artifacts 与 code-context-artifacts 快照），`wire_normalizer_test.go` 把实例根与会话 ID 归一化。

## Risks / Trade-offs

- [共享 `/tmp` 中路径可预测，可能被抢先创建] → `Mkdir` 而非 `MkdirAll`，存在时严格校验属主/权限/非链接，失败即 503，不尝试修复或删除。
- [删除会话时 agent 仍在向文件夹写入] → 删除发生在会话已退出并被移除之后；残留进程写入的新文件由下一次对账或启动清扫回收。
- [`Retain` 判断错误导致误删可恢复会话的文件] → 记录服务不可用时一律保留；清扫只删除名称合法、非链接的直接子目录。
- [慢订阅被断开漏掉 `removed`] → 断开后重新订阅并对账。
- [上传路径约定变化影响已在草稿或待发送附件中的旧路径] → 升级后旧 `webmux-upload-*` 附件会被拒绝为无效附件；属一次性影响，用户重新上传即可。
- [系统临时目录被 systemd-tmpfiles 或重启清理] → 与“临时”语义一致；确保接口按需重建，代码栏以失败提示 + 重试处理绑定失效。
- [代码栏 `withRoot` 需在两种绑定中查找] → 每会话最多两项，开销可忽略。

## Migration Plan

- 无数据迁移；部署新二进制即生效。旧 `webmux-upload-*` 目录不再被读取或创建。
- 回滚：旧版本忽略 `webmux-artifacts-*` 目录，但设置文件需要注意。
  - `settings.json` 由 `statefile` 严格解码，未知键会使旧版本判定设置损坏（503）。
  - 因此 `settings.Values.ArtifactPrompt` 沿用 `ChatIdleSleepMinutes` 的先例使用 `json:",omitempty"`：值为 `null` 时不写入文件，文档版本号不变。
  - 回滚前在设置页“恢复默认”（PATCH `null`），旧版本即可正常读取。

## Open Questions

- 无阻塞问题。
