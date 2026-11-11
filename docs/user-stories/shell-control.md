# 脚本与 agent 控制

本组描述用 `webmux` 命令行在脚本中、或由运行在会话里的 CLI agent 控制 WebMux 的能力。命令经本机套接字访问服务，不需要 Token；使用说明见 [用脚本和 agent 控制 WebMux](../usage/shell-control.md)。

## S. 脚本与 agent 控制

- **US-S01 会话内找到所属实例**：作为在会话里运行的脚本或 agent，我希望不依赖 `PATH` 或默认配置就能连回启动我的 WebMux，以便控制的总是同一个实例。
  - 每个会话进程，包括混合会话聊天视图的 ACP agent，都有 `WEBMUX_SESSION_ID`、`WEBMUX_SOCKET`（本机 API 套接字绝对路径）与 `WEBMUX_BIN`（服务端正在运行的可执行文件）；
  - 三个变量属于服务端自有变量：创建请求或启动配置中出现时以 400 拒绝，`webmux run` 继承调用方环境时跳过；服务不修改会话的 `PATH`；
  - CLI 按 `--socket`、`--config`、`WEBMUX_SOCKET`、默认配置的顺序定位服务，`--socket` 与 `--config` 互斥；指定的套接字连不上时以退出码 3 结束，不回退到其他实例；
  - 接受会话 ID 的命令都接受 `self`；会话外使用 `self` 以退出码 2 结束；
  - 旧版 CLI 调用服务端没有的新接口时提示“服务端版本过旧”，给出两边的版本并建议使用 `$WEBMUX_BIN`；
  - 本机状态（`status --json`）包含 `bin`。
- **US-S02 读取会话元数据**：作为脚本或 agent，我希望一次取得会话所在的仓库、分支、启动配置、混合状态、父子会话与标签，以便判断自己的处境和要操作的对象。
  - `GET /api/sessions/{id}/details` 返回 `SessionDetails`，不含 Token、启动环境变量值或其他会话的输出；临时文件夹只在已存在时报告，接口不创建它；
  - `webmux get [SESSION]` 在会话内默认 `self`，会话外必须给出 ID；默认输出对齐的键值，`--json` 输出 `SessionDetails`；
  - `--field PATH` 按点分路径输出单个值（字符串不带引号，对象与数组为 JSON），值为空或不存在时以 1 结束；`--json` 与 `--field` 互斥。
- **US-S03 标记并找到自己创建的会话**：作为编排脚本，我希望知道哪些会话由我创建并能给会话打标签，以便事后筛选和清理。
  - 经本机 API 从会话内创建的会话（`POST /api/sessions`，包括带 `newWorktree` 或 `prompt` 的一次性创建；`POST /api/quick-starts` 已删除）自动记录 `parentId`；服务端按套接字对端进程判断，请求体无法指定，经 TCP 创建的会话没有父会话；
  - 创建请求可带 `labels`，非法标签以 400 拒绝且不创建会话；`PATCH /api/sessions/{id}/labels` 以 `{set, remove}` 原子修改；
  - 标签键匹配 `[a-z0-9][a-z0-9._-]{0,62}`，值最多 256 个字符且不含控制字符，每个会话最多 16 个；父会话与标签只在内存中，经 `/ws/sessions` 同步，Web 端暂不显示；
  - `webmux label SESSION KEY=VALUE... KEY-...` 设置或删除标签；`webmux ls` 支持 `--label KEY=VALUE`、`--label KEY`、`--parent SESSION` 与 `--status STATE`，条件同时满足，`--json` 输出筛选后的原始对象。
