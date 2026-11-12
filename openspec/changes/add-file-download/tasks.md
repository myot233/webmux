## 1. 依赖与契约对齐

- [x] 1.1 确认 #110（`add-code-image-preview`）已合入，并核对 raw 路由、`MaxDownloadBytes`、`Document.size/downloadable` 与现有下载组件
  - 合入：PR #110 于 2026-10-24 合入为 `ac82b9b3`，变更已归档为 `2026-10-29-add-code-image-preview`。
  - 路由：`GET /code/snapshots/{snapshotId}/raw` 见 `internal/api/code.go`。
  - 上限：`MaxDownloadBytes = 100 MiB` 见 `internal/codeview/media.go`。
  - 字段与组件：`Document.size/downloadable` 见 `internal/codeview/types.go`，组件为 `CodeDownloadButton.svelte`。
- [x] 1.2 若 #110 最终路由、字段或上限与本规格不同，先更新本变更的 Context 与 Decisions，再开始实现（已按 `main` 更新：两个根由 #213 以 `artifacts` 根交付，文件树下载 / ZIP 由 #224 交付）
- [x] 1.3 确认临时根实现不复制 raw 传输、身份校验、HEAD 或文件名编码逻辑（`Raw` 通过 `snapshotRoot` 复用同一 `openRaw` 与 `sendTransfer`，见 `internal/codeview/raw.go`、`internal/api/code.go`）

## 2. 服务端双根（由 `add-session-artifacts-folder` / #213 交付）

- [x] 2.1 `?root=workspace|artifacts` 选择根，`Context.rootKind` 区分根（`internal/api/code.go`、`Service.Context`）；未采用 `rootId` / `label` / `roots` 字段，客户端固定两个根
- [x] 2.2 `Service.roots` 以 `rootKey{session, root}` 保存绑定、`withRoot` 按 `contextId` 查找；`sweep` 与 `Close` 清理两个根（`internal/codeview/service.go`）
- [x] 2.3 第二个根为会话临时文件夹 `$TMPDIR/webmux-artifacts-<uid>-<实例摘要>/<会话ID>`（`bindArtifacts`，`Diff`/`Ignore` 为 false），不可用时前端显示原因与重试（`CodeNavigator.svelte` 的 `rootError`）
- [x] 2.4 归属隔离由文件夹 0700 与属主校验保证（`internal/artifacts`），不再浏览整个共享临时目录，无需逐条 UID 过滤
- [x] 2.5 临时文件夹根沿用 `binding.open` 逐段 `O_NOFOLLOW` 规则；`Diff=false` 时 `scanChanges`（`changes.go`）与索引 / HEAD 来源（`diff.go`）拒绝，`TestArtifactsRoot` 覆盖 `Changes` 拒绝
- [x] 2.6 定位基点：临时文件夹根以文件夹本身为相对基点（`CodeNavigator.svelte` 的 `locateBase`）
- [x] 2.7 Go 测试：`TestArtifactsRoot` 覆盖临时文件夹根浏览、快照、拒绝 Changes、不跟随符号链接与失效上下文 409

## 3. 根感知快照与原始字节

- [x] 3.1 快照通过 `contextId` 记录所属根，`Raw` / `SnapshotFile` 用 `snapshotRoot` 取对应绑定（`internal/codeview/service.go`、`raw.go`）
- [x] 3.2 保持 #110 的服务槽释放时机、4 个传输槽、写期限续期、工作树发送前后身份校验与发送中中断（`holdTransfer`、`deadlineWriter`、`Transfer.WriteTo`）
- [x] 3.3 `/code/context?root=`、`Content-Disposition` / HEAD / 413 / 429 接入现有 raw 路由（`internal/api/code.go` 的 `codeRaw` / `sendTransfer`）
- [ ] 3.4 Go 测试：临时文件夹根的 `Raw` 字节一致、打开后改写 409、跨根 / 跨会话快照拒绝（现有 `raw_test.go` 只覆盖工作区）

## 4. 前端契约与下载

