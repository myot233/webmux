# 需要你介入的提醒

当 CLI agent 在终端里停下来向你提问、请求工具授权、完成本轮任务或出错停止时，WebMux 可以识别它发出的信号，在**会话总览、浏览器标签页与手机锁屏推送**中提醒你及时接管。

## 开箱即用的 Agent

以下常用 CLI agent 在新版本中无需额外配置，启动后即可自动报告状态与介入提醒：

- **omp**、**Codex CLI**（0.164.0 及以上）：在终端中直接报告 `OSC 7501` 状态。
  - omp 额外说明：webmux 还默认在会话环境中设置 `WARP_CLI_AGENT_PROTOCOL_VERSION=1`，omp 会同时发出 Warp 结构化事件；关闭 `agent_protocol` 后 omp 改用响铃报告提问和完成。
- **Claude Code**（2.1.295 及以上）：在启动时发出 `OSC 7501;?` 探测，得到 webmux 的应答后自动报告状态。如果启动 webmux 的环境中设置了 `CLAUDE_CODE_DISABLE_TERMINAL_TITLE`，该变量会传入会话，Claude Code 将不再报告状态。
- **其他支持 `OSC 7501` 的程序**（例如按其源码，OpenCode 2.x）：同样无需配置。

如果你使用的是较早版本的 Claude Code 或 Codex CLI，见本页末尾的[旧版本配置](#旧版本配置)。

## 提醒种类与信号来源

程序主动发出的终端信号优先于普通输出启发式：一旦收到，会话显示具体提醒并保持“等待”，即使 agent 仍在刷新 spinner、计时器或状态栏。

| 提醒     | 来源                                                                                            |
| -------- | ----------------------------------------------------------------------------------------------- |
| 需要回答 | `OSC 7501` 状态 `blocked`（`kind=question`、`auth` 或未注明）；Warp 结构化事件 `question_asked` |
| 等待授权 | `OSC 7501` 状态 `blocked` 且 `kind=permission`；Warp 结构化事件 `permission_request`            |
| 本轮完成 | `OSC 7501` 状态 `done`；Warp 结构化事件 `stop` / `idle_prompt`                                  |
| 出错停止 | `OSC 7501` 状态 `error`；Warp 结构化事件 `stop_failure`                                         |
| 有通知   | 桌面通知序列 `OSC 9` / `OSC 777;notify`                                                         |
| 响铃提醒 | 终端响铃（BEL），在你最后一次输入 1 秒后才计入，避开补全响铃                                    |

其中，**需要回答**、**等待授权**与**出错停止**在界面上以醒目的红色徽章提示，其余等待状态显示琥珀色。

## 提醒如何影响会话状态

[会话状态](sessions.md#会话状态) 的启发式只在程序没有报告状态时使用。会话状态按下面的顺序判定：

1. 进程已退出：已退出。
2. 程序通过 `OSC 7501` 报告了状态：
   - 工作中显示“运行中”（超过 `idle_after_seconds` 没有任何输出时视为空闲）；
   - 阻塞显示等待，空闲显示“空闲”；
   - 完成或出错在提醒未处理前显示等待，处理后显示“空闲”。
3. 存在介入提醒：等待。
4. 程序曾发出 `OSC 7501` 或 Warp 结构化事件：这类 agent 总在读终端，末行也可能像提示符，因此不再使用“可能在等你”的启发式，只按输出新旧显示运行中或空闲。
5. 其他：会话状态表的启发式。

## 优先级与清除规则

`OSC 7501` 是 [Program Status Protocol](https://www.superlogical.com/rex/docs/build/program-status)：程序直接报告自己处于空闲、工作中、阻塞、完成还是出错。

- webmux 只读取无 `id` 的根记录中的 `state` 与 `kind`，`msg`、`title` 等正文从不解析、保存或下发。
- 程序发出 `OSC 7501;?` 探测时，由 webmux 服务端立即应答，即使没有浏览器打开该会话也一样。
- 结构化事件使用 Warp 的 `OSC 777;notify;warp://cli-agent;{json}` 格式，webmux 只读取其中的事件名。

同一会话可能同时发出多种信号，按来源分层合并，优先级为 `OSC 7501` 状态协议 > Warp 结构化事件 > `OSC 9` / `OSC 777` 桌面通知 > 终端响铃（BEL）：

1. **状态协议（`OSC 7501`）**：程序报告了 `OSC 7501` 状态后，其状态变化决定提醒（例如从“等待授权”转为工作中会清除提醒），其他来源不再改变提醒。
2. **Agent 事件（Warp 结构化事件）**：当会话未启用或尚未收到 `OSC 7501` 状态时生效：
   - WebMux 记录当前主会话的 `session_id`（取自首次 `session_start` 或每次 `prompt_submit`）；
   - 子代理（`session_id` 不同）发出的 `stop`、`idle_prompt`、`stop_failure` 与 `subagent_start` 会被自动忽略，不会提前触发主会话的“本轮完成”；
   - 子代理的提问（`question_asked`）与权限请求（`permission_request`）仍会正常提醒。
3. **桌面通知（`OSC 9` / `OSC 777;notify`）**：在未触发前两层协议时产生“有通知”提醒。
4. **终端响铃（BEL）**：最低优先级，仅在最后一次输入 1 秒后且无更高层提醒时产生“响铃提醒”。

- 较低层的信号不会覆盖或清除较高层的提醒；如果在启动配置中关闭了较高优先级的信号源，会话会自动回退到下一优先级的信号源。
- 启发式只影响[会话状态](sessions.md#会话状态)，从不产生提醒或推送。

## 按启动配置调整提醒与防误报

如果某个 CLI 工具或工作流容易产生误报（例如频繁响铃、子代理在无 `session_id` 的 `OSC 7501` 上提前报告 `done`、后台命令未结束就切回空闲，或你不希望收到“本轮完成”提醒），可以在设置页编辑对应[启动配置](sessions.md#编辑选项)，展开「完成与介入提醒」并取消勾选“跟随全局默认”：

- **提醒范围**：
  - **全部提醒**（默认）：保留完成、桌面通知、响铃及所有介入提醒；
  - **仅需介入**：只保留需要回答、等待授权与出错停止三类紧急提醒，自动抑制本轮完成、普通通知与终端响铃（终端端与混合聊天端同步生效）；
  - **关闭**：关闭该启动配置会话的一切自动提醒（脚本显式调用 `webmux notify` 仍可触发）。
- **信号来源开关**：按会话单独开关状态协议（`OSC 7501`）、Agent 事件（Warp 协议）、桌面通知（`OSC 9` / `777`）与终端响铃（`BEL`）。
  - 例如：**omp** 在运行子代理时若其 `OSC 7501` 根状态因计数归零提前报告 `done`，可在 omp 启动配置中关闭“状态协议 (OSC 7501)”并保留“Agent 事件 (Warp 协议)”，利用 Warp 事件的 `session_id` 过滤子代理完成信号。
- **防误报过滤**（“全部提醒”下可用）：
  - **最短回合耗时（0–300 秒）**：回合从开始到完成的耗时低于该值时，不触发本轮完成、普通通知或响铃提醒；
  - **完成确认延迟（0–60 秒）**：收到完成、通知或响铃信号后先等待该秒数，期间若程序恢复工作、用户输入或会话退出则自动取消，避免短暂切换造成的抢跑提醒。

**提醒何时清除：**

- 你向该会话输入（终端自动应答除外）；
- `OSC 7501` 报告工作中、空闲或清除；
- 没有 `OSC 7501` 状态时 agent 发出重新开始的 Warp 事件；
- 进程退出。

**程序报告的状态何时作废：**

- 工作中或阻塞状态会在 shell 提示符（`OSC 133;A`）出现时作废，完全重置（RIS）会丢弃全部状态。
- agent 正常退出时会自行清除状态。
- 如果它在等待授权或回答时被强制结束（如 `kill -9`），而 shell 没有启用发出 `OSC 133;A` 的 shell 集成，会话会一直显示等待，直到 shell 集成作废该状态、程序再次报告或会话退出。
- 输出和浏览器断线重连都不会清除提醒；重复报告同一状态不会在你处理后再次提醒。

## 旧版本配置

**Claude Code**（2.1.141 至 2.1.294）：在 `~/.claude/settings.json` 中注册 hook，由 Claude Code 代为输出事件序列：

```json
{
  "hooks": {
    "Notification": [
      {
        "matcher": "permission_prompt|idle_prompt|agent_needs_input|elicitation_dialog",
        "hooks": [
          {
            "type": "command",
            "command": "~/.claude/hooks/webmux-attention.sh"
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "~/.claude/hooks/webmux-attention.sh"
          }
        ]
      }
    ]
  }
}
```

```bash
#!/usr/bin/env bash
# ~/.claude/hooks/webmux-attention.sh — silent outside webmux sessions; needs jq.
[ -n "$WEBMUX_SESSION_ID" ] || exit 0
input=$(cat)
case "$(jq -r '.hook_event_name' <<<"$input")" in
  Notification)
    case "$(jq -r '.notification_type // empty' <<<"$input")" in
      permission_prompt) event=permission_request ;;
      agent_needs_input | elicitation_dialog) event=question_asked ;;
      idle_prompt) event=idle_prompt ;;
      *) exit 0 ;;
    esac
    ;;
  Stop) event=stop ;;
  *) exit 0 ;;
esac
seq=$(printf '\033]777;notify;warp://cli-agent;{"v":1,"agent":"claude","event":"%s"}\007' "$event")
jq -nc --arg seq "$seq" '{terminalSequence: $seq}'
```

**Codex CLI**（0.164.0 以前）：在 `~/.codex/config.toml` 的 `[tui]` 中设置 `notifications = true`，Codex 在回合结束和请求授权时发出 `OSC 9` 或响铃，webmux 分别显示“有通知”或“响铃提醒”。

其他程序只要输出上述任一序列即可获得提醒，不需要 webmux 适配。设置 `program_status = false` 可停用 `OSC 7501` 的识别与探测应答。