- **US-S04 完整的命令帮助**：作为第一次使用某个命令的人或 agent，我希望每个命令都有完整、可机读的帮助，以便不必猜测选项。
  - 所有命令（包括运维命令）在一张命令表中声明；`--help` 包含用法、说明、参数、选项与默认值、环境变量、输出、退出码、至少两个示例和相关命令，英文，80 列换行；
  - `-h`、`--help`、`webmux help <命令>` 写到标准输出并以 0 退出，不连接服务；用法错误以 2 退出并提示 `--help`；只给分组命令（如 `webmux config`）时把子命令列表写到标准错误并以 2 退出；
  - `webmux help --json [命令]` 输出命令树（选项、默认值、互斥关系、退出码、示例）；
  - 选项可以写在位置参数之后，`--` 之后的参数原样保留；`run` 在第一个位置参数处停止解析；
  - 顶层帮助保留服务端选项一行，Windows 客户端据 `-log-file` 判断 WSL 内的版本；
  - 示例使用统一后的命令（`run --new-worktree --view chat`、`send --reply`、`view`、`resume`、`rm --forget`），不出现已删除的 `chat new|send|cancel|switch|ls|resume|forget`；
  - 说明中用到手册已完整说明的概念（会话状态、`self`、临时文件夹）时只给一句话定义，并在相关命令中指向 `webmux guide <主题>`；
  - 测试保证每个可见命令有摘要和至少两个可解析的示例，每个选项有说明，运维命令的选项与实现一致。

- **US-S05 按需阅读 WebMux 工具手册**：作为 CLI agent，我希望从一个固定地址按需加载与当前服务版本一致的 shell 工具说明，以便只读需要的部分就能正确使用 `webmux`。
  - 服务端按 llms.txt 约定提供 `/llms.txt`（H1、摘要、命令前缀说明与主题链接，含 `## Optional`）、`/llms/<主题>.md` 与 `/llms-full.txt`；内容只在内存中渲染，不写入临时文件夹或其他文件；
  - 主题包括连接与身份（可执行文件与套接字的定位顺序、排错）、会话（含已归档会话的列出、恢复与删除记录）、启动、聊天视图专有操作、临时文件夹（原“临时文件夹”预设的全部说明）、命令目录、退出码、操作守则与常用流程；
  - 手册与命令帮助分工：`commands` 主题是命令目录，按分组每个命令一行（用法与摘要），页首指向 `webmux <命令> --help` 与 `webmux help --json`，不含选项、示例等帮助小节；
    它在服务启动时由命令表渲染，`webmux guide` 离线时用 CLI 自己的命令表渲染同一页，仓库中不保存生成的副本；
  - 手写主题不重复命令帮助（选项清单、按键与枚举清单、默认值、单命令示例），`recipes` 用 `run --new-worktree --view chat --prompt … --reply` 等跨命令流程；
    测试保证手册与使用说明中 `webmux <命令> … --<选项>` 的选项属于该命令，除 `recipes` 外没有与命令表示例相同的命令行；
  - TCP 上不需要 Token 但检查 Host，内容不含版本、主机、路径或会话信息，示例不写死安装路径；本机套接字上附加本实例的版本、可执行文件与套接字，并在调用者运行于某个会话内时附加“本会话 ID”（按套接字对端进程判定，与父会话记录相同），使 agent 只读手册也能知道自己是哪个会话；
  - `webmux guide [TOPIC] [--full]` 经本机套接字读取，服务未运行时输出内嵌副本并在标准错误说明；`--path` 输出服务端可执行文件与套接字路径；
  - `webmux dir [SESSION]` 输出会话临时文件夹路径（按需创建），会话内默认 `self`。
