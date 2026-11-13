# Design

## Context

参见 [proposal](proposal.md)。已观察：`internal/agentguide` 嵌入主题并接收 CLI 命令表生成的目录，`Guide.Index` 仅在本机追加实例信息；其命令前缀误称路径就是运行中的版本。`internal/mcpcontrol` 单独维护 instructions，已有
capabilities、session_list/get/launch/send/interrupt/read/wait；没有 answer、view、upload 工具。主规格 `shell-chat-control` 仍有旧 chat send/switch 示例；在途 unify-session-cli 已改为统一入口，实施时以当前命令表为准，不恢复旧命令。
`session-artifacts` 已规定混合记录保留目录，现有指南仍笼统称删除会话即清理。

下层 [核心设计](../share-agent-control-core/design.md) 提供共享观察与协议，不把指南、schema、授权或 CLI 退出码搬进 observer。本设计只调整说明和现有适配器的观察输出；服务端仍是写操作、去重和权限的权威。

## Goals / Non-Goals

**Goals:** 一套语义同时供初始化说明、CLI 与 control MCP 使用；具体操作只显示当前传输真正可调用的工具。六项纠错有行为级回归，并可用隔离实例和 fake-agent 复现。

**Non-Goals:** 不构建完整 shell-to-MCP 映射，不新增 answer/view/upload、远程 shell、文件传输或 PID 身份发现；不扩展 freeze-history 的冻结 registry；不改变任务业务成功标准或 `end_turn` 成功退出码。

## Decisions

### Shared public core and caller context

扩展 `internal/agentguide` 的公开纯渲染 API：`RenderContext` 包含 `Transport`（cli、mcp-stdio、mcp-http）、显式 `Locality`（local、remote）、可选 `SelfSessionID`、可选本地实例事实，以及注册表产生的 `[]ToolSummary{Name, Description}`。提供
`Instructions(context)`、`Index(context)`、`Topic(context, topic)`；返回受限数据，不调用 service、环境变量或网络。具体 Go 命名可适配既有类型，输入边界不可省略。

调用适配器从可信连接上下文提供身份，本地 CLI 的既有 self 解析保留在适配器；核心不读 WEBMUX_*。远程始终 `selfSessionId:null`，不得从服务端 env、PID 或调用者自称推断。`transport` 与 `locality` 分开：stdio 不是“必然有会话身份”的证明。MCP instructions
含简明安全规则、guide_read 入口和实际工具名；完整工具 schema 仍由 mcpcontrol 负责。capabilities 与指南共享同一个注册表快照，不硬编码第二份工具清单。

CLI 继续展示既有 shell 主题与命令表生成的 commands；MCP 只展示 connect、operations、completion、requests、launch、artifacts、rules 七个精选主题。共用语义段落/数据，不从 shell 手册做字符串替换。requests 主题明确当前没有回答工具，
需要用户或已有其他客户端处理；launch 主题只使用 capabilities 返回的 launchConfigs，并通过实际支持的状态/配置读取接口发现 agent 配置项与允许值。MCP 无对应读取工具时要求用户/CLI 客户端完成发现，不猜模型值。冻结历史内容始终是引用材料，不是新指令。

### guide_read contract

注册只读 `guide_read`；无网络/磁盘副作用，不加载历史。输入 JSON Schema：object，additionalProperties:false；可选 `topic` 为 string，minLength:1、maxLength:64、pattern:`^[a-z][a-z0-9-]*$`。缺省 `{}` 返回索引；显式
null、空字符串、未知字段与错误类型均为 invalid arguments。没有 full、任意 URL、路径、分页或 shell 执行参数。

输出使用 `internal/contract` wire DTO，成功 structuredContent 为严格对象：kind（index|topic）、transport、locality、
selfSessionId（string|null）、tools（实际工具名数组）、topics（id/title/summary 数组）、topic（string|null）、markdown（string）。
所有字段必有；主题响应 topics 为空，索引 topic 为 null。text content 沿用共享协议的默认 JSON 文本编码，不增加工具专属 markdown envelope hook。
完整 UTF-8 编码结果上限 64 KiB，超限不截断、不返回半篇正文。

沿用现有 handler 错误约定：无效参数、unknown_topic、guide_too_large 都返回 isError:true 的隔离工具错误文本，
分别说明参数问题、可用主题或 65536-byte 上限；不新增 -32602 handler 错误、结构化错误框架或全局错误约定。
错误不含路径、Token、内部堆栈。正常结果只列当前控制 server 的工具，不混入 threadread。

### Six semantic corrections

