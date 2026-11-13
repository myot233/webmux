# Spec Delta

## ADDED Requirements

### Requirement: 跨传输共享指南

初始化说明、CLI 指南与 control MCP instructions SHALL 共享操作语义，并按可信上下文裁剪入口。
CLI commands SHALL 继续由命令表生成；MCP SHALL 仅列当前 registry 的真实工具，不混入 freeze-history 的冻结 registry。

#### Scenario: 实际工具列表

- **WHEN** control MCP 注册工具发生变化
- **THEN** 指南和 capabilities 使用相同实际列表，不另维护硬编码清单

### Requirement: 逻辑操作重试与去重边界

指南 SHALL 要求同一逻辑写操作重试复用相同 ID 与负载，新操作才新 ID。
创建去重仅内存 10 分钟；混合命令仅每会话最近 256 项且 30 分钟，任一边界失效、淘汰或重启后不保证去重。
终端提交 SHALL 按独立 receipt、generation 与保留限制检查；原始按键没有通用 exactly-once，不确定时先 inspect。

#### Scenario: 响应丢失

- **WHEN** 创建响应丢失且仍在去重窗口内
- **THEN** 指南要求复用原 ID 和负载；超窗或重启后要求先检查资源，不盲目重发

#### Scenario: 容量先于时间失效

- **WHEN** 原混合请求已不在最近 256 项，即使未过 30 分钟
- **THEN** 指南不承诺不会重做，并要求先查询状态

### Requirement: 四类操作证据与业务验收

指南 SHALL 区分输入 accepted/written、会话状态变化、指定回合结束、进程退出与 exitCode 四类操作证据。
状态变化 SHALL NOT 证明指定回合完成；回合结束 SHALL NOT 证明进程退出。
任务验收是独立的第五类业务证据；idle、end_turn、退出码 0 或回复文本 SHALL NOT 单独证明业务成功。
超时仅停止观察，不取消执行，继续等待保持原目标。

#### Scenario: 观察到 idle

- **WHEN** 会话状态变化为 idle，但没有指定回合结束或进程退出证据
- **THEN** 指南只报告状态变化，不声称回合结束、进程成功退出或任务通过验收

### Requirement: 当前请求授权

指南 SHALL 先查询真实 pending request，区分授权与提问；超时、断线、取消或执行失败 SHALL NOT 被当作存在授权请求的证据。
授权 SHALL 使用实际 optionId，kind 仅为分类；只在用户授权范围内处理当前请求，不扩大到 auto 或未来请求。
终端按 Enter 选择默认项 SHALL 先确认画面中的请求内容与授权范围；MCP 缺少 answer 时 SHALL 说明需用户或已有其他客户端处理。

#### Scenario: 自定义授权 ID

- **WHEN** 当前选项 kind 为 allow_once，optionId 为 custom-approve-7
- **THEN** 使用 custom-approve-7 并核对当前请求和授权范围，不用 kind 替代

#### Scenario: 失败但没有授权请求

- **WHEN** send --reply 超时或断线，随后快照没有 pending permission request
- **THEN** 流程不调用 chat answer，不按 Enter 盲目批准，先查真实状态或报告失败原因

### Requirement: 版本与动态配置发现

指南 SHALL 将 WEBMUX_BIN/绝对路径仅视为文件定位，并要求比较客户端 version 与服务 status。
启动配置 SHALL 从真实列表选择，模型 SHALL 从 agent 公布的配置项及允许值发现，不硬编码 provider/model；能力不足时说明限制，不伪造值。

#### Scenario: 同路径不同版本

- **WHEN** 二进制路径未变但文件已升级替换
- **THEN** 指南仍要求比较 version/status，不声称路径保证版本一致

#### Scenario: 自定义模型配置

- **WHEN** 实例只有自定义启动配置，agent 公布非默认模型值
- **THEN** 流程使用真实 ID 与允许值，不依赖 Codex 或 gpt-5

### Requirement: 显式上下文与文件能力边界

指南 SHALL 区分 transport（cli、mcp-stdio、mcp-http）与 locality（local、remote）。
远程 self SHALL 为 null，不从 env、PID 或调用者自称推断；stdio 也不自动证明会话身份。
服务端绝对路径 SHALL NOT 被描述为远程客户端写入能力，缺少 answer/view/upload 时不提供虚构调用。
TCP llms 路径 SHALL 保持无实例事实的静态文档；本机原有信息边界不变。

#### Scenario: 远程控制上下文

- **WHEN** 远程 MCP 没有文件写入或 upload 能力
- **THEN** 指南返回未知 self 和真实工具列表，不让客户端写服务端路径或调用虚构工具