- **US-S06 在指令中插入 WebMux 工具说明**：作为使用者，我希望在快捷指令或仓库初始化指令中写 `{{webmux}}`，以便 agent 拿到本会话的手册地址、命令路径和交付文件位置。
  - `POST /api/sessions/{id}/agent-guide` 确保临时文件夹存在并返回说明：会话 ID、`webmux` 可执行文件与套接字的绝对路径（环境变量丢失时的用法）、`/llms.txt` 地址与 `webmux guide`、本会话临时文件夹路径与“交付文件放这里而不是项目仓库”的规则；不含 Token；
  - 初始化指令中的 `{{webmux}}` 不再被拒绝：会话创建后在 `{{dir}}` 之后插入说明，说明中的文字不再被当作变量；说明不可用时预检返回 503 且不创建 worktree；
  - 快捷指令正文中的 `{{webmux}}` 在选择时经该接口展开，失败时提示“WebMux 工具说明不可用”且不插入；快速开始页（US-Q18）不能在浏览器展开，菜单插入原样的 `{{webmux}}`，随 `expandPrompt` 由服务端在会话创建后展开，规则与初始化指令相同；
  - 设置页初始化指令的“可插入”提供内置的“WebMux 说明”按钮，快速开始预览说明 `{{webmux}}` 会在会话创建后替换；快捷指令名称仍不能是 `webmux`。

- **US-S07 从 shell 启动、跟随或等待会话**：作为脚本或 agent，我希望 `run` 具备 Web 端新建会话对话框的全部位置选择，并能不经终端跟随输出或等待状态，以便把启动、等待和取结果写进脚本。
  - 位置：
    - 当前目录所在的已登记 worktree 自动关联；
    - `--worktree REPO:WORKTREE|PATH` 选已有 worktree，`--cwd` 须在其中；
    - `--new-worktree` 是开关（`--branch`、`--base`、`--existing-branch`、`--worktree-path`、`--repo`、`--refresh-base`）：只发一次带 `newWorktree` 的 `POST /api/sessions`，由服务端先建 worktree 再启动；
      省略 `--branch` 时服务端自动生成 `wt/<日期>-<时间>` 形式的不冲突分支名；默认像 Web 端一样加入主页显示（`--no-pin` 不加入）；
    - `--` 之前出现位置参数时以 2 结束，与 `--new-worktree` 同用时提示改用 `--branch`（旧写法 `--new-worktree BRANCH` 不会被当作命令执行）；
    - 失败按服务端的 `step` 报告失败步骤，保留已创建的 worktree 与会话并给出路径与 ID，不回滚；
  - 视图与首条消息：`--view terminal|chat`（取代 `--start-side`，`chat` 隐含 `--detach`）、`--prompt TEXT|-`（终端视图且未给 `--name` 时即会话名，US-B07）、`--init`（需要工作区关联，含 `--new-worktree`）；
    `--set ID=VALUE`、`--permission ask|auto`、`--attach-file PATH`（先经 `POST /api/uploads` 上传，要求 `--prompt`）只在 `--view chat` 时可用，否则在本地以 2 结束；
  - 输出与等待：`--json` 输出 `{session}`（新建 worktree 时另含 `worktree` 与 `configWarnings`），配置警告写到标准错误，`--wait STATE --timeout DUR` 等会话第一次进入该状态（超时 124），
    `--reply` 要求 `--view chat` 与 `--prompt` 并与 `--wait`、`--follow` 互斥，`--label` 设置标签，`--dry-run` 只打印请求且隐藏环境变量值、不上传附件；
  - 没有终端时自动分离，`--attach` 强制附着；
  - `--cols N --rows M` 设定终端的初始尺寸，必须同时给出且各为 1–1000，否则在本地以 2 结束；省略时为 80×24；请求体的 `cols`、`rows` 贯通到 PTY，所以进程第一次输出就按该尺寸排版，
    会话信息与预览尺寸一致，混合会话的终端端同样使用它；随后第一个取得控制权的浏览器或 `attach` 仍按自己的窗口调整，这是原有行为；
  - `--follow` 以观察者身份把输出写到标准输出（`--strip` 去除控制序列），以程序的退出码退出（信号为 128+N）；SIGINT/SIGTERM 只停止跟随（130/143），`--kill-on-interrupt` 先结束程序；连接中断以 1 退出并提示；
    `--follow` 与 `--detach`、`--attach`、`--view chat`、`--reply` 互斥。
