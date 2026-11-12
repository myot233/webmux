# 用脚本和 agent 控制 WebMux

`webmux` 命令行可以完成 Web 端的大部分操作：脚本和运行在 WebMux 会话里的 CLI agent 都能直接创建会话、输入、等待、读取输出和提醒你。

先从下面的常见任务找到对应命令；单个命令的全部选项、默认值、输出与示例写在 `webmux <命令> --help` 中，本页不重复。`webmux help --json` 输出机器可读的命令树。

## 常见任务

| 我想……                                  | 命令                                                                                       |
| --------------------------------------- | ------------------------------------------------------------------------------------------ |
| 让 agent 在新 worktree 里做事并拿到回复 | `webmux run --new-worktree --launch-config "Claude Code" --view chat --prompt "…" --reply` |
| 在持久会话里跑命令，按命令退出码结束    | `webmux run --follow --strip -- go test ./...`                                             |
| 给已有会话发一句话并等它做完            | `webmux send <ID> "…" --reply`（聊天视图）或 `--enter --wait idle`（终端视图）             |
| 读 agent 最后的回复                     | `webmux chat log <ID>`                                                                     |
| 看全屏 TUI 当前画面                     | `webmux screen <ID>`                                                                       |
| 等会话需要我或空闲                      | `webmux wait <ID> --until attention,idle --changed`                                        |
| 任务完成后提醒我                        | `webmux notify --kind done "构建完成"`                                                     |
| 查看当前会话的仓库与分支                | `webmux get`                                                                               |
| 找到本会话的临时文件夹                  | `webmux dir self`                                                                          |
| 读 WebMux 工具手册                      | `webmux guide`                                                                             |

下面按用途展开，每节先给命令，再说明规则与边界。

## 连接到正确的实例

命令经本机 Unix 套接字访问服务，不需要 Token。CLI 按以下顺序找到服务：

1. `--socket PATH`：直接连接这个套接字；
2. `--config PATH`：按配置文件定位，规则与服务相同；
3. 环境变量 `WEBMUX_SOCKET`：WebMux 为每个会话设置，指向启动它的实例；
4. 默认配置 `~/.config/webmux/config.toml`。

`--socket` 或 `WEBMUX_SOCKET` 指定的套接字连不上时，命令以退出码 3 结束，不会改连其他实例。

每个会话（包括在聊天视图中运行的 agent 进程）还会得到：

| 变量                | 含义                                                      |
| ------------------- | --------------------------------------------------------- |
| `WEBMUX_SESSION_ID` | 当前会话 ID；命令中的 `self` 指的就是它                   |
| `WEBMUX_SOCKET`     | 本实例本机 API 套接字的绝对路径                           |
| `WEBMUX_BIN`        | 服务端正在运行的可执行文件；`PATH` 里没有 `webmux` 时用它 |

`PATH` 中的 `webmux` 可能比服务旧或新。用 `"$WEBMUX_BIN"` 运行的命令总是与服务同一版本；旧版 CLI 调用新接口时会提示“服务端版本过旧”并给出两边的版本。

## 查看会话元数据

```sh
webmux get                               # 在会话内查看当前会话：仓库、分支、启动配置、视图、父子会话与标签
webmux get abcd2345 --json               # 另一个会话的完整详情（SessionDetails）
branch=$(webmux get --field worktree.branch)   # 不用 jq 取单个字段
```

详情不包含 Token 或启动环境变量的值；临时文件夹只在已经存在时报告，`get` 不会创建它。文本输出不显示会话链接，需要链接时用 `webmux open <ID>`；`--json` 仍按 `SessionDetails` 返回服务端的 `link` 字段。

## 父会话与标签

从会话内经本机 API 创建的会话会自动记录 `parentId`，即调用者所在的会话。服务端按连接进程判断，请求体无法指定。

```sh
webmux label self task=lint run=42       # 设置标签；KEY- 删除
webmux ls --parent self                  # 我创建的会话
webmux ls --label run=42 --status exited --json   # 按标签和状态筛选
```

