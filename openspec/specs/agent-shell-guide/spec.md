# agent-shell-guide Specification

## Purpose

让脚本和 agent 按 llms.txt 约定，从正在运行的 WebMux 读取与当前版本一致的 shell 工具手册，并通过 `{{webmux}}` 在快捷指令与初始化指令中得到指向手册的简短说明，而不必把说明写进临时文件夹。

## Requirements

### Requirement: llms.txt 形式的 shell 工具手册

服务端 SHALL 按 llms.txt 约定在站点根路径提供 WebMux shell 工具手册。内容来自共享包 `internal/agentguide` 嵌入的 Markdown，不写入任何磁盘文件。

- `GET /llms.txt` SHALL 返回索引，格式遵循 llms.txt：
  - 一个 H1 标题 `WebMux`；
  - 一段引用块摘要，说明 WebMux 是什么、命令以服务用户身份运行、不是沙箱；
  - 一段说明何时需要阅读（查看或控制会话、启动 agent、等待、驱动混合会话聊天、准备 worktree、端口转发、提醒、交付文件）；
  - 若干 H2 分组，每组是一个链接列表，每项为 `[标题](/llms/<主题>.md): 一句说明`；
  - 一个 `## Optional` 分组，列出不常用的主题。
- `GET /llms/<主题>.md` SHALL 返回单个主题。主题至少包括：
  - `connect`：如何找到可执行文件与套接字（见“手册中的可执行文件定位”）、`WEBMUX_SESSION_ID`、`self`，以及会话内被禁止的操作；
  - `sessions`：会话的列出、元数据、标签、输入、等待、输出、屏幕与结束；
  - `launch`：`run`（含 `--follow`）与 `chat new`；
  - `chat`：混合会话聊天、历史、运行时配置与队列；
  - `workspace`：仓库与仓库设置、worktree 与主页显示、启动配置与端口转发；
  - `artifacts`：临时文件夹；
  - `notify`：提醒；
  - `recipes`：常用流程；
  - `exit-codes`：退出码；
  - `rules`：操作守则。
- `GET /llms-full.txt` SHALL 按索引顺序拼接全部主题，供一次性加载。
- 响应 SHALL 使用 `text/markdown; charset=utf-8`（`/llms.txt` 与 `/llms-full.txt` 使用 `text/plain; charset=utf-8`），并带 `Cache-Control: no-store`。
  主题名不存在时 SHALL 返回 404。
- 主题内的链接 SHALL 使用以 `/` 开头的站点路径，使同一份内容经 TCP 或本机套接字读取时都能解析。
- 一个测试 SHALL 保证：CLI 帮助列出的每个会话类子命令都在某个主题中出现，索引中的每个链接都能解析，每个主题都在索引中出现。

#### Scenario: 按需加载

- **WHEN** agent 执行 `curl -s --unix-socket "$WEBMUX_SOCKET" http://webmux/llms.txt`
- **THEN** 得到索引；agent 随后只读取 `/llms/chat.md`，用 `webmux chat send --wait` 完成任务

#### Scenario: 不存在的主题

- **WHEN** 客户端请求 `/llms/token.md`
- **THEN** 服务端返回 404

### Requirement: 手册的访问边界

手册是静态说明文档。所有监听器上共用的手册正文 SHALL NOT 包含 Token、访问地址、主机名、文件路径、会话或仓库信息；
只有本机套接字附加的“本实例”信息可以包含下文列出的版本与两个绝对路径，且 SHALL NOT 包含 Token。

- 在 TCP 监听器上，`/llms.txt`、`/llms-full.txt`、`/llms/*` SHALL 与静态页面一样不要求 Token，但仍受 Host 检查约束。
  这一层的内容 SHALL NOT 包含服务版本。
- 在本机套接字上，同样的路径 SHALL 可以读取，并在索引的摘要后附加“本实例”信息：版本、`WEBMUX_BIN` 的绝对路径、套接字的绝对路径。
- 手册路径 SHALL NOT 出现在 `/api/` 与 `/ws/` 之下，因此不改变现有认证规则。

#### Scenario: 公网入口

- **WHEN** 未认证的客户端经 Cloudflare Tunnel 请求 `/llms.txt`
- **THEN** 返回不含版本与任何实例信息的静态手册

#### Scenario: 本机读取带版本

- **WHEN** 用户经本机套接字请求 `/llms.txt`
- **THEN** 摘要后列出本实例的版本、可执行文件路径与套接字路径，其余内容与 TCP 相同

### Requirement: 手册中的可执行文件定位