- **US-S08 一条命令完成快速开始**：作为编排者，我希望从 shell 把任务交给新 worktree 中的 agent 并取回结果，以便并行分派工作。
  - `webmux run --new-worktree --view chat --launch-config NAME --prompt TEXT|- --reply` 一次完成：仓库默认是当前目录所属的已登记仓库（`--repo` 可改），分支名省略时由服务端生成，起始提交默认取仓库默认基点；
  - `--set ID=VALUE`、`--permission ask|auto`、`--init`（需显式给出，不再随仓库初始化指令默认开启）、`--attach-file PATH`（先上传）、`--label`；`--view terminal` 或省略 `--view` 时在 agent 的终端中启动；
  - 失败按步骤报告并列出已创建的 worktree 与会话，不回滚；配置警告写到标准错误；
  - `--reply` 跟踪首条提示开启的回合，按 `send --reply` 的规则输出 agent 的最终回复（`--json` 为回合事件），`end_turn` 以 0 退出，其他结束原因或出现授权、表单请求以 1 退出，`--auto-wait-requests` 继续等待，超时 124；
  - `webmux launch-config options NAME [--repo REPO]` 经 `POST /api/launch-configs/{id}/options` 列出 agent 提供的配置项与可选值，供 `--set` 使用；未开启聊天视图的配置以 1 结束；
  - `chat new` 已删除，按未知命令以 2 结束。

- **US-S09 向会话输入并等待结果**：作为编排脚本，我希望向终端会话输入并可靠地等到它有反应，以便把交互程序串进流程。
  - `webmux send SESSION [TEXT|-] [--enter] [--key NAME]...` 调用 `POST /api/sessions/{id}/input`，由服务端按当前视图分派，CLI 不再自己连接终端 WebSocket；
  - 终端视图：以带回执的提交写入文字，收到 `written` 才以 0 退出，`partial`、`failed`、`missing`（409 `input_failed`）以 1 退出；命名按键在文字之后发送，未知按键以 2 结束；
  - 发送像浏览器输入一样取得控制权，控制指示器显示 `script`，在会话内附上来源会话 ID；
  - 脚本之后要改终端尺寸见 US-S19；
  - 聊天视图：文字作为提示发送（见 US-S14）；选项与当前视图不符（终端视图带 `--attach-file`/`--mode`，聊天视图带 `--key`）时服务端返回 409 `wrong_view`，CLI 以 1 结束且不写入任何内容；
  - 切换中（`switching`）或会话已归档（`not_active`）时以 1 结束并报告原因；
  - `webmux interrupt SESSION` 调用 `POST /api/sessions/{id}/interrupt`：终端视图写入档案中断键或 Ctrl-C，聊天视图取消进行中的回合，没有回合时以 0 退出；
  - `send --wait STATE` 先订阅会话列表再输入，只接受输入之后的变化；STATE 可以是逗号分隔的多个状态；离开订阅时的状态后再回到它（如 waiting、running、waiting）也算变化；
  - `webmux wait SESSION... --until STATES [--changed] [--stable DUR] [--any] [--timeout DUR] [--json]`：`attention` 表示有锁存的提醒（agent 完成一回合后也会锁存），超时 124，会话被移除或在未等待 `exited` 时结束以 1 退出；
  - 状态语义：`waiting` 表示需要用户介入（agent 的授权、提问、未读的完成或出错，或没有 agent 信号时的提示符启发式，所以停在提示符的 shell 是 `waiting`），`idle` 只表示长期无活动，不是“命令或回合已结束”的信号；等 shell 命令结束用 `--wait waiting,idle`，等 agent 一回合结束用 `--until attention`。
