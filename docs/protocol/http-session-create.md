# HTTP API：创建会话

`POST /api/sessions` 是唯一的会话创建接口：一次请求可以同时新建 worktree、选择起始视图、应用 agent 配置并发送首条提示。
Web 端的新建会话对话框、快速开始页与 `webmux run` 都调用它，没有第二个创建入口。

- 线上类型：[`internal/contract/sessions.go`](../../internal/contract/sessions.go) 中的 `CreateSessionRequest`、`CreateSessionResult` 与 `CreateSessionFailure`。
- 会话的其他操作见 [会话、启动配置与其他](http-sessions.md)；聊天视图的行为见 [聊天视图与混合会话](http-hybrid.md)。

创建成功时，实际提交初始聊天提示才返回可选 `initialInput`（沿用会话输入回执，含 turnId/promptId 或排队项）；空白提示和空初始化指令不返回该字段。

## 请求体

```jsonc
// 显式自定义命令
{ "cwd": "/home/u/proj", "cmd": ["bash", "-lc", "make dev"], "name": "可选" }
// 已保存的启动配置
{ "cwd": "/home/u/proj", "configId": "k3m9q2x7", "name": "可选" }
// 新 worktree 中以聊天视图开工
{
  "clientRequestId": "r-8f2k1m9q",
  "newWorktree": { "repositoryId": "r1", "mode": "new", "base": "main" },
  "configId": "k3m9q2x7",
  "view": "chat",
  "prompt": "修复 lint",
  "agentConfig": { "model": "sonnet" },
  "permissionMode": "auto",
}
// 已有工作树中以聊天视图开工（快速开始的「已有工作树」，与上一段二选一）
{
  "clientRequestId": "r-9c3m2n8p",
  "workspace": { "repositoryId": "r1", "worktreeId": "w2" },
  "configId": "k3m9q2x7",
  "view": "chat",
  "prompt": "继续修复 lint",
  "agentConfig": { "model": "sonnet" },
  "permissionMode": "auto",
}
```

请求体拒绝未知字段，只接受一个 JSON 对象；旧字段 `startSide` 作为未知字段返回 400。

**位置**三选一：

- `cwd`：直接目录。
- `workspace: {repositoryId, worktreeId}`：已登记 worktree。省略 `cwd` 时使用工作树根目录，提供时必须位于该工作树内。
- `newWorktree: {repositoryId, mode, branch?, base?, path?}`：先新建 worktree，再在其根目录创建会话。
  - `mode` 为 `new`（从 `base` 新建分支 `branch`）或 `existing`（检出已有分支 `branch`）。
  - `mode` 为 `new` 且省略 `branch` 时，服务端生成 `wt/<日期>-<时间>`，与已有分支或目录冲突时追加 `-2`、`-3` 等后缀。
  - 省略 `base` 时取仓库的默认基点；省略 `path` 时取 `defaultParent/<分支名末段>`。

**命令**二选一：

- `cmd`（加可选 `env`）：完整 argv，不经 shell 展开，首项非空；不接收 `preset` 或单独的预设 `args`。
- `configId`：按已保存配置的完整 argv 与环境变量启动。
  - `cmd` 须为空数组或省略，`env` 须省略或为 `{}`，否则 400。
  - 空字符串 `configId` 400，未知 ID 404，启动配置存储不可用 503。这些检查都在目录与 Git 操作之前完成。
  - 之后修改或删除该配置不影响已启动会话的命令与环境。会话信息保留 `configId`，首页按该配置当前保存的预览设置渲染，配置删除后回退浏览器全局设置。

显式命令可附加 `env: {"NAME": "value"}`：只为该会话在服务环境之上追加或覆盖列出的变量。

- 名称须匹配 `[A-Za-z_][A-Za-z0-9_]*`，值不含 NUL；`WEBMUX_SESSION_ID`、`WEBMUX_SOCKET` 与 `WEBMUX_BIN` 保留（服务端为每个会话设置它们），出现时 400。
- 服务端不自行复制调用方的环境，`env` 也不出现在 `SessionInfo` 中。
- Linux/macOS 的 `webmux run` 会把调用方环境（去掉终端身份等服务端自有变量）放进该字段发送。

