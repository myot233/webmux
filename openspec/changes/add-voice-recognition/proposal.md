## Why

在手机或桌面端使用 WebMux 与 CLI agent 对话（聊天模式）或向终端输入复杂指令（终端模式）时，长段技术描述往往包含大量中英混排术语，键盘打字效率较低。同时，不同用户偏好的语音识别（ASR）服务提供商各不相同，且涉及各自的 API Key 或外部脚本环境，WebMux 不应内置绑定特定云厂商 SDK，也不应接管用户的凭据管理。

将语音识别设计为基于 `STDIN / STDOUT` 的可插拔外部子进程模块，并在配置文件中仅提供启动命令、参数与环境变量配置，能同时满足解耦与易用性：

- 用户可以用任意可执行文件（如 `node /path/to/provider.js`）接入自己选定的语音识别服务；
- 浏览器端在聊天模式（输入框旁）和终端模式（快捷栏内）统一提供“点击开始录音（加载 -> 显示实时音量） -> 再次点击完成识别（加载 -> 写入文本且不自动回车）”的交互体验。

## What Changes

- **配置与公开能力标识（`internal/config`、`internal/contract`、`web/src/contracts/schemas.ts`）**：
  - `config.toml` 新增可选 `[voice]` 配置段，包含 `cmd`（字符串数组，首项为可执行文件路径或名称，其余为参数，不经 shell 展开）与 `env`（`KEY=VALUE` 环境变量数组，遵循 `contract.ValidEnvName` 与 `!contract.ReservedEnv` 校验）。
  - `GET /api/config` 响应新增布尔字段 `voice`，仅表示服务端是否配置并启用了语音识别，不暴露 `cmd` 路径或 `env` 密钥。
- **基于 `STDIN / STDOUT` 的可插拔语音识别服务（`internal/voice`）**：
  - 新增 Service 层包 `internal/voice`（登记于 `internal/archtest/layers_test.go`），在每次录音会话开始时按配置的 `cmd` 和 `env` 启动独立子进程。
  - **输入协议（`STDIN`）**：向子进程 `stdin` 持续写入 `16000 Hz`、单声道（Mono）、16-bit 有符号小端（`s16le`）原始 PCM 字节流；用户点击结束录音时关闭 `stdin`（发送 `EOF`），通知子进程刷新缓冲区并输出最终识别结果。
  - **输出协议（`STDOUT`）**：按行读取子进程 `stdout`，支持 NDJSON 事件（`{"type":"ready"}`、`{"type":"partial","text":"..."}`、`{"type":"final","text":"..."}`、`{"type":"error","error":"..."}`），同时兼容纯文本输出（子进程以退出码 `0` 结束且输出非 JSON 文本时自动作为最终结果）。
- **语音识别 WebSocket 通道（`internal/api`）**：
  - 新增受认证与 Origin 检查保护的 `GET /ws/voice` 端点。
  - 接收浏览器发送的二进制 PCM 音频帧并转发至子进程 `stdin`；接收客户端 `{"type":"stop"}` 控制帧并关闭子进程 `stdin`；将子进程产生的 `ready`、`partial`、`final`、`error` 事件以 JSON 帧推送给浏览器。
- **浏览器音频采集与双模式录音组件（`web/src`）**：
  - 新增 `web/src/hooks/shared/voice.svelte.ts` 与 `web/src/components/shared/VoiceButton.svelte`，仅在 `config.voice === true` 时显示。
  - **点击切换（Toggle）四阶段交互流程**：
    1. 点击按钮进入**连接加载态（`connecting`）**，请求麦克风权限、初始化 `16kHz` 音频采集并连接 `/ws/voice`；
    2. 连接就绪后进入**录音态（`recording`）**，实时计算并显示当前麦克风音量大小（`level`）；
    3. 再次点击按钮停止麦克风采集并发送 `{"type":"stop"}`，进入**识别完成加载态（`finishing`）**；
    4. 收到最终文本后恢复空闲态，并将识别内容插入当前输入框或终端，**绝不自动追加回车（`\r`）或自动提交**。
  - **挂载位置**：
    - **聊天模式**：挂载在会话聊天输入框（`ChatComposer.svelte`）和首页“新聊天”输入框（`QuickStart.svelte`）操作栏旁。
    - **终端模式**：挂载在终端底部软键快捷栏（`SoftKeys.svelte`）、桌面端顶栏快捷区（`TerminalHeader.svelte`）以及终端草稿输入框（`TerminalInputComposer.svelte`）中。
- **文档与协议说明（`docs/`）**：
  - 在配置文档与 WebSocket 协议文档中完整记录 `[voice]` 配置项及基于 `STDIN / STDOUT` 的语音识别协议细节，方便用户自行编写或对接外部识别脚本。

## Capabilities

### New Capabilities

- `voice-recognition`: 基于 `STDIN / STDOUT` 可插拔外部子进程的流式语音识别服务、`/ws/voice` 传输协议，以及聊天模式与终端模式下的点击录音、实时音量指示与结果写入交互。

### Modified Capabilities

<!-- 无现有 capability 的需求级变更 -->

## Impact

- 后端：`internal/config`、`internal/contract`、`internal/voice`（新增）、`internal/api`、`cmd/internal/assemble`、`internal/archtest/layers_test.go`。
- 前端：
  - 契约与核心组件：`web/src/contracts/schemas.ts`、`web/src/hooks/shared/voice.svelte.ts`（新增）、`web/src/components/shared/VoiceButton.svelte`（新增）；
  - 聊天模式挂载点：`web/src/components/chat/ChatComposer.svelte`、`web/src/pages/QuickStart.svelte`；
  - 终端模式挂载点：`web/src/components/terminal/SoftKeys.svelte`、`web/src/components/terminal/TerminalViewport.svelte`、`web/src/components/terminal/TerminalHeader.svelte`、`web/src/components/terminal/TerminalInputComposer.svelte`、`web/src/components/terminal/SessionTerminal.svelte`。
- 文档与用户故事：`deploy/config.example.toml`、`docs/deployment/configuration.md`、`docs/protocol/websocket.md`、`docs/specs.md`、`docs/user-stories/`。
