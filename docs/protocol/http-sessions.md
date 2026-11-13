# HTTP API：会话、启动配置与其他

## 会话

会话是唯一的名词：终端与聊天是会话的**视图**，不是会话种类。输入、中断、切换、恢复与结束都作用于会话本身，由服务端按当前视图分派。

| 方法   | 路径                             | 说明                                                                                             |
| ------ | -------------------------------- | ------------------------------------------------------------------------------------------------ |
| GET    | `/api/sessions`                  | 会话列表 `SessionInfo[]`；`?state=` 按状态筛选，见下文                                           |
| POST   | `/api/sessions`                  | 创建会话，201 返回 `{session, worktree?, configWarnings}`，见 [创建会话](http-session-create.md) |
| GET    | `/api/sessions/{id}`             | 单个会话（任何状态，含已归档）；不存在 404                                                       |
| PATCH  | `/api/sessions/{id}`             | 设置用户标题 `{"name"}`，返回会话；空串恢复自动标题；含控制字符或超过 512 字节 400               |
| DELETE | `/api/sessions/{id}`             | 停止当前视图的进程（SIGHUP → SIGKILL），204；见“结束、移除与删除记录”                            |
| DELETE | `/api/sessions/{id}?remove=1`    | 从运行中列表移除，204；普通会话仍在运行 409                                                      |
| DELETE | `/api/sessions/{id}?forget=1`    | 移除并删除归档记录与临时文件夹，204                                                              |
| POST   | `/api/sessions/{id}/input`       | 按当前视图输入，见下文                                                                           |
| POST   | `/api/sessions/{id}/interrupt`   | 按当前视图中断，204，见下文                                                                      |
| POST   | `/api/sessions/{id}/resize`      | 脚本调整终端尺寸 `{cols, rows, take?}`，200 返回 `SessionInfo`，见“调整终端尺寸”                 |
| POST   | `/api/sessions/{id}/view`        | 切换视图，见 [聊天视图与混合会话](http-hybrid.md#视图切换与恢复)                                 |
| POST   | `/api/sessions/{id}/resume`      | 恢复已归档会话，见 [聊天视图与混合会话](http-hybrid.md#视图切换与恢复)                           |
| GET    | `/api/sessions/{id}/output`      | 分页读取原始保留字节，见下文；不附着终端、不触发重绘                                             |
| GET    | `/api/sessions/{id}/buffer`      | 下载完整原始缓冲（`application/octet-stream`）                                                   |
| GET    | `/api/sessions/{id}/upload`      | 读取本会话已上传的单个文件（`?path=`），见下文                                                   |
| POST   | `/api/sessions/{id}/upload`      | 上传文件，见下文                                                                                 |
| POST   | `/api/uploads`                   | 创建会话前暂存附件，201 `{path}`，见 [创建会话](http-session-create.md#创建前的上传与选项探测)   |
| POST   | `/api/sessions/{id}/artifacts`   | 确保会话临时文件夹存在，200 `{path}`，见下文                                                     |
| GET    | `/api/sessions/{id}/details`     | 会话详情 `SessionDetails`，见下文；不存在 404                                                    |
| PATCH  | `/api/sessions/{id}/labels`      | 修改标签 `{set, remove}`，返回 `SessionInfo`；非法标签 400，不修改任何标签                       |
| POST   | `/api/sessions/{id}/agent-guide` | `{{webmux}}` 的说明 `AgentGuidePrompt`，见下文；会话不存在 404，临时文件夹或说明不可用 503       |
| POST   | `/api/sessions/{id}/attention`   | 发出提醒 `{kind, message?}`，返回 `SessionInfo`，见下文；非法 400，已退出 409                    |
| DELETE | `/api/sessions/{id}/attention`   | 清除锁存的提醒（与用户输入效果相同），204                                                        |

以下旧接口已删除，请求返回 404，没有别名或转发：`/api/quick-starts*`、`/api/hybrid-sessions*`、`/api/sessions/{id}/switch`、`/api/sessions/{id}/chat/prompts` 与 `/api/sessions/{id}/chat/cancel`。

### 按状态列出

`GET /api/sessions?state=STATE` 可重复，取值：

- `active`（缺省）：运行中的会话，按 `lastActiveAt` 倒序，与 `/ws/sessions` 一致。
- `closed`、`interrupted`：已归档的会话，按 `archive.updatedAt` 倒序。
- `archived`：等于 `closed` 加 `interrupted`。
- `all`：先列 `active`，再列已归档。

未知取值 400。普通会话没有持久化，服务重启即结束，因此已归档列表只会出现带聊天视图的会话。

### 输入与中断

`POST /api/sessions/{id}/input` 的请求体为 `{clientRequestId, text, submit?, keys?, attachments?, mode?}`，按会话当前视图分派：

- 终端视图：`text` 与 `submit` 作为一次带回执的提交写入 PTY，随后按顺序写入 `keys`。
  - `submit` 缺省 `false`，为 `true` 时追加档案或终端的提交键。
  - `keys` 取 `backspace`、`ctrl-c`、`ctrl-d`、`down`、`enter`、`esc`、`left`、`right`、`tab`、`up`，未知名称 400。
  - 回执为 `written` 时 200 `{view: "terminal"}`；`partial`、`failed` 或 `missing` 时 409 `input_failed`，`submission` 带回执状态。
  - 输入像浏览器输入一样取得终端控制，控制者标签为 `script`；从会话内经本机套接字调用时为 `script · <来源会话 ID>`。
  - 带 `attachments` 或 `mode` 时 409 `wrong_view`，不写入任何内容。
- 聊天视图：`text` 与 `attachments` 作为提示，`mode` 取 `auto`（缺省，空闲时发送，否则排队）或 `steer`（插话）；`submit` 被忽略。
  - 200 `{view: "chat", queued, itemId?, turnId?, promptId?}`，含义见 [聊天视图的输入](http-hybrid.md#聊天视图的输入)。
  - 带 `keys` 时 409 `wrong_view`。

`POST /api/sessions/{id}/interrupt {clientRequestId}`：

- 聊天视图：取消进行中的回合；没有回合时不做任何事。
- 终端视图：写入档案定义的中断键（混合会话），否则写入 Ctrl-C（`\x03`）。

两者的共同规则：

- `clientRequestId` 须为 `[A-Za-z0-9_-]{8,64}`，否则 400；同一 ID 的重复请求只生效一次。
- `text` 为空且没有 `keys`、`attachments` 时 400。
- 切换进行中 409 `switching`；会话已归档 409 `not_active`；不存在 404。

### 调整终端尺寸

`POST /api/sessions/{id}/resize {cols, rows, take?}` 让脚本设置会话终端运行的 PTY 尺寸，认证与 Origin 检查同其他会话接口，本机套接字上的会话内调用同样可用。`cols`、`rows` 各为 1–1000，否则 400。

- 允许的情形：没有控制端，或控制端是脚本输入（`input` 取得的 `script` 控制权）。成功时 PTY、`SessionInfo` 与预览同时变为新尺寸，并向观察端广播 `0x06` 控制帧。
- 控制端是浏览器或 `attach` 时 409，响应体只有 `{error}`：浏览器收到控制帧后会把自己的网格重发，脚本强行修改会来回抢。`take: true` 时脚本以 `script` 标签接管控制权，调整后释放，原控制端变为观察端。
- 检查与调整在同一个临界区内完成，期间不会有另一个连接接管。
- 会话不存在 404；会话已退出，或处于聊天视图（没有终端运行）409，响应体只有 `{error}`。

### 结束、移除与删除记录

- `DELETE /api/sessions/{id}`：停止当前视图的进程。普通会话留在运行中列表，显示为已退出；带聊天视图的会话先取消聊天回合或按中断流程处理终端回合，再结束进程，变为 `closed` 并离开运行中列表。切换中 409 `switching`。
- `DELETE /api/sessions/{id}?remove=1`：从运行中列表移除，普通会话须已退出（仍在运行 409）；带聊天视图的会话保留归档。
- `DELETE /api/sessions/{id}?forget=1`：隐含 `remove`，并删除归档记录与会话临时文件夹，不删除 agent 自己的会话库。对运行中的带聊天视图会话先关闭再删除。之后该 ID 在任何状态下都返回 404。

### 冲突响应

所有会话、视图、输入、中断、恢复与聊天命令的 409 使用同一形状 `SessionConflict`，未设置的字段省略：

```jsonc
{
  "error": "…",
  "reason": "wrong_view",
  "eligibility": "…",
  "candidates": [],
  "connector": {},
  "submission": {},
}
```

`reason` 是稳定的代码：

- 当前状态不允许：`switching`（切换进行中）、`wrong_view`（选项不属于当前视图）、`not_active`（会话已归档）、`not_archived`（恢复运行中的会话）。
- 会话没有聊天视图：`no_chat_view`，对普通会话调用 `/view`、`/api/sessions/{id}/chat/*` 或 `/ws/s/{id}/chat` 时返回。
- 终端输入未完整写入：`input_failed`，`submission` 为回执状态。
- 聊天视图专有的原因（`not_eligible`、`connector_missing`、`session_ambiguous`、`occupied_external`、`answered`、`sent`、`steering_unavailable`、`session_missing`、`config_missing`、`limit`）
  见 [聊天视图与混合会话](http-hybrid.md#冲突与错误)。

### 父会话与标签

- 创建请求可附加 `labels: {"KEY": "value"}`。键须匹配 `[a-z0-9][a-z0-9._-]{0,62}`，值最多 256 个字符且不含控制字符，最多 16 个；非法时 400，且不创建会话。
- 经本机套接字从某个会话内的进程创建时，服务端按对端进程记录 `parentId`（调用者所在会话）；请求体不能指定它，经 TCP 创建的会话没有。
- `SessionInfo` 中二者为空时省略，变化经 `/ws/sessions` 的 `updated` 事件同步；只保存在内存中。
- `PATCH /api/sessions/{id}/labels` 的 `set` 先合并、`remove` 再删除，整体生效；结果超过 16 个时 400。

**会话详情**：`GET /api/sessions/{id}/details` 返回：

```jsonc
{
  "session": { "id": "ab2cd3ef", "...": "SessionInfo" }, // 视图、agent 与归档信息都在这里
  "link": "http://127.0.0.1:7681/s/ab2cd3ef", // 服务不知道本机链接时为 null
  "repository": { "id": "…", "name": "app", "path": "/home/u/app" }, // 无工作区关联时为 null
  "worktree": {
    "id": "…",
    "branch": "fix/lint",
    "path": "/home/u/app.worktrees/fix-lint",
    "main": false,
  },
  "launchConfig": {
    "id": "k3m9q2x7",
    "name": "Claude Code",
    "icon": "claude",
    "chat": true, // 是否开启了聊天视图
  }, // 否则 null
  "launchConfigMissing": true, // 仅当配置已被删除
  "artifactsDir": null, // 临时文件夹已存在时为路径；接口不创建它
  "forwards": [], // 来源为该会话的端口转发
  "parentId": "p4rent22",
  "labels": { "run": "42" },
  "children": ["ch2ld345"], // parentId 为该会话的会话，按创建时间排序
}
```

详情不含 `hybrid` 字段、Token、启动环境变量的值或其他会话的输出；对已归档会话同样可用。

**WebMux 工具说明**：`POST /api/sessions/{id}/agent-guide` 确保会话临时文件夹存在，返回 `{prompt, guideUrl, dir, bin, socket}`：

- `prompt` 是 `{{webmux}}` 展开后的文字：会话 ID、`bin` 与 `socket`（不在 `PATH` 时用绝对路径，环境变量丢失时加 `--socket`）、手册地址 `guideUrl`（本机链接加 `/llms.txt`）与 `webmux guide`、本会话临时文件夹 `dir` 与交付规则；路径原样插入，不含 Token。
- `bin`、`socket` 未知时为 `null`，`prompt` 省略对应内容。

**WebMux shell 工具手册**：按 llms.txt 约定在站点根路径提供，不在 `/api/` 下，因此 TCP 上与静态页面一样不需要 Token（仍检查 Host）：

| 路径                  | 内容                                                             |
| --------------------- | ---------------------------------------------------------------- |
| `GET /llms.txt`       | 索引（`text/plain`）：H1、摘要、命令前缀说明、按组列出的主题链接 |
| `GET /llms/<主题>.md` | 单个主题（`text/markdown`），未知主题 404                        |
| `GET /llms-full.txt`  | 索引后依次拼接全部主题（`text/plain`）                           |

- 内容嵌入在可执行文件中，只在内存中渲染，`Cache-Control: no-store`。
- TCP 上不含版本、主机、路径或任何会话信息；本机套接字上在摘要后列出本实例的版本、可执行文件与套接字路径。
- 本机套接字上，`/llms.txt` 与 `/llms-full.txt` 还按对端进程（与父会话记录相同的判定）指出调用者所在的会话，在“本实例”清单里多一行“本会话 ID”；调用者不在任何会话内、或会话已不存在时省略该行。判定不创建临时文件夹，`/llms/<主题>.md` 不随调用者变化。

**上传**：`multipart/form-data`，恰好一个文件字段 `file`。成功返回 201 `{"path": "<服务器绝对路径>"}`。

- 文件位于会话临时文件夹的 `uploads/` 中，同名时追加 ` (2)` 等序号而不覆盖。
- 单文件上限 100 MiB（超限 413），读期限 5 分钟（超时 408）。
- 无效请求 400，会话不存在 404，会话已退出 409，临时文件夹不可用 503。
- `GET /api/sessions/{id}/upload?path=<服务器绝对路径>` 读取该会话 `uploads/` 内的普通非符号链接文件（上限 10 MiB，超限 413），按文件内容嗅探 `Content-Type` 并返回原始字节；其他路径或不存在的文件返回 400，会话不存在 404。

**会话临时文件夹**：`POST /api/sessions/{id}/artifacts` 幂等地创建并返回 `$TMPDIR/webmux-artifacts-<uid>-<实例摘要>/<会话ID>`（0700）。

- 会话存在（含已退出）时 200，不存在 404。
- 路径已被符号链接、他人目录或可被其他用户访问的目录占用时 503，且不修改该条目。
- 普通会话移除后删除文件夹；带聊天视图的会话在 `DELETE /api/sessions/{id}?forget=1` 成功后删除；服务启动清扫不属于可恢复混合会话的文件夹。
- 代码视图以 `GET /api/sessions/{id}/code/context?root=artifacts` 浏览它（`rootKind: "artifacts"`，无 Diff）。`root` 缺省或为 `workspace` 时是工作区，其他值 400，文件夹尚未创建时 404。

```ts
interface SessionInfo {
  id: string;
  generation: string; // 本次运行的代际，16 位十六进制；输出 offset 与提交都绑定它。聊天视图（没有终端运行）与已归档时为空字符串
  title: string;
  cwd: string;
  cmd: string[];
  status: "running" | "waiting" | "idle" | "exited"; // waiting 表示提示符、终端读取或 attention；计时休眠不是 waiting
  exitCode: number | null;
  preview: string; // 屏幕底部最多 100 条非空行，以 \n 分隔
  createdAt: string; // RFC 3339
  lastActiveAt: string;
  cols: number;
  rows: number;
  clients: number; // 当前附着的连接数
  workspace: {
    repositoryId: string;
    worktreeId: string;
    worktreePath: string;
  } | null;
  // 程序主动报告的“需要介入”，存在时 status 为 "waiting"；不含任何终端正文。
  attention: {
    kind: "question" | "permission" | "done" | "error" | "notify" | "bell";
    source: "agent" | "terminal" | "cli"; // 结构化事件 / 普通通知或响铃 / webmux notify
    since: string; // RFC 3339，种类变化时更新
    message?: string; // 仅 cli 来源：脚本附带的文字，只保存在内存中
  } | null;
  // 程序通过 OSC 7 报告的目录；null 表示未报告或进程已退出。
  // 只作导航线索，不改变 cwd、workspace、读取边界或命令执行环境。
  reportedCwd: {
    path: string; // 已解码的绝对路径
    host: string; // 报告中的主机名，可能为空
    remote: boolean; // 非本机（例如 SSH 远端）；不得用于本机文件定位
    since: string; // RFC 3339，路径或主机变化时更新
  } | null;
  // 创建时使用的已保存启动配置；临时命令省略该字段。
  configId?: string;
  // 生命周期：普通会话恒为 active（进程退出后仍是 active，status 为 exited）；
  // 带聊天视图的会话关闭或中断后为 closed / interrupted，只存在于归档中。
  state: "active" | "closed" | "interrupted";
  // 会话支持的视图：普通会话 ["terminal"]，用开启了聊天视图的启动配置创建的会话 ["terminal", "chat"]。
  views: ("terminal" | "chat")[];
  // 最近一次落定的视图；切换进行中仍为来源视图。
  view: "terminal" | "chat";
  // 切换进行中时为目标视图与步骤，否则为 null。
  transition: {
    to: "terminal" | "chat";
    phase:
      | "interrupting"
      | "awaiting_choice"
      | "cancelling"
      | "stopping"
      | "starting"
      | "loading"
      | "rolling_back";
  } | null;
  // views 含 chat 时为聊天视图背后的 agent，否则为 null。
  agent: {
    name: string; // 档案名：opencode、omp、codex、claudecode 或 agy
    title: string; // 档案的显示名，如 OpenCode、Claude Code
    icon: string; // 品牌图标键，属于启动配置图标目录；浏览器不认识时只显示 title
    sessionId: string | null; // agent 自己的会话 ID，未知时为 null
    work?: string; // 聊天视图回合进行中的一行当前工作，回合外省略
  } | null;
  // 只在 state 为 closed 或 interrupted 时出现；不含环境变量值、消息或队列正文。
  archive?: {
    configMissing: boolean; // 启动配置已被删除
    previousAgentSessionIds: string[]; // 最近 8 个历史 agent 会话 ID
    activity?: "idle" | "turn" | "switching"; // 中断前的活动，后两者表示上一轮可能未完成
    queuedCount: number; // 中断时丢失的排队条数
    interruptedAt: string | null; // RFC 3339
    updatedAt: string; // RFC 3339
  };
  // 会话进程树中监听在回环或通配地址上的 TCP 端口，按端口升序，不含 WebMux 自身端口；
  // 端口开始或停止监听后 3 秒内更新。不支持检测的平台（非 Linux/macOS）恒为 []。
  listeningPorts: { port: number; process: string }[];
}
```

`SessionInfo` 不再含 `hybrid` 对象；线上视图取值只有 `terminal` 与 `chat`，不再出现 `tui`、`acp` 或 `switching`，表示视图的字段名为 `view`（切换请求中为 `to`）。

- 普通会话：`state: "active"`、`views: ["terminal"]`、`view: "terminal"`、`transition: null`、`agent: null`，没有 `archive`。
- 会话在聊天视图时：`status`、`attention`（来源 `agent`）与 `preview`（最新 agent 消息前三行）来自聊天视图的回合状态；`cmd` 为启动配置的命令。
- 已归档（`closed` 或 `interrupted`）的会话：运行时字段取零值——`generation` 为空字符串、`status` 为 `exited`、`exitCode` 为 `null`、`clients`、`cols` 与 `rows` 为 0、`preview` 为空字符串、`listeningPorts` 为 `[]`；`lastActiveAt` 等于 `archive.updatedAt`。客户端以 `state` 区分它与已退出的普通会话。

### 提醒

`POST /api/sessions/{id}/attention` 让脚本（通常是 `webmux notify`）发出提醒：

- `kind` 取 `notify`、`done`、`error`、`question`；`message` 可选，去掉首尾空白后最多 200 个字符且不含控制字符，否则 400。
- 提醒来源记为 `cli`，覆盖当前锁存的提醒而不受来源分层限制；之后按原有规则清除：用户输入、程序的下一次状态报告，或 `DELETE`。
- 会话在聊天视图时，提醒保留到 agent 报告新的状态（新的提醒或开始新一轮）。
- `message` 只保存在内存中，用作推送正文（受 `include_title` 控制）和提醒提示；服务重启后消失。
- 已退出的会话 409，不存在 404。

`DELETE /api/sessions/{id}/attention` 不论提醒来源都清除它，经 `/ws/sessions` 同步到所有设备；没有提醒时同样返回 204。

## 其他

| 方法 | 路径                   | 说明                                                                                                                                                                                                           |
| ---- | ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| GET  | `/api/health`          | `{ok: true, sessions: n, load?, memory?}`，见下方说明                                                                                                                                                          |
| GET  | `/api/config`          | `{defaultCwd, defaultShell, hostname, workspace: {gitAvailable, registryAvailable, gitError, registryError}, tunnel, pendingUpdate?, portDetection}`，不含 token；`portDetection` 表示本平台会话会报告监听端口 |
| GET  | `/api/fs/dirs?prefix=` | 匹配前缀的全部子目录（含符号链接目录），普通目录优先，按名称排序                                                                                                                                               |
| POST | `/api/fs/mkdir`        | `{path}`，204                                                                                                                                                                                                  |

`/api/health` 的 `load` 为 `{one, five, fifteen, cpus}`（1/5/15 分钟负载与逻辑核心数），`memory` 为 `{total, available}`（字节，`available` 不含需换出才能回收的部分）；平台无法读取时省略对应字段（Linux 与 macOS 支持）。

`tunnel` 是只读的运行时隧道状态：`{enabled, state, named, hostname, publicUrl?}`。

- `state`：`off`、`starting`、`ready` 或 `failed`。
- `enabled`：仅在 `starting`/`ready` 时为 `true`，与[公网隧道设置](http-tunnel.md)的启用期望不同。
- `named` 与 `hostname`：动态读取当前有效设置（Web 覆盖优先，否则使用配置文件），关闭时也保留模式信息；Named Tunnel 缺少 `hostname` 时为空字符串。
- `publicUrl`：仅在 `ready` 时出现且不含 token。
- 响应不包含 Cloudflare 凭据。

`defaultShell` 为只读的启动文件 shell：`"bash"`、`"zsh"`，或空字符串表示不自动包装；由服务进程 `$SHELL` 的 basename 决定，未设置时为 `"bash"`。

`pendingUpdate` 仅在可执行文件路径上已安装的版本已知且不同于运行版本时出现：`{installedVersion, launcher}`，`launcher` 为 `systemd`、`launchd`、`windows-tray` 或 `manual`，决定 Web UI 给出的重启方式。旧服务端不返回该字段。

## 启动配置

启动配置保存在服务端 `<state_dir>/launch-configs.json`，在所有已认证设备间共享。只提供用户自定义的启动配置，不再提供内置预设或 `/api/presets` 路由；新安装首次启动时服务端在该文件写入常见 agent 的普通配置，经同一接口列出、修改和删除。

| 方法   | 路径                               | 说明                                                                                                                      |
| ------ | ---------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| GET    | `/api/launch-configs`              | 摘要数组（按创建顺序）：`{id, name, icon?, cmd, envNames, chatOptions, preview, chat, chatSupport, createdAt, updatedAt}` |
| GET    | `/api/launch-configs/{id}`         | 完整配置：`{id, name, icon?, cmd, env, preview, chat, chatSupport, createdAt, updatedAt}`                                 |
| POST   | `/api/launch-configs`              | `{name, icon?, cmd, env?, preview?, chat?}`，201 返回完整配置                                                             |
| PUT    | `/api/launch-configs/{id}`         | `{name, icon?, cmd, env?, preview?, chat?}` 整体替换，省略的 `env` 与 `preview` 被清空，200 返回完整配置                  |
| DELETE | `/api/launch-configs/{id}`         | 204                                                                                                                       |
| POST   | `/api/launch-configs/chat-support` | `{cmd}`，只读返回 `chatSupport`，见 [聊天视图与混合会话](http-hybrid.md#启动配置)                                         |
| POST   | `/api/launch-configs/{id}/options` | `{repositoryId}`，读取 agent 的配置项，见 [创建会话](http-session-create.md#创建前的上传与选项探测)                       |

`chat`（是否开启聊天视图，界面称“混合模式”）与只读的 `chatSupport` 的规则见 [聊天视图与混合会话](http-hybrid.md#启动配置)；请求体中的 `hybrid`、`chatSupport` 或 `hybridSupport` 视为未知字段返回 400。

`icon` 为可选的本地图标 ID：`terminal`、`antigravity`、`claudecode`、`codex`、`geminicli`、`cursor`、`githubcopilot`、
`opencode`、`qwen`、`kimi`、`codebuddy` 或 `pi`。

- 省略或空字符串表示自动匹配；更新时同样恢复自动匹配。
- 不接受 URL 或 SVG 内容。
- 旧文件省略此字段仍可读取：浏览器按可识别的直接命令或标准 Bash/Zsh 启动文件包装匹配图标，未匹配时显示通用终端。
- `terminal` 可显式覆盖自动匹配。

`preview` 为 `null` 或省略时沿用服务端同步的全局预览设置；否则为 `{lines, skip}`，`lines` 为 1–50 的整数、`skip` 为 0–50 的整数。会话卡片按创建时记录的 `configId` 读取该配置的当前预览值，配置被删除或不可用时回退全局设置。

列表的 `chatOptions` 是该配置最近报告的配置项清单与最后的值（结构同聊天状态的 `config`），还没有时为空数组：

- 来源是该配置最近一次聊天视图会话，或 `POST /api/launch-configs/{id}/options` 的配置读取；
- 服务端保存在 `<state_dir>/agent-options.json`，删除启动配置时一并清除；
- 它只供快速开始与新建会话在 agent 启动前展示配置，不参与切换或恢复。

列表只含环境变量名（字典序），值只由单条读取与创建/更新响应返回。限制：

- 名称去首尾空白后 1–64 个字符且唯一（重名 409）。
- `cmd` 1–128 项、首项非空、单项不超过 8 KiB。
- `env` 最多 64 项，名称规则与会话 `env` 相同，值不超过 8 KiB。
- 最多 100 条。

错误状态码：

- 校验失败 400，未知 ID 404，落盘失败 500（内存回滚）。
- 文件损坏、目录不可读写或状态目录已被另一实例占用：503，错误信息包含原因与文件路径。

`launch-configs.json` 当前版本为 `2`，新增 `preview` 字段。本次升级仅接受版本 `2`；版本 `1` 文件保留原样，启动配置 API 返回 503。升级前备份旧配置，通过支持 v1 的旧版本保存为 v2，或备份后移走旧文件并在新版重新创建；详见 [部署升级说明](../deployment.md)。

## 原始终端输出分页

`GET /api/sessions/{id}/output?maxBytes=65536` 返回仍保留的原始 PTY 字节，独立于 WebSocket 的终端恢复画面。
返回 generation、start、end、nextOffset（十进制字符串）、encoding=base64、data、reset、historyTruncated 和 limited。
maxBytes 缺省 65536，范围 1–65536；只复制当前页，不因保留历史总量较大而失败。

续读同时传回 generation 和 offset=nextOffset；同代且仍保留的偏移直接续读。
代次变化、偏移已淘汰或超前时 reset=true，从当前最早保留字节重新返回；historyTruncated 表示更早字节已被覆盖。
limited 表示当前页尚未到达本次读取时的 end。字节可能从 UTF-8 或 ANSI 序列中间开始，不承诺可独立渲染。
空页 data 为空字符串。读取不附着客户端、不取得控制权、不改变终端尺寸或触发重绘；响应使用 Cache-Control: no-store。
未知、重复、空参数及非法游标返回 400；不存在返回 404，无终端运行返回 409；鉴权与 Origin 沿用现有边界。