**可选字段**：`name`、`labels`（规则见 [父会话与标签](http-sessions.md#父会话与标签)）、`view`、`prompt`、`useInitInstruction`、`expandPrompt`、`attachments`、`agentConfig`、`permissionMode`、`cols`、`rows`、`clientRequestId`。

`cols` 与 `rows` 是 PTY 的初始尺寸：两者都缺省或为 0 时为 80×24，否则必须同时给出且各为 1–1000，只给一个或越界时在预检 400，不创建任何东西。PTY 启动即是该尺寸，`SessionInfo` 与预览一致；
混合会话的终端端同样使用它（聊天视图创建的会话切到终端时亦然）。之后第一个取得控制权的浏览器或 `attach` 仍按自己的窗口调整尺寸；脚本用 [`POST /api/sessions/{id}/resize`](http-sessions.md#调整终端尺寸) 改尺寸。

## 视图

`view` 取 `terminal`（缺省）或 `chat`，只对开启了聊天视图（`chat: true`）的启动配置有意义：

- `terminal`：在 PTY 中运行配置的 argv；混合配置运行档案生成的终端命令（OpenCode 注入 `--standalone`）。
- `chat`：不启动终端，而是启动 ACP 进程并依次调用 `initialize` 与 `session/new`。

对普通配置或自定义命令提供 `view: "chat"` 返回 400。以聊天视图创建而档案的连接器未安装时返回 409 `connector_missing`，不创建会话。

`agentConfig` 与 `permissionMode` 只在 `view` 为 `chat` 时接受，否则 400：

- `agentConfig`：配置项 ID 到值的映射（选择类为取值，布尔类为 `"true"`/`"false"`）。agent 不接受的值记入 `configWarnings`，并作为聊天提示写入时间线，不阻断发送。
- `permissionMode`：`ask` 或 `auto`，缺省按服务端默认（见 [权限模式](http-hybrid.md#聊天视图的输入)）。

`attachments` 为 `POST /api/uploads` 返回的 `{path}`，最多 8 个，只读取 `path`；要求 `prompt` 非空。不是暂存文件夹中的普通文件时在预检返回 400。会话创建后，文件移入该会话的上传目录：聊天视图随首条提示按聊天附件规则发送，终端视图在输入的首条消息后追加 `附件路径：` 与各文件在上传目录中的路径。

## 首条提示与初始化指令

`prompt` 是用户的第一条消息，由服务端在会话启动后发送，浏览器在过程中断开不影响：

- 聊天视图：作为第一条聊天提示，带上 `attachments`。
- 终端视图（普通命令或混合会话的终端视图）：在程序有输出并安静 0.7 秒后（最长等 15 秒）输入并回车（带 `attachments` 时将移入会话上传目录后的路径附在末尾），多行文本在程序开启括号粘贴时按粘贴发送。

`useInitInstruction` 为 `true` 时，把仓库的初始化指令放在用户输入前面；位置须为 `workspace` 或 `newWorktree`，否则 400。仓库没有初始化指令时只发送用户输入。

- 内置变量 `{{user}}`（首尾空白忽略）表示用户输入：指令里写了它，用户输入就放在那里，可以出现多次；没写则为“展开后的指令、空行、用户输入”。
- 展开按单次扫描把 `{{名称}}` 替换为同名快捷指令的正文，插入的正文不再被扫描；服务端读取当前的 `quickCommands` 设置。快捷指令不能命名为 `user` 或 `webmux`。
- 内置的 `{{webmux}}` 在会话创建后替换为该会话的 WebMux 工具说明（与 `POST /api/sessions/{id}/agent-guide` 的 `prompt` 相同），它在 `{{dir}}` 之后插入，其中的文字不再被当作变量。说明不可用时预检返回 503（`step: "precheck"`），不创建 worktree。
- 引用了不存在或保留的快捷指令在预检返回 400（`step: "precheck"`），错误体列出缺少的名称，此时不创建 worktree。
- 展开后文本里的 `{{dir}}` 在会话创建后替换为该会话临时文件夹的路径（必要时创建）；用户输入里的 `{{dir}}` 只在设置了 `expandPrompt` 时替换（见下）。文件夹不可用时在 `prompt` 步骤失败。
- `prompt` 与指令合计超过上限时 400。

`expandPrompt` 为 `true` 时，服务端在会话创建后、`prompt` 发出前，把用户输入里的 `{{dir}}` 换成该会话的临时文件夹路径（按需创建）、
`{{webmux}}` 换成该会话的 WebMux 工具说明（与 [`POST /api/sessions/{id}/agent-guide`](http-sessions.md) 同源）。快速开始页的聊天与终端起点都设置它。

- 先替换 `{{dir}}`、说明最后插入，单遍：说明里的文字不再被当作占位符；`{{user}}` 等其他名称原样保留。
- 缺省或 `false` 时 `prompt` 一字不改（CLI `webmux run --prompt` 不设置它）；终端视图的首条消息同样遵循。
- 用户输入含占位符而临时文件夹或说明不可用时，预检返回 503 且不创建任何东西；会话创建后的失败按 `step: "prompt"` 报告。
- 与 `useInitInstruction` 同用时，先替换用户输入，再放入初始化指令的 `{{user}}`。

## 执行步骤、结果与失败

服务端按顺序执行，任一步失败即停止后续步骤：

1. `precheck`：请求形状、启动配置与视图、仓库可用、初始化指令、首条消息的大小与附件；新 worktree 加聊天视图时还检查混合会话记录与第三方连接器。
2. `worktree`：有 `newWorktree` 时创建 worktree。
3. `session`：创建会话。
4. `config`：应用 `agentConfig` 与 `permissionMode`。
5. `prompt`：发送 `prompt`。

成功返回 201：

```jsonc
{
  "session": { "id": "ab2cd3ef", "...": "SessionInfo" },
  "worktree": { "...": "Worktree" }, // 只在本次新建时出现
  "configWarnings": [], // agent 没有接受的配置项
}
```

每个失败都返回 `CreateSessionFailure`，未设置的字段省略：

```jsonc
{
  "error": "…",
  "step": "precheck | worktree | session | config | prompt",
  "reason": "…", // 同 SessionConflict 的原因码；分支或路径已存在时为 conflict
  "eligibility": "…",
  "connector": { "...": "AgentConnector" },
  "worktree": { "...": "Worktree" }, // 已创建的 worktree
  "sessionId": "ab2cd3ef", // 已创建的会话
}
```

- 已完成步骤创建的 worktree 与会话保留，不回滚。重试时把已建好的 worktree 作为 `workspace` 传入，只启动会话。
- 状态码按失败原因映射：不含 `newWorktree` 与 `prompt` 的单步请求失败时 `step` 为 `precheck` 或 `session`，状态码与以前相同。
- 分支或路径冲突为 409、`step: "worktree"`、`reason: "conflict"`。
- 混合会话的失败（普通会话不受这些检查影响）：记录文件 `hybrid-sessions.json` 不可用 503，错误信息含文件路径；记录已达 200 条且没有可淘汰的已归档记录 409 `reason: "limit"`；agent 启动或建会话失败 502，错误信息附 agent 标准错误的末尾，仅返回给请求方，不写日志。
- 记录已满时，新建混合会话会先淘汰最近未更新的已关闭或已中断记录，再写入新记录；仍在运行的记录不会被淘汰。创建失败时会把刚淘汰的记录恢复。

**去重**：带 `clientRequestId`（`[A-Za-z0-9_-]{8,64}`）的请求在第一次执行进行中或完成后 10 分钟内重复提交时，等待并返回第一次的结果，不再创建 worktree、会话或提示；首条提示复用该 ID。
缺省时不去重。处理不依赖客户端连接，最长 3 分钟。

## 创建前的上传与选项探测

`POST /api/uploads` 在会话存在之前接收附件：

- multipart 正文与 `POST /api/sessions/{id}/upload` 相同（单个名为 `file` 的部分），201 返回 `{path}`。
- 文件存放在服务实例临时根目录下共享的 `quick-start/` 暂存文件夹（0700/0600），被创建请求移走前保留；服务启动时清空。

`POST /api/launch-configs/{id}/options {repositoryId}` 在发送前读取启动配置的 agent 提供的配置项：

- 服务端在仓库主工作区以聊天视图临时启动该 agent，`session/new` 后读取配置项，随即关闭会话（agent 支持时发 `session/close`）并结束进程，不创建 WebMux 会话或记录。
- 成功：200，结构同聊天状态的 `config` 数组，同时记为该启动配置的 `chatOptions`。
- 启动配置未开启聊天视图：409 `no_chat_view`；连接器缺失：409 `connector_missing`；agent 调用失败：502；其余状态码与创建会话相同。
- 与资格探测共享并发上限，最长 45 秒，客户端断开即取消。

## 安全边界

- 接口使用现有认证与 Origin 校验；仓库、启动配置、基准分支与配置项都按标识或数据传递，不拼接成命令。
- 经本机套接字从某个会话内的进程创建时，服务端按对端进程记录 `parentId`；请求体不能指定它。
- 能在创建前判断的条件（仓库可用、启动配置开启了聊天视图、第三方连接器已安装）都在创建 worktree 之前校验。