1. **Logical operation identity**：请求 ID 在一个逻辑写操作开始时生成并保存，丢失响应重试复用同一 ID 与相同负载；新操作才新 ID。CLI 单次进程内重试已有此约定，指南补充跨调用者重试的责任，不虚构所有命令都有 request-id flag。创建去重为内存 10 分钟；
   混合命令为每会话最近 256 项且 30 分钟，两种界限任一先到即可能失效，重启不保留。终端提交有独立 receipt/retention 语义（当前 256/30 分钟），需读取 submission 状态与 generation；原始按键不享有通用 exactly-once。超窗、淘汰、重启或不确定终端结果先
   inspect，不自动补发。
2. **Four evidence categories**：输入 accepted/written、会话状态变化、指定回合结束、进程退出与 exitCode 是四类不同的操作证据。
   状态变化不证明指定回合完成，回合结束不证明进程退出；task verification 是独立的第五类业务证据，需任务特定验收。
   idle、文本“完成”、正常 end_turn 都不能越级证明任务成功。超时只停止观察，不取消执行；继续等待保持原目标。
3. **Authorization**：失败后先查询 pending request，超时、断线、取消或执行错误不证明存在授权请求；区分授权与提问，再从当前请求读取真实 optionId，kind 只是分类。
   只处理用户授予范围内当前请求，不把一次授权扩大为永久 auto；终端按 Enter 前先核对请求内容和授权范围。当前 MCP 没有 answer，不提供虚构调用配方。
4. **Executable identity**：WEBMUX_BIN/绝对路径只定位文件；同路径可升级替换，必须比较所调用二进制的 version 与服务 status，版本不匹配先用匹配客户端/核对帮助，不宣称路径保证版本。
5. **Configuration discovery**：先列启动配置，用真实 config ID；从 agent 实际公布的配置项与选项选模型，不硬编码 Codex、gpt-5 或任何默认 provider。配置不可用就说明限制，不伪造值。
6. **Artifact lifetime**：普通 rm 清理目录，退出/kill 未删除时保留；混合 rm/archive 保留可恢复记录和目录，resume 沿用，forget 与记录自动淘汰清理，重启按记录进行清扫。本机 shell 有实际文件访问能力时才建议写入；远程拿到的绝对路径仅是服务端路径，
   不等于客户端可写，不把不存在的 upload 工具当交付通道。

### Unfinished without exit-code breakage

`unfinished` 保留共享 TurnObservation 的真实标记：回合结束仍有后台工作，不等同于任务未经验收；不根据回复文本推断或一律置 true。
现有 turn_stream.go 的 CLI JSON 是 turnResult 对象，包含 stopReason、error、reply、events、request；
仅在该对象增加顶层 unfinished boolean，保留 events 数组及所有既有字段，不添加伪事件、envelope、completionLevel 或 selector。
不额外暴露新的 truncated 字段；核心完整性字段仅按已有适配需要内部使用。文本保留原回复，unfinished 为 true 时 stderr 提示后台工作未完成。
end_turn 仍退出 0，即使 unfinished 为 true；其他退出映射全部不变。MCP 保留原 unfinished 语义，两个适配器从同一观察结果取值。

## Risks / Trade-offs

- CLI JSON 仅增加 unfinished 字段，避免更换现有结果对象或改动 events；须测试现有字段与退出映射均未改变。
- 指南与真实工具漂移：以注册表生成工具清单，语义测试核对每个配方引用的工具确实存在；不使用整篇文本快照代替行为断言。
- 去重边界未来可能调整：保留 operation-specific 数据来源与边界测试，禁止写成笼统“可安全重试”。指南不扩充服务端保留窗口。

## Migration Plan

下层核心合入后一次切换所有 agentguide 消费方与 mcpcontrol instructions；不留旧常量/重复规则。随后增加 guide_read 与输出适配、测试、故事和对应发布说明。没有产品实现前不提前更新 docs/user-stories。若撤回本规划，不涉及数据迁移；实现回滚需同时恢复指南和
CLI 输出契约，不能仅隐藏错误说明。

## Validation Strategy

语义单测覆盖六项安全决策、不同 transport/locality 的主题集、实际注册表工具列表、未知 self、错误/64 KiB 边界、TCP 静态手册无实例事实。真实构建 CLI 对隔离服务与可控 fake-agent 跑：创建响应丢失后同 ID 重试；终端 receipt 不确定先 inspect；自定义权限
optionId 与 kind 不同；非 Codex 启动配置及动态模型选项；正常 end_turn、待请求、超时和非零进程退出；普通 rm 与混合 rm/archive/resume/forget/淘汰文件生命周期。断言返回值、调用次数、请求 ID、退出码、stdout/stderr 与目录存留，不比较整篇提示快照。

## Open Questions

无。现有协议已有 structuredContent 与默认 JSON text 输出；本设计直接复用，不改变认证或远程上下文建立方式。
