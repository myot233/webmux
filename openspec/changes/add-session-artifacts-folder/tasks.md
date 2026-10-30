## 1. 后端：`internal/artifacts` 服务

- [x] 1.1 新建 `internal/artifacts`：`Options{TempDir, StateDir, Manager, Retain, Logger}`、`New`、`Close`；实例根 `webmux-artifacts-<uid>-<hash8(canonical state_dir)>`（规范化 `TempDir` 与 `state_dir`）；会话 ID 字符集校验
- [x] 1.2 实现 `Ensure(id) (string, error)`、`UploadsDir(id)`：`Mkdir(0700)`，已存在时 `Lstat` 校验目录/非链接/属主为 euid/`mode&0077==0`；会话不在 `Manager` 中返回 `ErrNotFound`，校验失败返回 `ErrUnavailable`
- [x] 1.3 实现 `Remove(id)`、`Sweep()`（对账：删除不在 `Manager` 且 `Retain` 为假的合法名直接子目录，不跟随链接）与 `Manager.Subscribe()` 事件循环（`removed` → 按 `Retain` 删除；`Slow` → 重新订阅并对账）
- [x] 1.4 实现 `UploadPath(sessionID, path)`：只接受该会话 `uploads/` 下的直接子项普通文件，路径各级非符号链接；删除 `pathutil.UploadDirPrefix`、`pathutil.UploadPath` 及其测试
- [x] 1.5 在 `internal/archtest/layers_test.go` 登记 `internal/artifacts` 为 service；在 `persistence_inventory.json` 登记新的 `os.Mkdir`/`os.RemoveAll` 入口（owner `user-files`）并更新 `docs/persistence.md` 例外说明
- [x] 1.6 `internal/artifacts` 单元测试：实例隔离、预置符号链接/他人目录被拒、删除与 `Retain` 保留、对账只删合法名目录、`UploadPath` 拒绝跨会话与链接路径

## 2. 后端：上传与聊天附件

- [x] 2.1 `files.StoreUpload(dir, filename, src, limit)`：在 `uploads/` 内 `O_CREAT|O_EXCL|O_NOFOLLOW` 0600 创建，冲突时追加 ` (2)`、` (3)`（扩展名前）；`Upload.Discard` 只删除本文件；更新持久化清单中 `Discard` 的入口
- [x] 2.2 `api.upload` 先调用 `Artifacts.UploadsDir(id)`，失败按 `api/errors.go` 映射（404/503）；更新 `upload_test.go`（路径断言、同名序号、取消只删本文件）
- [x] 2.3 `internal/hybrid/attachments.go` 的 `resolveAttachments`/`readAttachment` 改用注入的 `artifacts` 按会话校验；更新 `attachments_test.go`，新增跨会话路径被拒用例

## 3. 后端：代码视图第二种根

- [x] 3.1 `codeview` 绑定键改为 `(sessionID, rootKind)`，`Context(ctx, id, root)`；`withRoot` 按 `contextId` 在会话的两种绑定中查找；`sweep` 丢弃两种绑定
- [x] 3.2 实现 artifacts 绑定：根为会话文件夹规范路径，`Kind "artifacts"`、`Browse true`、`Diff/Ignore false`，不运行 git；目录缺失返回 `ErrNotFound`；`Changes`/`Diff` 对其返回 `ErrUnavailable`
- [x] 3.3 `internal/api/code.go`：`GET code/context` 解析 `root`（缺省/`workspace`/`artifacts`，其他 400）；`codeview` 测试覆盖 artifacts 树/文件/搜索、链接不跟随、修改/Diff 被拒

## 4. 后端：API、设置与装配

