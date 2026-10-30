# cli-config Specification

## Purpose

让客户端（CLI、托盘）在不改写用户手写配置的前提下定位、检查和初始化 WebMux 配置，并以与服务端完全一致的规则得到生效配置，避免各客户端各自解析 TOML。

## Requirements

### Requirement: 定位与查看生效配置

`webmux config path` SHALL 报告配置路径及是否存在。

`config show` 有两种模式：

- 默认（磁盘）：SHALL 只读解析磁盘 TOML 与专用覆盖，输出 view configured、字段值/来源和各存储健康摘要；SHALL NOT 初始化、加锁、chmod 或修复文件。
- `--live`：SHALL 通过同 UID socket 读取已验证运行实例的内存快照，输出 view effective、实例身份、实际监听地址与观察时间；不可达 SHALL 报错且不回退磁盘。

两种模式 SHALL 都支持人类输出及单个 JSON 对象，且不输出凭据正文、启动命令/env、推送订阅或快捷指令正文。

#### Scenario: 显式默认值

- **WHEN** TOML 显式写入与默认值相同的值
- **THEN** 字段来源仍为 toml

#### Scenario: 运行后修改磁盘

- **WHEN** 操作者修改 TOML 后尚未重启
- **THEN** 默认查看新磁盘值，--live 查看进程已采用的旧值

#### Scenario: 活进程不可达

- **WHEN** 使用 --live 但没有可验证实例或 socket 不可达
- **THEN** 退出非零且不输出 configured 冒充 effective

#### Scenario: 无配置文件

- **WHEN** 默认配置文件不存在且用户运行 config show
- **THEN** 输出默认配置和 default 来源，不创建配置文件或状态目录

#### Scenario: 不输出 Token

- **WHEN** 配置或运行进程包含登录 token 或隧道 token
- **THEN** 诊断仅报告凭据是否存在，不输出凭据内容

### Requirement: 校验配置

`webmux config validate` SHALL 对配置执行与服务启动相同的解析与校验，成功时退出码 0；失败时向标准错误输出可定位的原因（TOML 语法错误的行列、非法字段值、预设冲突等），退出码 1。`--json` 时标准输出为包含 `v`、`ok`、`errors` 数组的 JSON 对象。

校验 SHALL NOT 修改任何文件，也 SHALL NOT 要求服务正在运行。

#### Scenario: 语法错误

- **WHEN** 配置文件第 3 行缺少引号
- **THEN** `webmux config validate` 输出包含行号的错误并以退出码 1 结束

### Requirement: 初始化配置

`webmux config init` SHALL 在配置路径不存在时，以内置的示例配置为模板创建配置文件：生成一个密码学安全的随机 Token（至少 256 位熵）写入 `token`，`--listen <addr>` 指定时同时写入 `listen`；文件权限 SHALL 为 0600，父目录不存在时 SHALL 以 0700 创建。命令输出 SHALL 只包含文件路径，SHALL NOT 输出 Token。

目标文件已存在时 SHALL 拒绝并以退出码 1 结束；传入 `--force` 时 SHALL 先将原文件保留为同目录下的 `.bak`，再创建新文件。

客户端 SHALL NOT 以程序方式修改已有配置文件中的单个字段；修改配置由用户在编辑器中完成。

#### Scenario: 首次初始化

- **WHEN** 配置文件不存在，用户执行 `webmux config init`
- **THEN** 生成 0600 的配置文件，其中 `token` 为新随机值、其余项与示例配置一致；标准输出只有文件路径

#### Scenario: 已存在时拒绝

- **WHEN** 配置文件已存在，用户执行 `webmux config init`
- **THEN** 原文件内容不变，命令提示使用 `--force` 并以退出码 1 结束

#### Scenario: 强制覆盖

- **WHEN** 配置文件已存在，用户执行 `webmux config init --force`
- **THEN** 原内容保存在 `config.toml.bak`，新文件包含新 Token