标签键匹配 `[a-z0-9][a-z0-9._-]{0,62}`，值最多 256 个字符，每个会话最多 16 个。父会话和标签只保存在内存中，随会话结束而消失。

## 启动会话

```sh
webmux run --detach --json --wait idle --timeout 60s --launch-config omp   # 等 agent 就绪
webmux run --follow --strip -- go test ./...       # 在持久会话里跑命令，输出纯文本，以命令的退出码退出
webmux run --new-worktree --branch fix/lint --launch-config "Claude Code" --view chat --init --prompt "修复 lint" --reply
webmux launch-config options "Claude Code"         # 列出 run --set 可用的配置项
```

- `run` 是唯一的创建命令，与 Web 端新建会话对话框一样用一次请求完成。可以指定：
  - 位置：当前目录、已有 worktree 或新建 worktree；
  - 命令或启动配置、起始视图、首条消息、agent 设置、权限模式与附件。
- 失败时按步骤报告，已创建的 worktree 与会话保留。
- `--new-worktree` 是开关，分支名用 `--branch` 给出，省略时由服务端生成 `wt/<日期>-<时间>`；新 worktree 像 Web 端一样加入主页显示。
  - 没有 `--` 时出现的位置参数是用法错误，因此旧写法 `--new-worktree BRANCH` 不会被当作命令执行。
- 聊天视图没有终端可附着，以聊天视图启动时总是后台运行；`--reply` 跟踪首条提示的回合并输出 agent 的最终回复。
- 仓库有初始化指令时，`--init` 需要显式给出。
- `--cols N --rows M` 给终端设定初始尺寸，程序第一次输出就按它排版。
  - 两者须同时给出，各 1–1000，默认 80×24；
  - 第一个取得控制权的浏览器或 `attach` 仍按自己的窗口调整。

## 输入、中断与读取输出

```sh
webmux send <ID> 'make test' --enter --wait idle --timeout 10m   # 输入并回车，等会话回到空闲
webmux send <ID> "修复 lint 并汇报改动" --reply --timeout 20m     # 聊天视图：输出 agent 的最终回复
webmux interrupt <ID>                            # 取消聊天回合，或在终端写入中断键
webmux resize <ID> 120x40 [--take]               # 脚本改终端尺寸；浏览器或 attach 控制时需要 --take
webmux wait <ID> --until attention,idle --changed --timeout 30m
webmux logs <ID> --strip --tail 50               # 最近 50 行纯文本
webmux screen <ID> [--scrollback 200]            # 全屏 TUI 当前画面的文本；可带上方的滚动历史；--json 的 stable 表示画面是否已静止
webmux events --session <ID>                     # 会话事件，NDJSON
webmux kill <ID>; webmux rm <ID>; webmux rename <ID> 新名称
webmux api /api/launch-configs                   # 没有专用命令的接口，原样输出响应
```

- `send` 与 `interrupt` 只需要会话 ID，由服务端按会话当前视图分派：
  - 终端视图：带回执的输入。它取得控制权（控制指示器显示 `script`），在会话内还附上来源会话 ID；
  - 聊天视图：作为提示立即发送或排队；`--mode` 取 `auto`（默认，空闲即发、忙则排队）、`steer`（插入当前回合）、`refuse`（忙时以 1 结束、不排队）或 `now`（取消当前回合并立即发送）；
  - 排队时标准错误报告队列项与位置（`--json` 的 `position`），标准输出保持为空；
  - 选项属于另一种视图时服务端以 `wrong_view` 拒绝，什么也不写入。
- `send --wait` 先订阅再输入，只接受之后的状态变化，对两种视图都适用；`--reply` 只用于聊天视图，在终端视图上以 1 结束并提示改用 `--wait`。
- `logs --strip` 适合按行输出的程序；全屏 TUI 用 `screen`，画面在 CLI 本地渲染，服务端不做终端模拟。
- `resize` 在没有控制端、或控制端是脚本（`send` 取得的 `script` 控制权）时直接生效。
  - 浏览器或 `attach` 控制时以 1 结束（服务端 409）：浏览器会把自己的网格重发，两边会来回抢；
  - `--take` 让脚本接管控制权、调整后释放。
