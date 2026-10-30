## Why

用户与 CLI agent 交换的文件目前没有固定落点：

- 上传的截图、素材散落在 `$TMPDIR/webmux-upload-*` 的一次性目录里。
- agent 生成的报告、图表、导出数据要么写进项目仓库污染工作树，要么写到用户在 WebMux 里看不到的位置。

需要一个按会话隔离、可在代码栏直接查看、并能一键告诉 agent 如何使用的“交互物临时文件夹”。

## What Changes

- 每个会话拥有一个服务端临时文件夹（交互物临时文件夹），位于系统临时目录下、按 WebMux 实例和会话 ID 隔离，权限 0700，按需创建；会话被删除（普通会话 `remove`、混合会话删除记录）后清理，服务启动时清扫无主的残留文件夹。
- 新增 `POST /api/sessions/{id}/artifacts`，确保文件夹存在并返回其绝对路径。
- **BREAKING**（服务端路径约定）：终端与聊天的文件上传改为保存到该会话临时文件夹的 `uploads/` 子目录，同名文件以 ` (2)` 等后缀区分；不再创建 `webmux-upload-*` 目录。已上传文件改为随会话删除而清理，不再保留到进程结束。聊天附件只接受本会话 `uploads/` 中的文件。
- 代码栏新增根切换：“工作区”（现有行为）与“临时文件夹”；临时文件夹根只读浏览、搜索、预览与变化检测，不提供 Git 修改与 Diff。
- 终端 ⚡ 快捷指令菜单顶部新增固定项“插入临时文件夹提示词”：确保文件夹存在后，把提示词模板中的 `{dir}` 替换为真实路径，按草稿/直接模式插入，永不自动回车。
- 服务端设置新增 `artifactPrompt`（`null` 使用内置模板），在设置页快捷指令卡片中编辑与恢复默认，跨设备同步。

## Capabilities

### New Capabilities

- `session-artifacts`: 会话交互物临时文件夹的位置、安全、生命周期与清扫；确保接口；上传存储布局与聊天附件校验；代码栏临时文件夹根；提示词模板与 ⚡ 菜单插入行为。

### Modified Capabilities

- `terminal-quick-commands`: 快捷指令菜单增加不可删除、不可排序的固定项“插入临时文件夹提示词”，空状态与禁用规则随之调整。
- `server-settings-sync`: 设置文档新增同步键 `artifactPrompt` 及其校验与默认值。
- `terminal-input-composer`: 上传结果路径的存储位置与清理边界改由 `session-artifacts` 约定。

## Impact

- 后端：
  - 新增服务包 `internal/artifacts`，登记到 `internal/archtest` 层清单与持久化清单。
  - `internal/files`：`StoreUpload` 目标目录与同名处理；`internal/pathutil`：移除 `UploadDirPrefix`/`UploadPath`。
  - `internal/hybrid/attachments.go`：附件校验改为按会话；`internal/codeview`：第二种根绑定。
  - `internal/settings`、`internal/contract`、`internal/wire`：新设置键与契约。
  - `internal/api`：新路由、`code/context?root=`、混合记录删除后清理；`cmd/webmux`：构造与启动清扫。
- 前端：`schemas.ts`（`CodeContextSchema.rootKind`、`SettingsValuesSchema.artifactPrompt`、新确保接口响应）、`CodePanel.svelte`/`CodeNavigator.svelte`、`QuickCommandMenu.svelte`、`SessionTerminal.svelte`、`pages/Settings.svelte`、`settings-intent.ts`。
- 协议快照：`internal/api/testdata/wire` 的上传、设置与代码上下文快照需重新录制。
- 用户故事：修改 US-E05，新增代码栏与快捷指令组的故事。
- 不涉及：聊天视图（混合会话聊天端）不提供该快捷入口；不向会话环境注入变量。
