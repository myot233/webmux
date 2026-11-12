# session-metadata Specification

## Purpose

让脚本和 agent 一次取得会话的完整背景（仓库、worktree、启动配置、混合状态、链接、父子会话与标签），并能记录由谁创建、按标签分组，用来编排多个会话。

## Requirements

### Requirement: 会话详情接口

`GET /api/sessions/{id}/details` SHALL 返回 `SessionDetails`，把分散在多个接口中的会话元数据集中到一处。各字段如下：

- `session`：`SessionInfo`。
- `link`：不含 Token 的本机会话链接，服务不知道本机链接时为 `null`。
- `repository`：`{id, name, path}`；`worktree`：`{id, branch, path, main}`。会话没有工作区关联时，两者都为 `null`。
- `launchConfig`：`{id, name, icon, hybrid}`。会话不是用启动配置创建的，或该配置已被删除时为 `null`；删除时另设 `launchConfigMissing: true`。
- `hybrid`：混合会话的 `HybridRecord`，包括 agent、所在端、agent 会话 ID 与状态；其他会话为 `null`。
- `artifactsDir`：临时文件夹已存在时为其绝对路径，否则为 `null`。该接口 SHALL NOT 创建文件夹。
- `forwards`：来源为该会话的端口转发。
- `parentId` 与 `labels`：同 `SessionInfo`。`children`：`parentId` 为该会话的会话 ID 列表，按创建时间排序。

其他要求：

- 字段 SHALL NOT 包含 Token、启动环境变量的值，或其他会话的输出。
- 会话不存在时 SHALL 返回 404。
- 新增的契约类型 SHALL 同步到 `internal/contract`、`internal/wire` 与 `web/src/contracts/schemas.ts`，并重新录制 wire 快照。

#### Scenario: agent 了解自己的处境

- **WHEN** 在 Claude Code 混合会话中，agent 请求自己会话的详情
- **THEN** 响应包含仓库与 worktree 分支、启动配置名称“Claude Code”、`hybrid.side` 和 agent 会话 ID，不包含任何环境变量值

#### Scenario: 不创建临时文件夹

- **WHEN** 会话从未使用过临时文件夹，客户端请求详情
- **THEN** `artifactsDir` 为 `null`，临时根下没有为该会话新建目录

### Requirement: webmux get

`webmux get [SESSION] [--json] [--field PATH]` SHALL 输出会话详情。

- 在会话内调用时，`SESSION` 默认为 `self`；不在会话内时 SHALL 必填，缺少时以 2 结束。
- 默认 SHALL 以对齐的键值形式输出常用字段：ID、名称、状态、命令、目录、仓库与分支、启动配置、所在端、临时文件夹、
  链接、父会话、子会话数、标签、监听端口。
- `--json` SHALL 原样输出 `SessionDetails`。
- `--field PATH` SHALL 只输出一个字段的值，`PATH` 为点分路径（例如 `worktree.branch`）。字符串原样输出，不带引号；
  对象或数组输出 JSON；字段为 `null` 时输出空行并以 1 结束。这样不依赖 `jq` 也能在脚本中取值。

#### Scenario: 不用 jq 取分支

- **WHEN** 会话内的脚本执行 `branch=$(webmux get --field worktree.branch)`
- **THEN** 变量值为当前会话所在 worktree 的分支名

#### Scenario: 会话外缺少 ID

- **WHEN** 用户在普通终端执行 `webmux get`
- **THEN** 退出码 2，提示需要会话 ID

### Requirement: 父会话

从一个会话内的进程经本机 API 创建的会话，SHALL 自动记录 `parentId` 为调用者所在的会话。

- 适用的创建入口：`POST /api/sessions` 与 `POST /api/quick-starts`。服务端 SHALL 用套接字对端 PID 判断调用者所在的会话：
  Linux 优先读取进程环境中的 `WEBMUX_SESSION_ID`，再沿进程树查找会话主进程；macOS 只沿进程树查找。
- 经 TCP（浏览器）创建，或调用者不在任何会话内时，SHALL 不设置 `parentId`。请求体 SHALL NOT 能指定或伪造 `parentId`。
- `parentId` SHALL 出现在 `SessionInfo` 中（为空时省略），经 `/ws/sessions` 实时同步。父会话被移除后，子会话的 `parentId` 保持不变。
- `parentId` 只保存在内存中，与会话同生命周期；混合会话恢复后 SHALL 不再带有它。

#### Scenario: agent 创建的子会话

- **WHEN** 会话 `p1` 中的 agent 执行 `webmux run --detach -- make test`
- **THEN** 新会话的 `parentId` 为 `p1`，`webmux ls --parent p1` 列出它

#### Scenario: 浏览器创建

- **WHEN** 用户在 Web 端新建会话
- **THEN** 该会话没有 `parentId`

### Requirement: 会话标签

会话 SHALL 支持用户定义的标签（键值对），用于脚本标记与筛选会话。

- 键 SHALL 匹配 `[a-z0-9][a-z0-9._-]{0,62}`，值 SHALL 不超过 256 个字符且不含控制字符，每个会话 SHALL 最多 16 个标签。不合法时 SHALL 以 400 拒绝且不做任何修改。
- 创建请求 SHALL 接受可选的 `labels` 对象；`PATCH /api/sessions/{id}/labels` SHALL 接受 `{set, remove}`，在一次操作中原子地完成设置与删除，并返回 `SessionInfo`。
- `labels` SHALL 出现在 `SessionInfo` 中（为空时省略），经 `/ws/sessions` 实时同步。标签只保存在内存中，与会话同生命周期。
- CLI：
  - `webmux run --label`、`webmux chat new --label` 在创建时设置；
  - `webmux label SESSION KEY=VALUE... KEY-...` 设置或删除（以 `-` 结尾表示删除）；
  - `webmux ls` 新增 `--label KEY=VALUE`（可重复，全部满足）、`--label KEY`（存在即可）、`--parent SESSION` 与 `--status STATE` 筛选，
    `--json` 输出筛选后的数组。
- 本变更不在 Web 端显示标签。

#### Scenario: 批量清理一次任务的会话

- **WHEN** 编排脚本以 `--label run=42` 启动了三个会话，完成后执行 `webmux ls --label run=42 --json | jq -r '.[].id' | xargs -n1 webmux rm`
- **THEN** 三个会话被移除，其他会话不受影响

#### Scenario: 非法标签

- **WHEN** 客户端设置键为 `Task Name` 的标签
- **THEN** 服务端返回 400，会话标签保持不变
