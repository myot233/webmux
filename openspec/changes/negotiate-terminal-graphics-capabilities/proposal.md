## Why

#130 已实现浏览器端 Kitty 图片渲染，并为预置的 omp 启动配置设置 `PI_FORCE_IMAGE_PROTOCOL=kitty`（新安装首次写入及设置页“添加系统预设”补回，见 `internal/launch/presets.go`）。这证明显式启用后的渲染可用，不证明所有应用能在启动时自动发现图形能力。

现有基础：#168 已让会话以 `TERM_PROGRAM=webmux` 和服务版本声明身份，#143 已让会话自身应答 OSC 7501 探测。

缺口：PTY 程序先于浏览器附着启动，Kitty 图形查询与 DA1 的应答仍依赖浏览器，回放也不应补答。因此需要独立规定启动能力声明、查询时序和应答归属。

## What Changes

- 本变更仅提交未来实施的规划；本次合并保留 #130 的实现、omp 专用环境变量和既有配置不覆盖策略，不把下述要求标为已经实现。
- 沿用 #168 已实现的 WebMux 真实终端身份和显式启动变量优先级（`agent-attention` 的“PTY 环境规范化与协议变量注入”），补充不伪造 Kitty 身份、不把图形协议等同于 Kitty 键盘等其他扩展的约束。
- 为虚拟会话建立固定的基础图形能力边界：仅已实现且可验证的直接传输子集；首次启动、无浏览器、断线和接管不改变该边界。
- 让有界、无图像渲染的基础能力查询应答独立于浏览器就绪；同一输出流中 Kitty 查询必须先于后续 DA1 应答，不能用历史重放或迟到应答弥补启动探测。
- 为服务端接管的查询确定唯一应答方，阻止浏览器或原生 attach 再次提交重复应答；其他依赖当前屏幕或视口的设备查询继续遵循既有控制权规则。
- 明确环境提示、标准探测和应用专用强制开关的区别；不承诺仅凭新的终端身份就让所有应用自动启用图片。

## Capabilities

### New Capabilities

- `terminal-graphics-negotiation`：会话级图形能力边界、启动及离线查询、FIFO 应答、回放静默和多客户端兼容性。

### Modified Capabilities

- `session-launch-environment`：在已实现的真实终端身份及显式环境优先规则之上，补充图形相关的启动声明约束（不伪造 Kitty 身份、不全局注入应用专用图形变量），不修改原有 argv、配置引用与环境验证契约。
- `terminal-control`：对服务端接管的基础查询设置单一应答权威，保留其余设备自动回复和输入/尺寸的控制端语义。
- `program-status`：把启动探测场景中的 DA1 结果从无人应答改为会话级应答，与 `terminal-control` 的单一权威保持一致。

## Impact

- 未来的服务端实现涉及：
  - `internal/session` 的实时读取与查询识别：`osc.go` 现仅解析 OSC，Kitty APC 需另行识别；
  - `io.go` 的有界应答队列与 PTY 输入顺序，复用 #143 的 `queueProbeReplyLocked`、`flushRepliesGated` 与 `replyLoop`；
  - `run.go` 的运行生命周期，`control.go` 的应答连接判定；
  - `internal/hub`、`internal/contract`、`internal/wire` 的自动应答边界；如新增 JSON 字段，须与 `web/src/contracts/schemas.ts` 和消费者同时迁移。
- 终端身份已在 `internal/session/env.go` 落地，无需再改。
- 浏览器涉及两条应答路径：
  - 图形应答：`web/src/domains/terminal/kitty-graphics.ts`、`web/src/hooks/terminal/terminal-graphics.ts` 与 `terminal-socket.svelte.ts`；
  - xterm 内核的 DA1 应答：`terminal-input.ts` → `terminal.svelte.ts` → `sendReply`。
- 原生客户端涉及 `internal/attach` 的自动设备回复处理；现有 `probe.go` 仅在输出侧过滤 7501 探测。
- 仅提取必要的有界查询识别，不在 Go 后端构建完整终端模拟器或图片渲染器。配置转换和服务组装继续只由 `cmd/` 完成。
- 不新增依赖，不改变会话持久性、输入权限、认证、Origin 或现有回放截断边界。未来实施须更新 US-D12、US-D06（`docs/user-stories/terminal.md`）、US-B08（`docs/user-stories/access-and-sessions.md`）等受影响故事；本规划不新增“已实现”故事。
