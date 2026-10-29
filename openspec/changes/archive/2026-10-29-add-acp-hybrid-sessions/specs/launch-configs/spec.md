## ADDED Requirements

### Requirement: 启动配置的混合模式开关

启动配置 SHALL 支持可选的布尔字段 `hybrid`，缺省为 `false`。

- 保存时为 `false` 的字段 SHALL 不写入文件。
- `launch-configs.json` 的版本 SHALL 保持 2；不含该字段的已有文件 SHALL 照常读取，所有配置视为未勾选。

`hybrid` 为 `true` 时，服务端 SHALL 按 `hybrid-agent-profiles` 校验 argv，校验不通过时 SHALL 返回 400 并给出原因，已有数据不变。

新安装预置的 OpenCode 与 omp 配置 SHALL 为 `hybrid: true`，其 argv（含各自的自动批准参数）SHALL 通过 `hybrid-agent-profiles` 校验。修改已勾选配置的 argv 使其不再受支持时，保存 SHALL 返回 400，用户须先取消勾选。

新安装预置的 agent 配置 SHALL 带上各自的自动批准参数，使工具调用无需逐次批准：Claude Code `--dangerously-skip-permissions`、Codex CLI `--dangerously-bypass-approvals-and-sandbox`、Gemini CLI `--yolo`、OpenCode `--auto`、omp `--auto-approve`。Shell 配置 SHALL 不变。已存在的 `launch-configs.json` SHALL NOT 被改写。

#### Scenario: 新安装的预置配置

- **WHEN** 服务首次启动且 `launch-configs.json` 不存在
- **THEN** 预置的 omp 配置 argv 为 `["bash","-ic","exec -- \"$@\"","webmux","omp","--auto-approve"]`，`hybrid` 为 `true`，`hybridSupport` 为 `{supported: true, agent: "omp"}`；OpenCode 配置同样为混合配置；Claude Code、Codex CLI、Gemini CLI 带自动批准参数且 `hybrid` 为 `false`

#### Scenario: 已有安装不改写

- **WHEN** 升级前已存在 `launch-configs.json`
- **THEN** 服务不修改其中任何配置，包括 `hybrid` 与命令参数

#### Scenario: 勾选 omp 配置

- **WHEN** 用户对自己的 omp 配置把 `hybrid` 改为 `true` 并保存
- **THEN** 保存成功，摘要中 `hybrid` 为 `true`，`hybridSupport` 为 `{supported: true, agent: "omp"}`

#### Scenario: 旧文件

- **WHEN** 升级前保存的 `launch-configs.json` 不含 `hybrid` 字段
- **THEN** 服务正常读取，所有配置的 `hybrid` 为 `false`，文件不被改写

#### Scenario: 不支持的命令

- **WHEN** 用户对 Claude Code 配置提交 `hybrid: true`
- **THEN** 服务端返回 400，原因说明当前版本只支持 OpenCode 与 omp

### Requirement: 启动配置的混合支持信息

启动配置的完整配置、摘要、创建与更新请求 SHALL 包含 `hybrid`。完整配置与摘要 SHALL 额外包含只读的 `hybridSupport: {supported, agent?, reason?}`，由服务端按当前 argv 计算；请求体中提供 `hybridSupport` SHALL 视为未知字段而返回 400。

#### Scenario: 请求体携带只读字段

- **WHEN** 客户端创建或更新启动配置时在请求体中提供 `hybridSupport`
- **THEN** 服务端返回 400，已有数据不变

### Requirement: 设置页的混合模式勾选框

设置页的启动配置编辑器 SHALL 提供“混合模式（聊天 ↔ 终端）”勾选框。

- 勾选框是否可用 SHALL 由服务端判定，浏览器 SHALL NOT 复制档案规则。打开已有配置时使用其 `hybridSupport`；编辑命令时，设置页 SHALL 去抖调用 `POST /api/launch-configs/hybrid-support {cmd}`（已认证，只读，返回同样的 `hybridSupport`）。
- 不可用时，勾选框 SHALL 禁用，并在旁边显示原因。
- 启动配置列表 SHALL 为已勾选的配置显示“混合”标记。

#### Scenario: Shell 配置

- **WHEN** 用户打开 Shell 配置的编辑器
- **THEN** 混合模式勾选框不可用，并显示“未识别到支持的 Agent”