- [x] 4.1 `web/src/contracts/schemas.ts`：`CodeContextSchema` 已含 `rootKind`、`writableAvailable`；根由请求参数选择，无需新增 `rootId` / `roots`
- [x] 4.2 `web/src/platform/api.ts`：`apiBlob(path, signal, received)` 已累计 `response.body` 字节，保留 Bearer、`APIError`、401 事件与 `AbortSignal`
- [ ] 4.3 `CodeDownloadButton.svelte` 增加下载确认对话框：文件名、`absolutePath`、大小、所属根、不可下载原因；取消不发请求；确认后显示进度（`size` 已知时显示百分比）、取消与错误，保存沿用 `saveBlob(ownerDocument, …)`
- [ ] 4.4 `CodeDocumentView.svelte`：为所有 `source !== "empty"` 的完整文件提供统一下载入口（当前仅图片与超限 PDF）；`downloadable=false` 时禁用并说明原因
- [x] 4.5 下载失败映射：`rawError` 区分 401 / 403 / 404 / 409 / 410 / 413 / 429 / 网络错误，`downloadFailed` 把 409 → `updated`、410 → `expired`；`saveBlob` 下一帧回收对象 URL
- [ ] 4.6 浏览器测试：确认框元数据、取消不请求、进度、失败状态、非文本与文本下载入口、对象 URL 回收

## 5. 根切换、文件面板与上传定位

- [x] 5.1 `CodeNavigator.svelte`：“工作区 | 临时文件夹”切换器、不可用原因与重试；临时文件夹根隐藏“修改”页签
- [x] 5.2 `CodeWorkspace.svelte`：保存 `root`，`switchRoot` 中止待处理请求并重新加载上下文；根路径行显示 `context.rootPath`
- [x] 5.3 `CodeDocumentView.svelte`：接收 `diffAvailable`，`CodeWorkspace` 对 `rootKind === "artifacts"` 的标签传 false，不显示“查看 Diff”
- [ ] 5.4 `TerminalHeader.svelte`：顶栏“代码”改为“文件”，桌面展开态显示“折叠文件”，非桌面展开态保持“终端” / “聊天”；`SessionViewTabs.svelte` 窄屏页签同步；检查 `aria-label` 与 `terminal-code-references` 规格措辞
- [ ] 5.5 `useUpload`（`web/src/hooks/shared/upload.svelte.ts`）/ `SessionTerminal.svelte`：批量上传成功后显示“在临时文件夹中查看”；触发时打开代码区、切到 `artifacts` 根，经 `/locate` 定位到 `uploads/`，不自动打开或下载
- [ ] 5.6 浏览器测试：上传提示与定位、路径不在临时文件夹根时的说明

## 6. 端到端与回归

- [ ] 6.1 `web/test/e2e/artifacts.spec.ts`（或 `codeview.spec.ts`）：在临时文件夹写入随机二进制文件，从文档动作栏确认并下载，比对 SHA-256
- [ ] 6.2 覆盖工作区文本 / ZIP / 超过文本上限文件从文档动作栏下载，以及上传后定位到临时文件夹再下载
- [ ] 6.3 确认下载不修改源文件、mtime 与 Git 状态；文件变化、快照过期、超过 100 MiB 与取消均有可解释结果
- [ ] 6.4 在桌面与 Pixel 7 项目运行目标 E2E；运行 `go test -race ./internal/codeview ./internal/api`、目标 Vitest 与 `make check`

## 7. 文档、用户故事与验收

- [ ] 7.1 `docs/protocol/http-workspace.md`：如前端用法引入新约定（例如确认依赖的字段语义），同步说明；raw / node / archive 路由与 100 MiB 上限已有记录
- [ ] 7.2 `docs/usage.md`“代码工作区”：补充文档动作栏“下载文件”、确认与错误；`docs/usage/terminal.md` 补充上传后“在临时文件夹中查看”
- [ ] 7.3 `docs/user-stories/code-and-worktrees.md`：新增或更新文件下载故事（关联 US-I15、US-I18、US-I19），同步 `docs/user-stories.md` 索引与 `docs/user-stories/coverage.md`
- [ ] 7.4 `openspec validate add-file-download --strict --no-interactive` 通过
- [ ] 7.5 UI 变更的 PR 描述附桌面 / 手机与亮 / 暗主题截图，使用隔离样例数据
