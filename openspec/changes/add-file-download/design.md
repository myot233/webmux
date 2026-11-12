## Context

- 当前 `main` 的代码栏已有两个根（#213，`add-session-artifacts-folder`）：
  - 绑定：`internal/codeview.Service.roots` 以 `rootKey{session, root}` 为键，`RootWorkspace = "workspace"`、`RootArtifacts = "artifacts"`。
  - 上下文：`GET /api/sessions/{id}/code/context?root=workspace|artifacts` 返回对应根的 `Context`。
    - 字段：`contextId`、`rootPath`、`rootKind`、`browseAvailable`、`diffAvailable`、`ignoreAvailable`、`reason`、`writableAvailable`。
    - 其余接口仍以 `contextId` 定位根（`withRoot` / `rootOfLocked`）。
  - 临时文件夹根是 `$TMPDIR/webmux-artifacts-<uid>-<实例摘要>/<会话ID>`（0700、属主校验见 `internal/artifacts`），`rootKind: "artifacts"`，无 Diff / 忽略项 / Git 状态；上传保存在其 `uploads/` 子目录，不再创建 `webmux-upload-*`。
  - 前端根状态在 `CodeWorkspace.svelte`（`root: "workspace" | "artifacts"`、`switchRoot`、`rootError`）；`CodeNavigator.svelte` 顶部用 `Tabs` 显示“工作区 | 临时文件夹”，临时文件夹根隐藏“修改”页签，`locateBase` 以文件夹本身为相对基点；`CodePanel.svelte` 只是承载 `CodeWorkspace` 与独立窗口的外壳。
- 原始字节（#110，已合入）：
  - `internal/codeview/raw.go` 的 `Raw` 通过 `snapshotRoot(id, sid)` 用快照所属根的绑定打开来源，工作树来源前后比较 `dev/inode/size/mtime`，索引 / HEAD 按 OID 读取；`MaxDownloadBytes = 100 MiB`（`media.go`），4 个传输槽（`transferConcurrency`）。
  - `internal/api/code.go` 的 `GET .../snapshots/{snapshotId}/raw` 在 10 秒 JSON 包装之外流式响应，支持 HEAD、`download=1`、RFC 2231 文件名编码，SVG / PDF 附加 CSP。
  - `Document` 已含 `size`、`downloadable`、`mediaType`。
- 资源管理器文件操作（#224，已合入）：`GET /code/node?contextId&nodeId`（`NodeRaw`，不经快照，按当前文件下载）与 `GET /code/archive`（目录流式 ZIP）；`CodeFileActions.svelte` 的右键 / 长按菜单提供“下载”“下载为 ZIP”，显示已接收字节并可取消。
- 前端下载工具已齐备：
  - `web/src/platform/api.ts` 的 `apiBlob(path, signal, received?)` 已累计 `response.body` 字节。
  - `web/src/platform/download.ts` 的 `saveBlob(owner, blob, filename)` 在所属文档中创建临时 `<a download>`，并在下一帧回收对象 URL。
  - `web/src/domains/code/code-image.ts` 的 `rawError` 已区分 401 / 403 / 404 / 409 / 410 / 413 / 429 / 网络错误。
- 文档动作栏：`CodeDocumentView.svelte` 只在图片（`source !== "empty"`）与超限 PDF 时渲染 `CodeDownloadButton`（`CodePdfView.svelte` 工具栏另有一个）；`downloadFailed` 已把 409 / 410 映射为 `updated` / `expired`。其余文档类型只显示原因，没有下载入口与确认步骤。
- 顶栏开关在 `TerminalHeader.svelte`，文案仍为“代码 / 折叠代码”；窄屏由 `SessionViewTabs.svelte` 的“代码”页签切换。
- 上传：`useUpload`（`web/src/hooks/shared/upload.svelte.ts`）在 `SessionTerminal.svelte` 中使用，成功后只把绝对路径插入终端或草稿，没有文件面板定位入口。

## Goals / Non-Goals

**Goals:**

- 让任意已打开的完整文件文档都能先确认来源与大小，再下载、显示进度、取消并得到可解释的失败结果。
- 复用既有快照 `raw` 端点、`apiBlob`、`saveBlob` 与 `rawError`，不写第二套传输逻辑。
- 上传完成后可一步定位到临时文件夹中的上传文件。
- 保持现有认证、Origin、路径边界、快照和会话生命周期语义。

**Non-Goals:**

- 不新增根（不浏览整个系统临时目录），不改变临时文件夹的位置、权限与生命周期。
- 不支持批量下载、Range / 断点续传、公开分享链接或产物数据库；目录 ZIP 与文件树菜单下载保持 #224 的现状。
- 不自动扫描目录、不从终端输出自动识别路径。
- 不改变文本 2 MiB / 20000 行、图片与 PDF 20 MiB 预览上限，也不改变终端 buffer 导出。
- 不实现后台下载、服务端流式落盘或 File System Access API。

## Decisions

### 1. 复用快照 raw，不新增 probe 接口

