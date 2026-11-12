## Why

#69 需要把会话中明确选中的普通文件下载到当前设备。以下基础已经合入 `main`：

- #110（`add-code-image-preview`，已归档）：快照原始字节端点 `GET /api/sessions/{id}/code/snapshots/{snapshotId}/raw`、`MaxDownloadBytes = 100 MiB`、`Document.size` / `downloadable` 与 `CodeDownloadButton`。
- #213（`add-session-artifacts-folder`，已归档）：代码栏“工作区 | 临时文件夹”两个根（`/code/context?root=workspace|artifacts`），临时文件夹是 `$TMPDIR/webmux-artifacts-<uid>-<实例摘要>/<会话ID>`，上传保存到其 `uploads/` 子目录。
- #224（资源管理器文件操作）：文件树右键菜单的“下载”（`GET /code/node`）与“下载为 ZIP”（`GET /code/archive`），带已接收字节与取消。

但完整文件文档的动作栏仍只为图片和 PDF 提供“下载”；文本、普通二进制、编码不支持与超过文本上限的文件打开后只显示原因，没有就地下载入口，也没有下载前确认来源与大小的步骤；上传完成后也无法从提示直接定位到临时文件夹。本变更补齐这些缺口，不再新增根或传输接口。

## What Changes

- 完整文件文档（文本、图片、PDF、二进制、编码不支持、超过文本阅读上限）统一提供“下载文件”入口，复用既有快照 `raw` 端点；工作区与临时文件夹两个根同样适用。
- 点击后先确认文件名、来源绝对路径、大小、所属根与不可下载原因；确认后显示进度（已接收字节，可计算时显示百分比）并可取消。
- 下载失败按内容变化、快照过期、超过上限、认证、传输繁忙与网络错误给出可区分说明；409 / 410 标记文档需要刷新。
- 顶栏代码区开关文案由“代码”改为“文件”（展开时桌面显示“折叠文件”）。
- 上传成功后提供“在临时文件夹中查看”，切到临时文件夹根并打开上传文件所在目录（`uploads/`），不自动下载。
- 文本 2 MiB / 20000 行、图片与 PDF 20 MiB 预览上限保持不变；不新增根、路径输入框、批量下载、Range / 断点续传、公开分享或产物数据库；不自动扫描目录，也不从终端输出识别路径。

## Capabilities

### New Capabilities

- `terminal-file-download`：从文件视图显式下载任意常规文件、下载前确认、进度与取消、失败说明和上传后定位到临时文件夹。

### Modified Capabilities

- `terminal-diff-review`：完整文件视图增加下载入口。

## Impact

- 后端：无新增路由；`internal/codeview` 的 `Raw` 已经按快照所属根读取（`snapshotRoot`），只需补充临时文件夹根的原始字节测试。
- 前端：改动以下组件；`web/src/platform/api.ts` 的 `apiBlob(path, signal, received)` 已提供带字节计数的 Blob 读取，无需改动。
  - `web/src/components/code/CodeDocumentView.svelte`（统一下载入口）、`CodeDownloadButton.svelte`（确认与进度）。
  - `CodeWorkspace.svelte`（根状态与定位）、`TerminalHeader.svelte`（顶栏文案）。
  - `SessionTerminal.svelte` 与 `web/src/hooks/shared/upload.svelte.ts`（上传后定位）。
- 文档：`docs/usage.md` 的“代码工作区”、`docs/protocol/http-workspace.md`（如契约说明需补充）、`docs/user-stories/code-and-worktrees.md` 与 `docs/user-stories.md` 索引。
- 不涉及：持久化迁移、会话生命周期、终端 buffer 导出语义、公开分享或产物存储库。