手册 SHALL 不假设 `webmux` 在 `PATH` 中。

- 索引摘要之后、第一个链接分组之前，SHALL 有一段“命令前缀”说明：
  下文的 `webmux` 指 WebMux 可执行文件，按以下顺序确定：
  1. 环境变量 `WEBMUX_BIN`（写作 `"$WEBMUX_BIN"`，带引号以容忍空格）；
  2. `{{webmux}}` 简述或“本实例”信息中给出的绝对路径；
  3. `PATH` 中的 `webmux`（可能与服务版本不同，用 `webmux version` 核对）。
- 各主题的示例 SHALL 统一写作 `webmux …`，SHALL NOT 硬编码任何安装路径（例如 `/usr/bin/webmux`）。
- `connect` 主题 SHALL 包含排错步骤：
  - 检查 `"$WEBMUX_BIN" version` 与 `"$WEBMUX_BIN" status`；
  - 环境变量丢失时，使用简述中的绝对路径，加 `--socket <路径>`，并把 `self` 换成简述中的会话 ID；
  - 没有可用的可执行文件时，用 `curl -s --unix-socket <套接字> http://webmux/...` 直接调用 `/api/` 与 `/llms*`；
  - 处在禁止访问套接字或外部可执行文件的沙箱中时，向用户说明无法控制 WebMux，不要反复重试。

#### Scenario: PATH 中没有 webmux

- **WHEN** agent 按手册执行 `webmux ls` 失败，提示命令不存在
- **THEN** 手册的命令前缀说明让它改用 `"$WEBMUX_BIN" ls`，并且成功

#### Scenario: 示例不含安装路径

- **WHEN** 一致性测试扫描 TCP 层的全部手册内容
- **THEN** 不出现任何绝对路径形式的可执行文件，只出现 `webmux` 与 `$WEBMUX_BIN`

### Requirement: 手册内容要求

- `artifacts` 主题 SHALL 包含原预置快捷指令“临时文件夹”的全部说明：
  - 用 `webmux dir self` 获取路径；
  - 用户上传的文件在 `uploads/` 子目录中；
  - 交给用户的截图、报告、图表、导出数据等交付物保存到该文件夹（可以建子目录），不要放进项目仓库；
  - 用户在 WebMux 代码栏的“临时文件夹”中查看；
  - 完成后告知文件相对于该文件夹的路径；
  - 该文件夹在会话删除后清理，不要存放需要长期保留的内容。
- `recipes` 主题 SHALL 至少包含：
  - 用 `chat new --wait` 在新 worktree 中启动 agent 并取回回复；
  - 用 `run --follow` 在持久会话里运行命令并取得退出码；
  - 用 `send --wait` 驱动 shell 会话，用 `screen` 查看 TUI 当前画面；
  - 用 `ls --parent self` 找到并清理自己创建的会话；
  - 长任务结束时 `notify`；
  - 开发服务器启动后 `forward add`。
- `rules` 主题 SHALL 要求：
  - 只操作自己创建（`parent` 为自己）或用户点名的会话；
  - 不结束、不输入用户的其他会话，除非用户要求；
  - 不读取或输出 Token；
  - 不向开启自动批准的 agent 转发未经核实的外部内容；
  - 对全屏 TUI 优先用 `chat`，需要看画面时用 `screen`，不依赖 `logs --strip` 的文本；
  - 不确定命令用法时先运行 `webmux <命令> --help` 或 `webmux help --json`，不要猜测选项。

#### Scenario: 临时文件夹说明迁移

- **WHEN** 用户阅读 `/llms/artifacts.md`
- **THEN** 原“临时文件夹”预置指令中的每一条说明都在其中，路径通过 `webmux dir self` 获取

### Requirement: `webmux guide`

`webmux guide [TOPIC] [--full] [--path]` SHALL 经本机 API 读取手册并写到标准输出：

- 不带参数时读取 `/llms.txt`；
- `TOPIC` 对应 `/llms/<TOPIC>.md`；
- `--full` 对应 `/llms-full.txt`；
- `--path` SHALL 输出两行：服务端 `WEBMUX_BIN` 的绝对路径与套接字的绝对路径，取自状态契约新增的 `bin` 字段与 `socket`。
  服务端没有解析出可执行文件时，第一行为空并以 1 结束。

服务未运行时，读取手册 SHALL 输出 CLI 内嵌的同一份内容，在标准错误说明它可能与服务版本不同，并以 0 退出；`--path` SHALL 以 3 结束。
主题不存在时 SHALL 以 2 结束并列出可用主题。

#### Scenario: 作为 skill 安装