- [x] 4.1 `contract.ArtifactsFolder{Path}`，`POST /api/sessions/{id}/artifacts`（`internal/api` 新路由，200/404/503，继承认证与 Origin 校验）；`removeHybridRecord` 成功后调用 `Artifacts.Remove(id)`；`api.Deps` 新增 `Artifacts`，`newServer` 校验非空
- [x] 4.2 设置新增 `artifactPrompt`：`settings.Values.ArtifactPrompt *string`（`omitempty`）、`validateValues`（非空、≤4096 UTF-16、含 `{dir}`）、`normalize`（CRLF/CR→LF、去末尾换行）、`apply`、`cloneValues`；`contract.SettingsValues`、`wire/settings.go` 的 `shapes` 与转换
- [x] 4.3 `cmd/webmux/main.go`：构造 `artifacts.New`（`Retain` = 混合记录存在或记录服务不可用），注入 `files`/`codeview`/`hybrid`/`api.Deps`；在 hybrid 打开后、开始服务前调用 `Sweep()`（混合记录不可读时跳过并告警）；关闭时 `Close()`；修正 `internal/api` 测试中的构造调用
- [x] 4.4 API 测试：确保接口幂等/404/认证与 Origin、删除普通会话后文件夹被删、混合会话关闭保留与删除记录后清理、设置 `artifactPrompt` 校验与 null 不落盘
- [x] 4.5 更新 `wire_normalizer_test.go` 归一化实例根与会话 ID，新增 artifacts 与 artifacts 代码上下文快照，运行 `go test ./internal/api -run TestWireSnapshots -update` 重录上传与设置快照

## 5. 前端：契约与设置

- [x] 5.1 `schemas.ts`：`ArtifactsFolderSchema`、`CodeContextSchema.rootKind` 增加 `artifacts`、`SettingsValuesSchema.artifactPrompt`（与服务端相同约束）；`api.ts` 增加确保接口调用
- [x] 5.2 新建 `web/src/artifactPrompt.ts`：内置模板、`renderArtifactPrompt(template, dir)`、模板规范化与校验；Node 单元测试覆盖多处 `{dir}` 替换与校验
- [x] 5.3 `settings-intent.ts` 对 `artifactPrompt` 整体覆盖重放；`pages/Settings.svelte` 快捷指令卡片内追加“临时文件夹提示词”区块（等宽多行输入、保存、恢复默认、缺少 `{dir}` 提示、设置不可用时禁用并保留输入）

## 6. 前端：快捷指令菜单与代码栏

- [x] 6.1 `QuickCommandMenu.svelte` 新增 `artifact` 回调与固定项（去向标注之后、分隔线隔开、受 `blocked` 规则约束、空状态下仍显示）
- [x] 6.2 `SessionTerminal.svelte` 新增 `insertArtifactPrompt()`：选择时 `drafts.begin(draft)` 绑定会话与目标，确保接口后用同一 operation `insertText`，不回车；失败提示“临时文件夹不可用”；清除软键修饰键
- [x] 6.3 `CodePanel.svelte`/`CodeNavigator.svelte`：“工作区 | 临时文件夹”根切换（复用 `Tabs`），切换先确保再取上下文；artifacts 根隐藏“修改”，显示根路径；标签记录 `rootKind` 并标注“临时文件夹”；确保失败显示原因与重试
- [x] 6.4 浏览器测试：`QuickCommandMenu` 固定项（空状态、禁用、草稿模式可用）、`CodePanel` 根切换与修改页签隐藏、设置区块保存/恢复默认；运行 `pnpm --dir web check:arch`

## 7. 端到端与文档

- [x] 7.1 Playwright：上传后文件出现在代码栏“临时文件夹”的 `uploads/`；⚡ 固定项在 shell 中插入含真实路径的文字且未执行；删除会话后确保接口返回 404
- [x] 7.2 用户故事：修改 US-E05（保存位置与随会话删除清理），新增 US-I18（代码栏查看临时文件夹）与 US-G06（插入临时文件夹提示词），同步 `docs/user-stories.md` 与 `coverage.md` 索引及依据链接
- [x] 7.3 `docs/protocol` 中设置键与新接口说明；运行 `make docs-check`、`make check`、`make test-fast` 与相关 E2E