- **US-S10 读取会话的输出与画面**：作为脚本或 agent，我希望读取会话输出，并看到全屏 TUI 当前显示的内容，以便判断下一步。
  - `webmux logs SESSION` 输出回放缓冲区原始字节，`--strip` 去除控制序列（CRLF、单独 CR、退格按行处理），`--tail N` 取最后 N 行（与 `--follow` 互斥），`--follow` 以观察者身份持续输出且不抢控制权；
  - `webmux screen SESSION` 以观察者身份取回放，在 CLI 本地按会话网格渲染，输出可见画面的纯文本，`--json` 含尺寸、光标、是否备用屏幕与 `stable`；回放被截断时在标准错误提示；聊天视图以 1 结束；服务端不做终端模拟；
  - `screen` 不无限等待画面收稳：回放到达后静默 150 ms 即返回；输出持续不断（如转圈动画）则最多再等 1 秒，按已收到的数据输出快照，`--json` 的 `stable` 为 false 并在标准错误提示，退出码仍为 0；会话已退出视为稳定；
  - `--scrollback N`（0–100000，默认 0 即只输出可见画面）让回放模拟器保留 N 行滚动历史，输出把历史行放在画面之前，`--json` 的 `lines` 同样包含；历史来自 2 MiB 回放缓冲区，按控制端当前宽度重排，
    缓冲区已丢弃的更早内容取不到，截断提示照旧；备用屏幕程序（vim、less、多数 TUI）没有滚动历史，只输出可见画面；不提供按其他尺寸回放的选项；`N` 越界以 2 结束。
- **US-S11 通用 API、会话生命周期与事件**：作为脚本，我希望结束、移除、改名会话，订阅会话事件，并调用没有专用命令的接口。
  - `webmux kill`（结束进程，会话保留为已结束；带聊天视图的会话变为已关闭并归档）、`webmux rm`（移除，运行中的先结束，带聊天视图的会话保留归档）、`webmux rename SESSION [NAME]`（空名称交还给终端标题）；
  - `webmux rm SESSION --forget` 调用 `DELETE /api/sessions/{id}?forget=1`，同时删除归档记录与临时文件夹（运行中的带聊天视图会话先关闭）；终端上先确认，非交互时需要 `--yes`，否则以 2 结束；
    不带 `--forget` 对已归档会话执行时以 1 结束并提示改用 `--forget`；
  - `webmux events [--session ID]...` 把 `/ws/sessions` 事件逐行输出为 JSON，可按会话筛选，断线后重连并输出新的快照；
  - `webmux api [-X METHOD] PATH [-d DATA|@FILE|-]` 只接受 `/api/` 与 `/local/v1/` 路径并原样输出响应，`-d` 须为 JSON，非 2xx 以 1 退出，WebSocket 路径以 2 拒绝；会话内的现有限制照常生效。
- **US-S12 shell 补全**：作为命令行使用者，我希望按 Tab 补全命令、选项与会话 ID，以便少记少打。
  - `webmux completion bash|zsh|fish` 输出补全脚本；隐藏的 `__complete` 给出命令、子命令、选项、枚举值，并向服务查询会话 ID（附标题；`resume` 与 `rm --forget` 补全 `GET /api/sessions?state=archived` 中的已归档会话）、启动配置、仓库、worktree 与手册主题；
  - 查询超过 1 秒或服务未运行时只给静态候选，不输出错误，也不输出 Token 或环境变量值；Windows 客户端不提供补全。
- **US-S13 从脚本提醒用户**：作为在会话里跑长任务的脚本或 agent，我希望任务结束或需要决定时直接提醒用户，以便用户不必盯着终端。
  - `webmux notify [--kind notify|done|error|question] [MESSAGE]` 默认作用于当前会话，会话外需 `--session`；消息最多 200 个字符、不含控制字符，超出时 CLI 以 2 拒绝；
  - 提醒来源为 `cli`，覆盖当前锁存的提醒而不受来源分层限制；会话徽章、页面内弹窗与后台推送照常出现，消息作为弹窗说明行与推送正文，只保存在内存中；
  - 之后按原有规则清除：用户输入、程序的下一次状态报告（会话在聊天视图时为 agent 的新状态或新一轮）；`notify --clear` 与 `send` 的输入同样清除，并同步到所有设备；已退出的会话以 1 拒绝。