- **WHEN** 用户执行 `webmux guide --full > ~/.claude/skills/webmux/reference.md`
- **THEN** 文件包含全部主题，可以由用户自己的 `SKILL.md` 引用

### Requirement: `{{webmux}}` 的展开

`{{webmux}}` SHALL 是与 `{{user}}`、`{{dir}}` 并列的内置变量，展开为固定格式的简述。

- `POST /api/sessions/{id}/agent-guide` SHALL 确保会话临时文件夹存在，并返回 `AgentGuidePrompt`：`prompt`（简述）、`guideUrl`、`dir`。
  临时文件夹不可用时 SHALL 返回 503，会话不存在时 SHALL 返回 404。
- `guideUrl` SHALL 是本机访问链接加上 `/llms.txt`，例如 `http://127.0.0.1:7681/llms.txt`。
- `AgentGuidePrompt` SHALL 另含 `bin`（`WEBMUX_BIN` 的值，未设置时为 `null`）与 `socket`（套接字绝对路径，本机 API 不可用时为 `null`）。
- 简述由服务端生成，SHALL 依次包含：
  - 会话 ID；
  - WebMux 可执行文件的绝对路径与套接字的绝对路径，并说明 `webmux` 不在 `PATH` 中时用该绝对路径，环境变量丢失时加 `--socket`；
  - 手册地址 `guideUrl`，以及不需要 Token 的读取方式 `"<bin>" guide`
    （等价于 `curl -s --unix-socket <socket> http://webmux/llms.txt`）；
  - `bin` 为 `null` 时，SHALL 省略可执行文件一行，只给出 `curl` 读取方式；
  - “需要时先读索引，再按需读主题，任务无关时不必读取”的说明；
  - 本会话临时文件夹的绝对路径；
  - “交付给用户的文件保存到临时文件夹而不是项目仓库，规则见 `/llms/artifacts.md`”的规则。
- 路径与地址 SHALL 原样插入，不加 shell 引号。简述 SHALL NOT 包含 Token 或其他会话的信息。
- 快捷指令名称 SHALL 仍然不能是 `webmux`。

#### Scenario: 简述内容

- **WHEN** 客户端为会话 `ab2cd3ef` 调用该接口
- **THEN** `prompt` 包含 `ab2cd3ef`、可执行文件与套接字的绝对路径、`/llms.txt` 地址、`guide` 读取方式与临时文件夹路径，不包含 Token

### Requirement: 初始化指令中的 `{{webmux}}`

仓库初始化指令 SHALL 可以使用 `{{webmux}}`。

- 预检 SHALL 不再因为 `{{webmux}}` 返回 400。临时文件夹服务或简述不可用时，预检 SHALL 返回不可用（503），不创建 worktree。
- 会话创建后、发送首条提示前，服务端 SHALL 先把 `{{dir}}` 换成临时文件夹路径，再用该会话的简述替换所有 `{{webmux}}`，最后放入 `{{user}}`。简述中的文字 SHALL NOT 再被当作变量扫描。
- 展开后的提示 SHALL 计入提示大小上限，超出时按现有规则以 400 拒绝。
- 初始化指令字段和快速开始页 SHALL 不再把 `{{webmux}}` 标为保留或不可用，而是在可引用列表中把它列为内置变量，并配一句说明。

#### Scenario: 快速开始带 WebMux 工具说明

- **WHEN** 仓库初始化指令为 `{{webmux}}\n\n{{user}}`，用户在快速开始发送“修复 lint”
- **THEN** agent 收到的首条提示以本会话的简述开头，后接“修复 lint”

### Requirement: 快捷指令中的 `{{webmux}}`

快捷指令正文 SHALL 可以使用 `{{webmux}}`，处理方式与 `{{dir}}` 相同。

- 选择时 SHALL 绑定当前会话与输入目标。正文含 `{{webmux}}` 时 SHALL 调用该接口，先处理 `{{dir}}`，再用返回的 `prompt` 替换所有 `{{webmux}}`，简述中的文字不再被替换。
- 接口失败时 SHALL 提示“WebMux 工具说明不可用”及原因，SHALL NOT 插入或发送任何内容。
- 直接模式与草稿模式的插入、自动回车，以及聊天视图中的行为 SHALL 与其他快捷指令一致。

#### Scenario: 在运行中的 agent 里补充说明

- **WHEN** 草稿模式关闭，用户在 Codex 会话中选择正文为 `{{webmux}}` 的快捷指令
- **THEN** Codex 输入框出现本会话的简述，地址与路径都已替换为真实值