- `screen --scrollback N` 在画面之前输出最多 N 行滚动历史（0–100000，`--json` 的 `lines` 同样包含）。
  - 历史来自 2 MiB 回放缓冲区，按控制端当前宽度重排，更早的内容已被丢弃；
  - 备用屏幕程序（vim、less、多数 TUI）没有滚动历史。

## 提醒用户

```sh
make release; webmux notify --kind done "release 构建完成"
webmux notify --clear --session <ID>
```

- 会话显示提醒徽章，打开的浏览器弹出提示（消息作为说明行），订阅推送的设备收到以消息为正文的通知（`include_title = false` 时用固定正文）。
- `--kind` 取 `notify`（默认）、`done`、`error`、`question`；消息最多 200 个字符，只保存在内存中，服务重启后消失。
- 提醒来源记为 `cli`，覆盖当前提醒；之后按原有规则清除。`--clear` 与 `send` 的输入都和用户输入一样清除提醒。

## 视图、聊天与已归档会话

```sh
webmux view <ID> chat                            # 切到聊天视图并等待完成；terminal 切回终端
webmux chat status <ID>                          # 视图、回合、队列、待回答请求与选项、agent 设置
webmux chat answer <ID> <请求ID> --option allow_once
webmux chat watch <ID>                           # 持续输出聊天
webmux chat log <ID>                             # 一次性读取最后一条消息（通常是 agent 的最后回复）后退出
webmux chat log <ID> --last 3 --tools           # 最近三条消息与其间的工具调用（标题与最终状态）
webmux chat set <ID> model=sonnet; webmux chat permission <ID> ask
webmux ls --archived --repo .                    # 已关闭或已中断、可以恢复的会话
webmux resume <ID> --view chat --detach
webmux rm <ID> --forget --yes                    # 删除归档记录与临时文件夹
```

- 从开启了聊天视图的启动配置创建的会话有终端与聊天两个视图，`webmux ls` 在状态列显示它当前的视图（终端、聊天、切换中）。
  - `ls --json` 原样包含 `views`、`view`、`transition` 与 `agent`，不再含 `hybrid` 对象。
- 启动配置用 `webmux launch-config add` 或 `webmux launch-config edit` 的 `--chat` / `--no-chat` 开启或关闭聊天视图（混合模式）；旧选项 `--hybrid` 已删除。
- `run --launch-config <名称>` 引用这类配置时创建混合会话，默认从终端开始；本地终端可以照常输入，附着中的终端不提供切换按键。
  - `--view chat` 从聊天开始（隐含 `--detach`）；
  - `--set ID=VALUE`、`--permission ask|auto`、`--attach-file PATH` 只在 `--view chat` 时可用。
- `send` 的 `--attach-file`、`--mode steer` 与 `--reply` 只用于聊天视图。
- `chat` 只保留聊天视图专有的操作（`status`、`watch`、`log`、`answer`、`set`、`permission`、`queue`）。会话没有聊天视图时以 1 结束；
  需要聊天视图而会话在终端视图时以 1 结束，并提示 `webmux view <ID> chat`。
- `chat log` 读取服务端仍保留的聊天历史后立即退出，只读、不取得控制。
  - 默认输出最后一条消息；`--last N`（1 到 1000，其他值以 2 结束）取最近 N 条用户与 agent 消息；
  - `--tools` 加上工具调用，同一个调用的进度帧折叠为一行：标题与最终状态；
  - `--json` 输出一个 `ChatFrame`（`type` 为 `events`，消息的分段已合并）；
  - 历史有长度上限，较早的消息可能已被丢弃（标准错误提示）；
  - 与 `send --reply` 不同，它不发送输入，也不等待回合。`chat watch --history` 的历史部分同样只显示每个工具调用的最终状态，实时帧不变。