- **US-S14 从脚本驱动混合会话聊天**：作为编排脚本，我希望向另一个 agent 发送提示、取回回复并代为处理它的请求，以便不打开浏览器也能完成多 agent 协作。
  - `webmux send SESSION TEXT` 对聊天视图的会话立即发送或进入服务端队列，可用 `--attach-file PATH`（先经会话上传接口上传，任一失败则不发送）附带文件；`--json` 输出 `{view, queued, itemId, position, turnId, promptId}`；
  - `--mode` 取 `auto`（默认，空闲即发、忙则排队）、`steer`（插入当前回合）、`refuse`（忙时以 1 结束并报告 `busy`，不排队）或 `now`（取消当前回合并立即发送）；排队时在不带 `--json` 时向标准错误输出队列项与位置，标准输出保持为空；
  - `--reply` 先连接聊天流再发送，跟踪本次输入开启的回合（插话时跟踪进行中的回合，`now` 跟随取消后开始的回合），与 `--wait` 互斥；会话在终端视图时不写入任何内容，以 1 结束并提示改用 `--wait STATE`；
  - `end_turn` 以 0 退出并输出最终回复（`--json` 为该回合事件），其他结束原因以 1 退出；遇到授权或提问时输出请求 ID 与选项并以 1 退出，
    `--auto-wait-requests` 继续等待；排队的提示被删除时以 1 退出；超时 124 且不取消回合；
  - `webmux chat` 组只保留聊天视图专有的操作：`chat status` 列出所在视图（取自会话信息的 `view` 与 `transition`）、回合、队列、权限模式、待回答请求（ID、选项、表单字段）与 agent 设置，
    `chat watch` 持续输出消息、工具调用、请求与回合（`--json` 为 NDJSON 帧，默认跳过历史；`--history` 的历史部分每个工具调用只显示最终状态，实时帧不变）；
  - `webmux chat log SESSION [--last N] [--tools] [--json]` 一次性读取聊天历史后退出（只读，聊天与终端视图均可）：默认输出最后一条用户或 agent 消息，`--last` 取 1 到 1000 条（其他值以 2 结束）；
    工具调用默认不显示，`--tools` 时每个调用一行（标题与最终状态，同一调用的进度帧折叠），`--json` 输出一个 `ChatFrame`（`events`）；服务端历史已被丢弃部分或回合进行中时在标准错误提示；
  - `chat answer`（`--option`、`--content`、`--decline`、`--cancel`）、`chat set ID=VALUE`（校验设置与可选值，失败的项报告后以 1 结束）、
    `chat permission ask|auto`、`chat queue ls|rm|edit|send-now|clear|resume` 使用与 Web 端相同的接口并同步到所有设备；结束当前回合用 `webmux interrupt`；
  - 目标没有聊天视图（`no_chat_view`）时以 1 结束并说明原因，需要聊天视图却在终端视图时以 1 结束并提示 `webmux view SESSION chat`；每次写操作带新的 `clientRequestId`；
  - 已删除的 `chat send`、`chat cancel` 按未知命令以 2 结束。
- **US-S15 从脚本切换端与恢复历史会话**：作为脚本，我希望在终端与聊天之间切换混合会话，并找回、恢复或清理已结束的 agent 会话。
  - `webmux view SESSION terminal|chat` 调用 `POST /api/sessions/{id}/view` 并等到切换结束；终端 TUI 忙时 `--force` 强制中断，否则取消切换并以 1 退出；
    服务端列出 agent 会话候选时用 `--agent-session` 重试（只在切到 `chat` 时有效）；普通会话以 1 结束并说明没有聊天视图；
  - `webmux ls --archived`（或 `--state closed|interrupted|archived|all`，可重复）与 `--repo REPO` 经 `GET /api/sessions?state=` 列出已归档会话，按归档更新时间从新到旧，状态列显示 `closed` 或 `interrupted`；
  - `webmux resume SESSION [--view terminal|chat]` 调用 `POST /api/sessions/{id}/resume`，缺省按归档时的视图恢复，终端视图且有终端时附着、否则输出 ID（`--detach` 不附着），
    启动配置已删除时需要 `--without-env`，候选冲突时用 `--agent-session` 重试，运行中的会话（`not_archived`）以 1 结束；
  - `webmux rm SESSION --forget` 删除已归档会话的记录（agent 自己的历史保留），终端上先确认，非交互时需要 `--yes`；
  - 已删除的 `chat switch`、`chat ls`、`chat resume`、`chat forget` 按未知命令以 2 结束。