#### Scenario: 静态公网文档

- **WHEN** TCP 客户端读取任一 llms 路径
- **THEN** 不包含服务版本、路径、会话身份或 Token

### Requirement: MCP 指南输入与主题

control MCP SHALL 提供只读 guide_read，输入为禁止额外字段的 object，仅可选 topic：1 至 64 字符并匹配 `^[a-z][a-z0-9-]*$`。
缺省返回索引；MCP 仅列 connect、operations、completion、requests、launch、artifacts、rules 精选主题，不复制完整 shell 手册。
工具 SHALL NOT 接受 URL、路径或任意命令。

#### Scenario: 按需读取

- **WHEN** 先以 `{}` 调用，再读取 completion
- **THEN** 分别返回索引和四类操作证据及独立业务验收说明

### Requirement: MCP 指南成功结果

成功 structuredContent SHALL 包含 kind（index|topic）、transport、locality、selfSessionId（string|null）、tools（实际工具名数组）、
topics（id/title/summary 对象数组）、topic（string|null）、markdown（string），所有字段必有。
索引 topic 为 null；主题 topics 为空数组。text content SHALL 沿用协议默认 JSON 编码，不增加专属 markdown envelope。

#### Scenario: 结构化结果

- **WHEN** guide_read 成功
- **THEN** 结构化字段完整且文本为相同结果的默认 JSON 表达

### Requirement: MCP 指南错误与大小限制

无效参数、未知主题和完整 UTF-8 编码结果超过 65536 bytes SHALL 沿用现有 handler 的 isError:true 工具错误文本。
文本 SHALL 分别说明参数问题、unknown_topic 与可用主题、guide_too_large 与上限；不得返回部分正文或敏感实例数据。
SHALL NOT 新增 -32602 handler 错误或结构化错误框架。

#### Scenario: 参数与大小边界

- **WHEN** topic 为 null、空值、错误类型、未知字段、合法未知主题或结果超限
- **THEN** 返回对应隔离工具错误文本，不返回成功结果或半篇正文

## MODIFIED Requirements

### Requirement: 手册内容要求

- `artifacts` 主题 SHALL 包含原预置快捷指令“临时文件夹”的全部说明：
  - 用 `webmux dir self` 获取路径；
  - 用户上传的文件在 `uploads/` 子目录中；
  - 有本机文件访问能力时，交给用户的截图、报告、图表、导出数据等交付物保存到该文件夹（可以建子目录），不要放进项目仓库；
  - 用户在 WebMux 代码栏的“临时文件夹”中查看；
  - 完成后告知文件相对于该文件夹的路径；
  - 普通会话移除后清理；退出或 kill 未移除时保留；混合会话移出运行列表但保留可恢复记录时，目录保留且恢复后沿用；
  - forget 或记录淘汰清理，重启按可恢复记录清扫；目录不是长期存储，远程拿到路径不等于客户端可写。
- `recipes` 主题 SHALL 至少包含：
  - 用 `run --new-worktree --view chat --launch-config CONFIG --prompt TEXT --reply` 在新 worktree 中启动 agent 并取回回复；
  - 用 `run --follow` 在持久会话里运行命令并取得退出码；
  - 用 `send --wait` 驱动 shell 会话，用 `screen` 查看 TUI 当前画面；状态变化不证明命令完成或成功；
  - 用 `ls --parent self` 找到并清理自己创建的会话；
  - 长任务结束时 `notify`；
  - 开发服务器启动后 `forward add`。
- `rules` 主题 SHALL 要求：
  - 只操作自己创建（`parent` 为自己）或用户点名的会话；
  - 不结束、不输入用户的其他会话，除非用户要求；
  - 不读取或输出 Token；
  - 不向开启自动批准的 agent 转发未经核实的外部内容；
  - 对全屏 TUI 优先用聊天视图与统一 send 入口，需要看画面时用 `screen`，不依赖 `logs --strip` 的文本；
  - 不确定命令用法时先运行 `webmux <命令> --help` 或 `webmux help --json`，不要猜测选项。

#### Scenario: 临时文件夹说明迁移

- **WHEN** 用户阅读 `/llms/artifacts.md`
- **THEN** 原“临时文件夹”预置指令的交付位置、uploads、子目录、代码栏查看与相对路径规则均保留，路径通过 `webmux dir self` 获取，清理条件按准确生命周期说明

#### Scenario: 交付目录生命周期

- **WHEN** 混合会话 rm/archive、resume、最终 forget 或记录淘汰
- **THEN** 指南说明前两步保留沿用目录，最终清理；普通 rm 则清理，不再笼统说会话删除即清理