- 文档动作栏的下载走 `snapshots/{snapshotId}/raw?download=1`，下载对象就是当前打开的快照，打开后文件变化由 raw 的 409 处理；不改用 `/code/node`（它下载当前文件，可能与已读快照不同）。
- `Document` 已有 `path`、`absolutePath`、`size`、`downloadable`；确认框直接使用这些字段，不请求 `?probe=1`。
- 不可下载原因在客户端按以下顺序解释：来源 `empty` → 文件不存在；`size > 100 MiB` → 超过下载上限；`downloadable === false` → 不是常规文件或来源不可读；否则可下载。

### 2. 文档动作栏入口

- `CodeDocumentView` 的动作栏为任何 `doc.source !== "empty"` 的完整文件显示“下载文件”；文本、图片、PDF、二进制、编码不支持与文本超限文档共用同一入口，取代目前只对图片与超限 PDF 渲染的条件。`doc.downloadable === false` 时按钮禁用并显示原因。
- `CodePdfView` 工具栏与图片 Diff 侧的既有下载按钮保留，同样经过确认。
- 两个根都适用；临时文件夹根的文档同样经 `snapshotRoot` 读取，不回退到工作区。

### 3. 下载确认、进度与取消

- 在 `CodeDownloadButton` 内加入确认对话框：显示文件名、`absolutePath`、可读大小与所属根标签（“工作区” / “临时文件夹”）；取消不发送请求。确认后对话框保持打开，显示已接收字节 / 总量（`size` 已知时显示百分比）、取消按钮和错误说明。
- 传输用 `apiBlob(path, signal, received)`；`AbortController` 在取消、关闭对话框、切换文档或卸载时中止；401 继续触发 `webmux:auth`。
- 保存用 `saveBlob(button.ownerDocument, …)`，兼容独立代码窗口；失败文案用 `rawError`，409、410 经 `downloadFailed` 分别把文档标记为 `updated`、`expired` 并提示刷新。
- 100 MiB 文件会在浏览器内存中形成 Blob，低端手机可能吃紧；这是已知取舍。

### 4. 顶栏文案

- `TerminalHeader.svelte` 的代码区开关由“代码”改为“文件”（展开时桌面显示“折叠文件”，非桌面展开时仍显示“终端”或混合会话的“聊天”）；`SessionViewTabs.svelte` 窄屏页签同步改为“文件”；`App.svelte` 独立代码窗口的 `label` 与相关 `aria-label` 一并检查。这是发现性调整，不改变面板结构。
- 规格 `terminal-code-references` 中“从‘代码’切回‘终端’”的措辞随实施一并评估是否需要 delta。

### 5. 上传后定位

- `useUpload` 在批量成功后保留本次上传的绝对路径（至少最后一个），`SessionTerminal` 显示可关闭的成功提示和“在临时文件夹中查看”动作；现有错误提示与路径插入行为不变。
- 触发定位时打开代码区（如尚未显示），把目标路径交给 `CodePanel` → `CodeWorkspace`；工作区切到 `artifacts` 根（需要时先调用确保接口），用该根的 `/locate` 把绝对路径解析为 `nodeId`，展开 `uploads/` 并让文件出现在当前列表。路径不在临时文件夹根内时，只切换根并说明无法定位，不打开工作区或其他路径。
- 定位不自动打开文件内容，也不自动下载。

### 6. 限制与安全

- 传输上限沿用 `MaxDownloadBytes = 100 MiB`；超过时 413 且不发送正文。
- 认证只接受 Bearer 头；token 不进入 URL、Referer 或日志，不生成公开分享地址。Origin 与 Host 检查沿用 `/api` 中间件。
- 响应头沿用 `sendTransfer`：`Content-Length`、`Content-Disposition`（`download=1` 时 `attachment`）、`Cache-Control: no-store`、全局 `nosniff`，SVG / PDF 附加 CSP。

### 7. 验证与文档

- Go：补充临时文件夹根的 `Raw` 字节一致、打开后改写 409 与跨根 / 跨会话快照拒绝（现有 `raw_test.go` 只覆盖工作区，`artifacts_test.go` 只覆盖浏览与快照读取）。
- 浏览器组件：确认框元数据、取消不请求、进度、409 / 410 / 413 / 429 / 401 / 网络错误、非文本下载入口、上传后定位。
- E2E：在 `web/test/e2e/codeview.spec.ts`（或 `artifacts.spec.ts`）中从文档动作栏下载文本、ZIP 与超过文本上限的文件并比对 SHA-256；上传后定位到临时文件夹再下载；桌面与 Pixel 7 项目运行。
- 文档：`docs/usage.md` 的“代码工作区”补充文档动作栏下载与确认；`docs/user-stories/code-and-worktrees.md` 新增或更新故事并同步 `docs/user-stories.md` 索引与 `coverage.md`。

## Risks / Trade-offs

- [100 MiB Blob 占用手机内存] → 只由用户确认后触发；保留 100 MiB 上限并避免自动下载。
- [发送中文件变化导致浏览器看到不完整下载] → 服务器在内容长度不满足时中断响应，界面按下载失败处理并提示重试。
- [确认步骤增加一次点击] → 只用于文档动作栏的单文件下载；文件树菜单下载保持 #224 的直接下载。

## Migration Plan

- 无数据迁移，无新增接口；前后端随同一二进制发布。
- 回滚即回退本变更；raw 端点、文件树下载与两个根保持可用。

## Open Questions

- 文件树菜单的“下载”是否也走确认框，留待实施时与 #224 的交互一并评估；本规格只要求文档动作栏入口确认。