- **US-S16 从脚本管理 worktree 与主页显示**：作为编排脚本，我希望为并行任务创建、查看和删除 worktree，并决定它们是否出现在主页，以便 Web 端与脚本看到同一个工作区。
  - `worktree ls|show|create|rm`：`REPO` 为 ID、路径或 `.`，worktree 为 ID、分支或路径，找不到时以 2 结束并列出候选；`create` 默认放在 `<仓库>.worktrees/<分支末段>`，
    `--refresh-base` 先取回上游；`rm` 先在标准错误输出预览，有阻止原因时需要 `--force`，终端上要求输入路径确认，非交互时需要 `--yes`；
  - 主页状态（`pinned`、`sessions`、`hidden`）与 Web 端规则一致：从未设置的仓库只显示主工作区，有会话或归档记录的 worktree 总会显示；
  - `worktree` 命令对已归档会话的计数读取 `GET /api/sessions?state=archived`；
  - `worktree create` 与 `run --new-worktree` 默认加入主页显示（`--no-pin`）；`worktree pin|unpin|home [--set|--reset]` 只改该仓库的 `homeWorktrees` 条目，
    409 时重新读取并重放（最多 3 次），清理已不存在的 worktree，并同步到所有浏览器。
- **US-S17 从脚本登记仓库与修改仓库设置**：作为在新克隆的仓库里工作的脚本，我希望登记仓库、查看分支与提交，并设置快速开始用的初始化指令。
  - 例如 `webmux repo add && printf '{{webmux}}\n\n{{user}}' | webmux repo settings . --init -` 之后，`webmux run --new-worktree --view chat --init` 带上包含 WebMux 工具说明的初始化指令；
  - `repo ls|add|rm|show|refresh|branches|commits|base`：`add` 幂等，`rm` 只取消登记并清理主页显示设置，`commits --from` 接受分支、基点或完整提交 ID，`base` 的取回失败只警告；
  - `repo settings` 对应仓库设置对话框：输出初始化指令、长度与引用；`--init TEXT|-|@FILE`、`--clear-init`、`--edit`；超过 4000 个字符在本地以 2 拒绝，
    引用不存在的快捷指令时给出与设置页相同含义的警告并仍然保存。
- **US-S18 从脚本管理启动配置与端口转发**：作为配置开发环境的脚本，我希望增删改启动配置并为开发服务器开转发。
  - `launch-config ls|show|add|edit|rm|presets|restore`：`show` 默认把变量值显示为 `<hidden>`，`--reveal-env` 才输出；`add` 的命令原样保存不经 shell，
    `edit` 只改给出的字段，未提及的变量值保持不变；名称不存在时以 2 结束并列出已保存的名称；变量值不出现在输出或诊断中；
  - 聊天视图（混合模式）开关：`add --chat` 先经 `POST /api/launch-configs/chat-support` 做支持检查，不支持时以 1 结束；`edit` 用 `--chat`/`--no-chat` 开关；`ls` 显示是否开启聊天视图；
    旧选项 `--hybrid`/`--no-hybrid` 按未知选项以 2 结束；
  - `forward ls|add|rm`：`add` 输出转发端口，`--session self` 关联当前会话，`--no-compat` 关闭兼容改写；转发关闭、端口非法或重复时给出原因并以 1 结束。
