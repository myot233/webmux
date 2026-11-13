## ADDED Requirements

### Requirement: Devin CLI 预置混合配置

新安装的预置 SHALL 增加 `Devin CLI`，图标为 `devin`，argv 在既有启动写法上追加 `--permission-mode dangerous`，并设置 `hybrid: true`。该预置 SHALL 通过 `hybrid-agent-profiles` 校验，`hybridSupport` 为 `{supported: true, agent: "devin"}` 且不包含连接器。

`--permission-mode dangerous` SHALL 作为新预置的显式自动批准行为呈现；用户可编辑配置移除它。其他预置与 JSON 结构 SHALL 保持不变。

#### Scenario: 新安装的 Devin 预置

- **WHEN** 服务首次启动且 `launch-configs.json` 不存在，服务端 `$SHELL` 为 Bash
- **THEN** Devin CLI 预置的图标为 `devin`，argv 为 `["bash","-ic","exec -- \"$@\"","webmux","devin","--permission-mode","dangerous"]`，`hybrid` 为 `true`，`hybridSupport.agent` 为 `devin`

#### Scenario: 从 Devin 预置创建会话

- **WHEN** 用户在新安装上选择 Devin CLI 预置
- **THEN** 可选择从终端或聊天开始，聊天端按配置推导使用 `mode=bypass`

### Requirement: 已有 Devin 配置不被迁移

已存在的 `launch-configs.json` SHALL NOT 因新增 Devin 档案而被改写。升级前已保存的 `devin` 配置，其命令、`hybrid` 字段与其他内容 SHALL 保持原值；没有 `--permission-mode` 或没有勾选混合模式的旧配置 SHALL NOT 自动补上这些设置。

旧 `devin` 配置未勾选混合模式时，用户手动勾选后 SHALL 按新档案校验并启用，无需删除或重建配置。

#### Scenario: 升级后旧配置保持原样

- **WHEN** 升级前已存在命令为 `devin`、未设置 `hybrid` 的配置
- **THEN** 升级后命令仍不含 `--permission-mode`，`hybrid` 仍为 `false`，状态文件不被重写

#### Scenario: 手动启用旧配置

- **WHEN** 用户对升级前保存的 `devin` 配置手动勾选混合模式并保存
- **THEN** 保存成功，`hybridSupport.agent` 为 `devin`，命令参数保持用户保存的内容
