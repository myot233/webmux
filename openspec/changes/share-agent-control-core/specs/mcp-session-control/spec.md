# Spec Delta

## Purpose

让使用 MCP 的编排客户端通过结构化工具控制 WebMux 持久会话，并正确区分输入回执、指定聊天回合、后台工作和进程退出。
本能力约束重试、观察完整性和冻结历史边界，使不同控制入口不会因为独立解释同一事件而产生相互矛盾的结果。

## ADDED Requirements

### Requirement: 控制工具的边界与发现

MCP 控制服务 SHALL 提供 capabilities、session_list、session_get、session_launch、session_send、session_interrupt、session_read 与 session_wait。
发现 SHALL 列出实际工具和可选启动配置，不启动 agent 探测、不返回配置 argv 或环境值，不提供通用 API 路径或服务管理工具。
工具较少 SHALL NOT 被描述为进程沙箱；启动命令仍以服务用户权限运行。

#### Scenario: 发现不启动会话

- **WHEN** 客户端读取 capabilities 或工具列表
- **THEN** 返回实际工具与配置摘要，不创建会话、不启动登录或模型探测，也不泄露环境值

### Requirement: 逻辑操作重试沿用服务端去重

创建、输入与中断工具 SHALL 要求显式 clientRequestId；同一逻辑操作重试 SHALL 使用同一个 ID 和相同请求内容，新操作 SHALL 使用新 ID。
结果与去重期限 SHALL 沿用服务端，不能承诺服务重启或淘汰后仍只执行一次。
JSON-RPC 请求 ID SHALL 仅关联协议响应，不替代业务请求 ID；部分创建失败 SHALL 保留真实步骤与已创建资源。

#### Scenario: 响应丢失后重试

- **WHEN** 创建响应丢失，客户端在服务端仍保留去重记录时使用相同业务 ID 与内容重试
- **THEN** 返回原操作结果，不另建会话或 worktree，不因辅助链接读取失败把已成功创建报告为失败

### Requirement: 等待只跟踪所选择的回合

turn 等待 SHALL 要求 turnId、queueItemId、firstTurn 三选一；发送和创建的等待选择器 SHALL 来自实际输入回执。
等待 SHALL NOT 用旧回合或后续任意回合替代目标。
firstTurn 在最早回放已淘汰时 SHALL 明确失败；精确 turnId 可使用匹配的当前状态或结束事件恢复观察，并标记缺失回复。

#### Scenario: 已有旧回复不满足新等待

- **WHEN** 回放有旧回合结果，而客户端等待新发送回执的 turnId
- **THEN** 旧回复不结束等待，结果只属于目标 turnId

#### Scenario: 开始事件已淘汰但结束仍可定位

- **WHEN** 指定 turnId 的 start 不在回放中，但匹配的 end 或当前已结束状态仍在
- **THEN** 返回该回合 ended 及真实结束原因，replyTruncated 为 true，不等待无关的新回合

#### Scenario: 首回合无法证明

- **WHEN** firstTurn 等待收到 truncated 或起点晚于首事件的回放
- **THEN** 明确报告首回合历史不可用，不把仍保留的第一个回合当作首回合

### Requirement: 已观察队列项的删除终止等待

排队项出现后，在匹配其启动回合之前消失时，等待 SHALL 及时报告排队输入已移除，不等到超时。
匹配 turn.start 的处理 SHALL 优先于同帧队列消失判断；回合已启动后移出队列 SHALL NOT 被当作删除。
提交后首次观察既没有队列项也没有可定位回合时 SHALL 报告无法定位，而非推断任务完成。

#### Scenario: 另一客户端删除排队项

- **WHEN** session_wait 已看到目标队列项，另一客户端在其发送前删除该项
- **THEN** 等待返回已移除错误，不继续报告 queued 到等待超时

#### Scenario: 正常开始后不误报删除

- **WHEN** 目标 turn.start 与队列项移除出现在同一帧或依次发布
- **THEN** 等待跟随该回合，不报告 queue removed