- **US-S19 从脚本调整终端尺寸**：作为脚本或 agent，我希望在没有浏览器控制终端时改变会话的 PTY 尺寸，以便让程序按我需要的宽高排版并用 `screen` 读到完整画面。
  - `POST /api/sessions/{id}/resize {cols, rows, take?}` 与其他会话接口一样要求认证并检查 Origin（本机套接字上的会话内调用同样可用），成功时 200 返回 `SessionInfo`，PTY、会话信息与预览同时变为新尺寸并向观察端广播控制帧；
  - 当前没有控制端，或控制端是脚本输入（`send` 取得的 `script` 控制权）时直接允许；控制端是浏览器或 `attach` 时返回 409，因为浏览器收到控制帧后会把自己的网格重发，脚本强行修改会来回抢；
    请求带 `take: true` 时脚本先接管控制权（控制指示器显示 `script`）再调整尺寸，随后释放；检查与调整在同一个临界区内完成；
  - `cols`、`rows` 各为 1–1000，否则 400；会话不存在 404；会话已退出或处于聊天视图（没有终端运行）409；
  - `webmux resize SESSION COLSxROWS [--take]` 调用该接口，成功时以 0 结束；格式错误或越界以 2 结束且不发请求；409、404 与其他失败以 1 结束并在标准错误给出原因；
  - 之后第一个取得控制权的浏览器或 `attach` 仍按自己的窗口调整尺寸。

### 依据

- [会话命令与命令表](../../internal/clicmd/command.go)、[帮助渲染](../../internal/clicmd/help.go)、[元数据命令](../../internal/clicmd/metadata.go)、[运维命令表](../../cmd/webmux/commands.go)
- [会话详情与标签接口](../../internal/api/sessionmeta.go)、[父会话与标签](../../internal/session/relations.go)、[进程所属会话](../../internal/session/manager_process.go)
- [本机套接字定位](../../cmd/webmux/sessions.go)、[会话环境](../../internal/session/env.go)、[保留变量](../../internal/contract/env.go)
- [手册与简述](../../internal/agentguide/agentguide.go)、[手册主题](../../internal/agentguide/llms/connect.md)、[手册路由与说明接口](../../internal/api/guide.go)、[guide 与 dir 命令](../../internal/clicmd/guide.go)
- [会话控制命令](../../internal/clicmd/control.go)、[补全](../../internal/clicmd/completion.go)
- [notify 命令](../../internal/clicmd/notify.go)、[脚本提醒](../../internal/session/manager_attention.go)、[推送正文](../../internal/push/alert.go)
- [worktree 与主页显示命令](../../internal/clicmd/worktree.go)、[仓库命令](../../internal/clicmd/repo.go)、[启动配置命令](../../internal/clicmd/launchconfig.go)、[转发命令](../../internal/clicmd/forward.go)
- [聊天命令](../../internal/clicmd/chat.go)、[聊天历史、设置与队列命令](../../internal/clicmd/chat_config_queue.go)、[聊天接口](../../internal/api/chat.go)
- [启动命令](../../internal/clicmd/launch.go)、[会话列表订阅、跟随与回合跟踪](../../internal/clicmd/streams.go)、[仓库与主页显示](../../internal/clicmd/workspace.go)
- [会话创建与初始化指令展开](../../internal/workspace/create_session.go)、[会话接口](../../internal/api/sessions.go)、[快捷指令运行](../../web/src/components/terminal/SessionTerminal.svelte)、[预设与内置变量](../../web/src/domains/settings/quick-commands.ts)
- [脚本调整尺寸](../../internal/session/session.go)、[尺寸校验与创建尺寸](../../internal/session/manager.go)、[尺寸请求类型](../../internal/contract/sessions.go)、[resize 命令与 screen 滚动历史](../../internal/clicmd/control.go)
