# acp-connector-availability Specification

## Purpose

TBD - created by archiving change add-codex-hybrid-profile. Update Purpose after archive.

## Requirements

### Requirement: 第三方 ACP 连接器的声明

档案 SHALL 说明 ACP 进程由谁提供：

- 内置子命令：ACP 由 agent 自己的可执行文件提供（OpenCode 的 `opencode acp`、omp 的 `omp acp`），SHALL NOT 声明连接器；
- 第三方连接器：ACP 由另一个可执行文件提供（Codex 的 `codex-acp`），档案 SHALL 声明连接器的
  可执行文件名、显示名称、安装命令与说明链接。

Codex 档案的连接器 SHALL 为：可执行文件 `codex-acp`，显示名称 “Codex ACP 适配器”，
安装命令 `npm install -g @agentclientprotocol/codex-acp`，说明链接 `https://github.com/agentclientprotocol/codex-acp`。

webmux SHALL NOT 自动安装连接器或其运行时（例如 Node），也 SHALL NOT 以 `npx` 等方式临时下载执行。

#### Scenario: 内置子命令不检查连接器

- **WHEN** omp 混合会话启动 ACP 进程
- **THEN** 系统不执行连接器检查，直接按 `omp acp` 启动

#### Scenario: 不自动安装

- **WHEN** 服务 PATH 中没有 `codex-acp`，用户从聊天开始 Codex 混合会话
- **THEN** 系统不执行任何安装或下载命令，只返回未安装的结果

### Requirement: 启动 ACP 进程之前检查连接器

对声明了连接器的档案，系统 SHALL 在每一次启动 ACP 进程之前检查连接器是否已安装，包括：
资格探测、从聊天开始、切到聊天、恢复到聊天，以及 ACP 进程意外退出后的自动重连。

检查 SHALL 使用与实际启动相同的写法与环境：

- 直接执行：在会话环境（启动配置的环境变量覆盖服务进程环境）的 `PATH` 中查找连接器可执行文件，
  查找规则 SHALL 与实际启动 ACP 进程时一致；
- bashrc 包装：以同样的包装执行 `command -v -- <连接器>`，以退出码判定是否找到；它的 stdout/stderr SHALL 被丢弃，
  单次检查 SHALL 在 10 秒内结束，超时视为检查失败而非未安装。

检查结果为未安装时，系统 SHALL NOT 启动 ACP 进程，并以原因码 `connector_missing` 结束该操作。
检查本身失败（例如超时）时，系统 SHALL 按原有的“启动 agent 失败”处理。

检查通过后实际启动仍报告可执行文件不存在时（直接执行的 `ENOENT`，或 bashrc 包装在 `initialize` 之前以退出码 127 结束），
系统 SHALL 同样按 `connector_missing` 处理。

#### Scenario: 配置的环境变量改变了 PATH

- **WHEN** 启动配置设置 `PATH=/opt/codex/bin:/usr/bin`，`codex-acp` 只在 `/opt/codex/bin` 中
- **THEN** 检查判定为已安装，ACP 进程正常启动

#### Scenario: 只在 .bashrc 中加入 PATH

- **WHEN** 配置使用 bashrc 包装，`codex-acp` 所在目录只由 `~/.bashrc` 加入 PATH
- **THEN** 检查判定为已安装；`.bashrc` 输出的欢迎语不影响判定

#### Scenario: 检查与启动之间被卸载

- **WHEN** 检查通过后、启动之前 `codex-acp` 被删除，直接执行时启动返回 `ENOENT`
- **THEN** 操作以 `connector_missing` 结束，而不是“启动 agent 失败”

### Requirement: 未安装时终端端照常使用

连接器未安装 SHALL NOT 影响混合会话的终端端：从终端开始的会话 SHALL 照常在 PTY 中启动 TUI，
切到终端、恢复到终端 SHALL 照常执行。保存勾选了混合模式的启动配置 SHALL NOT 检查连接器。

#### Scenario: 从终端开始

- **WHEN** 服务 PATH 中没有 `codex-acp`，用户以 Codex 混合配置从终端开始
- **THEN** 会话创建成功，TUI 正常运行，会话页的“聊天”按钮不可用并显示未安装提示

### Requirement: 连接器信息与原因码的负载

`connector_missing` 出现在以下负载中时，SHALL 同时携带连接器信息 `connector: {executable, title, installCommand, docsUrl}`：

- 会话信息中的聊天资格 `{available: false, reason: "connector_missing", connector}`；
- 新建会话、切换、恢复请求失败时的 409 冲突体 `{error, reason: "connector_missing", connector}`；
- 自动重连失败后会话进入 `interrupted` 时发布的通知。

连接器信息 SHALL 只来自档案，SHALL NOT 包含查找到的路径、PATH 内容或环境变量。

#### Scenario: 冲突体

- **WHEN** 客户端以 Codex 混合配置和 `startSide: "acp"` 创建会话，连接器未安装
- **THEN** 服务端返回 409，`reason` 为 `connector_missing`，`connector.executable` 为 `codex-acp`，不创建会话，不写入混合会话记录

### Requirement: 未安装后重新检查

聊天资格为 `connector_missing` 时，系统 SHALL 每 30 秒重新检查一次，直到判定为已安装或终端端的 agent 退出；
执行切换之前 SHALL 再检查一次。判定为已安装后，系统 SHALL 继续原有的资格判定，不需要用户刷新页面或重启会话。

#### Scenario: 安装后自动恢复可用

- **WHEN** 会话页显示未安装提示，用户在服务器上安装 `codex-acp`
- **THEN** 30 秒内“聊天”按钮的原因变为后续资格判定的结果（例如“正在核对会话”），无需刷新页面

### Requirement: 界面的安装提示

界面 SHALL 在以下位置提示用户安装连接器，文字包含连接器显示名称、可执行文件名与安装命令，安装命令 SHALL 可一键复制，并提供说明链接：

- 会话页：“聊天”按钮禁用，提示为“未安装 Codex ACP 适配器（codex-acp）”；会话页另显示可关闭的提示条，
  内容为安装命令与“安装后会自动重新检测”；
- 新建会话弹层：选中声明了连接器的混合配置时，在“从聊天开始”旁说明“聊天端需要另外安装 `<可执行文件>`”；
  从聊天开始失败且原因为 `connector_missing` 时，弹层 SHALL 保持打开、保留输入，显示安装命令，并提供“改为从终端开始”；
- 聊天视图：恢复或自动重连因 `connector_missing` 失败时，显示同样的安装提示与“切到终端”按钮；
- 设置页：混合模式勾选框可用且档案声明了连接器时，在勾选框旁说明“聊天端需要另外安装 `<可执行文件>`”及安装命令。

#### Scenario: 新建会话从聊天开始失败

- **WHEN** 用户在新建会话弹层选择 Codex 混合配置、从聊天开始并提交，连接器未安装
- **THEN** 弹层不关闭，显示“未安装 Codex ACP 适配器（codex-acp）”、可复制的安装命令与“改为从终端开始”

#### Scenario: 设置页提前提醒

- **WHEN** 用户打开预置的 Codex CLI 启动配置编辑器
- **THEN** 混合模式勾选框可用，旁边说明聊天端需要另外安装 `codex-acp` 并给出安装命令