- `view` 等到切换结束；终端 TUI 忙时 `--force` 强制切换，不强制就取消切换并以 1 结束。找不到 agent 会话时列出候选并接受重试。
- `attach` 在会话处于聊天视图或切换中时拒绝附着，以退出码 1 结束。
  - 附着期间被切到聊天视为被动分离：恢复终端、提示后以 0 结束；
  - 切回终端后需重新 `attach`。
- 带聊天视图的会话结束后成为已归档（`closed` 或 `interrupted`），`ls` 默认不列出。
  - `ls --archived`（或 `--state closed|interrupted|all`、`--repo`）列出它们；
  - `resume <ID> [--view chat|terminal]` 以原 ID 恢复；
  - `rm` 对它以 1 结束并提示改用 `--forget`（`rm <ID> --forget --yes` 删除记录）。

## 仓库、worktree、启动配置与转发

```sh
webmux worktree create . --branch fix/lint --base main   # 输出路径，并像 Web 端一样加入主页显示（--no-pin 不加入）
webmux worktree ls . --hidden                    # HOME 列：pinned、sessions、hidden
webmux worktree pin . exp/a; webmux worktree home . --reset
webmux worktree rm . fix/lint --yes              # 先预览；有阻止原因时需要 --force
webmux worktree switch . main feat/a             # 原地切换分支；也可 --track origin/fix/b、--create NAME [--from REF]、--detach REF
webmux repo add; webmux repo branches .; webmux repo commits . --from main; webmux repo base . main
webmux repo remotes .; webmux worktree show . feat/x --json   # 远程 URL 已脱敏；JSON 含 upstream、ahead、behind
webmux repo fetch . && webmux worktree prune . --yes   # 获取全部远程；清理目录已缺失的失效登记，分支保留
webmux repo ff . feat/a                          # 快进到上游：只用本地远程跟踪引用
webmux repo push . feat/c --remote origin --to feat/c --yes   # 发布没有上游的分支
webmux repo branch-rm . --cleanup                # 删除工作树遗留的分支；--unmerged 才删有独有提交的
printf '{{webmux}}\n\n{{user}}' | webmux repo settings . --init -
webmux launch-config add "Dev" --icon terminal --env MODE=dev -- bash -lc 'npm run dev'
webmux launch-config edit "Dev" --unset-env MODE; webmux launch-config show "Dev" --json
webmux forward add 5173 --session self --label web
```

- `REPO` 是仓库 ID、仓库内路径或 `.`；worktree 用 ID、分支名或路径。找不到时以 2 结束并列出候选。
- 主页显示的规则与 Web 端相同：从未设置的仓库只显示主工作区，有会话或已归档会话的 worktree 总会显示。
  - 写入只改该仓库的 `homeWorktrees` 条目，与其他设备冲突（409）时重新读取并重放，最多 3 次。
- `worktree rm`、`repo rm`、`launch-config rm` 与 `rm --forget` 一样：终端上先确认，非交互时需要 `--yes`。`repo rm` 不删除文件，并清理该仓库的主页显示设置。
- `worktree switch` 先在标准错误输出预览（阻止原因、警告与运行中的会话），会话不会被关闭：
  - 有阻止原因（未提交修改、进行中的合并或变基、分支已被其他工作树检出）时什么都不改，以 1 结束；
  - 有运行中的会话时在终端上要求输入 `yes`，非交互时需要 `--yes`，否则以 2 结束；
  - 成功时输出所在分支（`--detach` 时输出提交），`--json` 输出 Worktree。