### Requirement: 请求解决与后台工作不混淆完成

待回答请求 SHALL 以 waiting_for_user 返回实际请求信息，不自动批准；同一回放帧中已解决的请求 SHALL NOT 再阻止等待。
目标回合结束 SHALL 返回 ended、stopReason、error 与 unfinished，目标结束事件优先于旧快照。
end_turn SHALL 只证明回合正常结束；unfinished 为 true 表示仍有后台工作，不等于进程退出或任务验收成功。

#### Scenario: 回放中请求已经回答

- **WHEN** 回放包含同一请求的打开与解决事件，当前没有未解决请求
- **THEN** 不返回 waiting_for_user，继续观察目标回合

#### Scenario: 回合结束后后台工作仍在

- **WHEN** 目标回合以 end_turn 结束且结束事件的 unfinished 为 true，而先前快照为 false
- **THEN** 返回 ended、end_turn 和 unfinished=true，不以旧快照覆盖，不伪造进程退出

### Requirement: 超时和连接结束只结束观察

session_wait SHALL 默认等待 30 秒，最多 60 秒；超时返回最新部分结果与 waitTimedOut=true，不发送中断、不重发输入。
timeoutMs=0 SHALL 在最多十秒的初始连接与回放读取后返回当前观察结果，而不被视为立即超时。
断线或取消 SHALL 明确报告错误，不推断结束；持久会话不因观察结束而终止。

#### Scenario: 无输出直到等待超时

- **WHEN** 目标回合没有结束事件，且等待达到所选时限
- **THEN** 返回 waitTimedOut=true，不生成 stopReason，不以静默报告完成，会话继续运行

#### Scenario: 有界当前状态读取

- **WHEN** timeoutMs 为 0 且初始回放在期限内读完，目标回合仍运行
- **THEN** 返回 running 且 waitTimedOut=false，不等待回合结束

### Requirement: 进程退出与状态变化分离

exit 等待 SHALL 只在目标进程已退出时返回 exited 与真实可用的 exitCode；idle、waiting 或 attention SHALL NOT 代替退出。
目标从运行列表移除或初始不存在时 SHALL 明确报告无法观察，不能推断退出成功或伪造零退出码。
CLI 与 MCP 对同一目标退出事实 SHALL 一致，调用本身成功与进程退出码不同。

#### Scenario: 非零进程退出

- **WHEN** 目标先进入 idle，随后以退出码 7 结束
- **THEN** idle 不满足退出等待，退出后返回 exitCode=7，不改为 0

### Requirement: 回复完整性与持久历史分开表达

MCP 等待回复 SHALL 只返回目标回合最后一条 agent 消息，最多 64 KiB，保留原始分块空白并在 UTF-8 边界截断。
回放缺失或回复裁剪 SHALL 通过 replyTruncated 表达，不因后续 chunk 而清除缺失标记，不导出私有 thought。
activity 读取 SHALL 保留持久位置、gaps、historyOrigin 和 reconcileRequired；等待 SHALL NOT 用 activity 偷换有界回放数据源。

#### Scenario: 多字节文本超过上限

- **WHEN** agent 回复在一个 UTF-8 多字节字符处超过 64 KiB
- **THEN** 返回不超过上限的完整字符前缀和 replyTruncated=true，无关旧回合正文不进入结果

### Requirement: 控制与冻结历史始终分离

控制服务 SHALL NOT 注册 thread_read；交接历史服务 SHALL 仅提供冻结范围内的只读工具，不因共用协议机制而获得控制工具。
协议重构 SHALL 保持现有 stdio 握手、工具错误隔离与 stdout 仅协议的契约，不扩大冻结来源 conversation 或 position。

#### Scenario: 尝试跨服务调用

- **WHEN** 控制客户端调用 thread_read，或冻结历史客户端调用 session_launch
- **THEN** 返回未知工具，冻结历史 Reader 仍拒绝冻结点之后与其他 conversation 的内容