- `worktree ls` 的状态列在有上游时追加领先/落后（如 `+2/-1`）或 `gone`；`worktree show`、`repo show` 输出上游、领先/落后、最近提交与修改统计，`repo show` 另输出上次获取远程时间；`repo branches` 有上游与领先/落后列，无上游时为 `-`。这些数据只读本地引用，不联网；`--json` 与 HTTP 响应一致。
- `repo fetch` 逐行输出每个远程的结果（分类与脱敏同 Web 端），全部成功以 0 结束，任一远程失败或仓库没有远程以 1 结束。
- `worktree prune` 只选未锁定、目录已缺失的失效登记（不执行 `git worktree prune`），把批量预览写到标准错误，确认规则同 `worktree rm`，非交互且没有 `--yes` 时以 2 结束。
  - 没有可清理项以 0 结束；确认后逐项输出 `removed`、`skipped`、`failed`，有 `failed` 时以 1 结束。
- `repo ff`、`repo push`、`repo branch-rm` 与 Web 端调用同一组预览与执行接口，预览输出到标准错误：
  - 有阻止原因时以 1 结束且不做改动；终端上先确认，非交互时需要 `--yes`，否则以 2 结束；
  - `repo ff` 快进检出的分支且工作树下有运行会话时同样需要确认；`repo push` 对已有上游的分支给出 `--remote` 或 `--to` 时以 2 结束；
  - `repo branch-rm --cleanup` 选出未被检出、且上游已删除或已合入默认分支的分支，有独有提交的分支只在加 `--unmerged` 时删除；已删除分支的输出行带恢复命令。
- 初始化指令最多 4000 个字符，超出时 CLI 以 2 拒绝；引用不存在的快捷指令只警告，仍会保存。`--edit` 用 `$VISUAL` 或 `$EDITOR` 编辑。
- 启动配置的变量值默认隐藏；`edit` 读取后只改给出的字段再整体保存；开启聊天视图（`--chat`）前先检查命令是否支持。

## shell 补全

```sh
echo 'source <(webmux completion bash)' >> ~/.bashrc
webmux completion zsh > ~/.zfunc/_webmux
webmux completion fish > ~/.config/fish/completions/webmux.fish
```

补全命令、选项与枚举值，并向运行中的服务查询会话 ID、启动配置、仓库与 worktree；服务未运行或 1 秒内没有响应时只给出静态候选。

## WebMux 工具手册

手册按 llms.txt 约定放在站点根路径，内容与运行中的服务版本一致：

```sh
webmux guide                     # 索引（/llms.txt），列出各主题
webmux guide commands            # 命令目录：每个命令一行的用法与摘要
webmux guide --full              # 全部主题（/llms-full.txt）
webmux guide --path              # 服务端可执行文件与套接字路径
curl -s http://127.0.0.1:7681/llms.txt   # 也可以直接读 HTTP 地址，不需要 Token
```

手册只写命令帮助给不了的内容：能力索引、概念（会话状态、视图、`self`、临时文件夹）、选择命令的依据、跨命令的流程与本实例信息。命令目录在服务启动时由命令表渲染，完整用法仍以 `webmux <命令> --help` 为准，手册不复制选项清单与单命令示例。

手册不写死安装路径：示例中的 `webmux` 指 `"$WEBMUX_BIN"`、指令中给出的绝对路径或 `PATH` 中的 `webmux`。经 TCP 读取的版本不含任何实例信息，经本机套接字读取时附带本实例的版本、可执行文件与套接字。

在快捷指令或仓库初始化指令里写 `{{webmux}}`，会插入一段说明：会话 ID、命令与套接字路径、手册地址，以及本会话临时文件夹的路径和“交付文件放这里而不是项目仓库”的规则。系统预设“WebMux 工具”就是一条正文为 `{{webmux}}` 的快捷指令。`webmux dir self` 输出临时文件夹路径。

## 帮助与退出码

- `webmux <命令> --help` 写到标准输出并以 0 退出，包括用法、参数、选项与默认值、环境变量、输出、退出码和示例。
- 用法错误以 2 退出，标准错误给出原因和 `Run 'webmux <命令> --help' for details.`。
- 选项可以写在位置参数之后（`webmux get self --json`）；`run` 在 `--` 之后的参数原样作为命令。
- 退出码：0 成功；1 操作失败；2 用法错误（含已删除的子命令与选项）；3 服务未运行；124 等待超时。
