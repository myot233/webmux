# Spec Delta

## ADDED Requirements

### Requirement: 启动配置完成与介入提醒设置数据模型与校验

启动配置对象及摘要 SHALL 支持可选字段 `attention`（类型为对象或 `null`/省略，省略时表示跟随全局默认配置）。当 `attention` 非空时，对象 MUST 包含以下字段，且超出范围或非法枚举值 MUST 返回 `400 Bad Request`：

- `mode`：取值为 `"all"`、`"urgent"` 或 `"off"`；
- `programStatus`、`agentProtocol`、`notify`、`bell`：布尔值；
- `minTurnSeconds`：整数，范围 `0`–`300`；
- `doneDelaySeconds`：整数，范围 `0`–`60`。

#### Scenario: 创建或更新带有效提醒设置的启动配置

- **WHEN** 客户端提交 `POST /api/launch-configs` 或 `PUT /api/launch-configs/{id}`，且 `attention` 为 `{"mode":"urgent","programStatus":false,"agentProtocol":true,"notify":true,"bell":false,"minTurnSeconds":10,"doneDelaySeconds":3}`
- **THEN** 服务端接受并持久化该配置，且在 `GET /api/launch-configs`、`GET /api/launch-configs/{id}` 和 `GET /api/config` 的启动配置摘要中返回相同的 `attention` 字段

#### Scenario: 省略或置空提醒设置表示跟随全局默认

- **WHEN** 客户端提交启动配置时省略 `attention` 或将 `attention` 设为 `null`，或者服务端加载旧版 `launch-configs.json` 中不含 `attention` 字段的记录
- **THEN** 服务端正常解析该配置，且该启动配置创建的会话跟随服务端全局提醒配置（`program_status`、`agent_protocol`、`bell_attention`）与默认提醒范围（全部提醒，无最短耗时与确认延迟限制）

#### Scenario: 拒绝非法的提醒范围或超出边界的阈值

- **WHEN** 客户端提交启动配置时 `attention.mode` 不属于 `"all"`、`"urgent"`、`"off"`，或 `minTurnSeconds` 小于 `0` / 大于 `300`，或 `doneDelaySeconds` 小于 `0` / 大于 `60`
- **THEN** 服务端返回 `400 Bad Request` 错误，且不修改已持久化的启动配置

### Requirement: 启动配置管理弹窗的完成与介入提醒设置与信号优先级提示

前端启动配置新增与编辑表单 SHALL 提供“完成与介入提醒”可选配置区，默认勾选“跟随全局默认”（对应 `attention = null`）。当取消勾选“跟随全局默认”时，表单 MUST 满足以下交互：

- 提供提醒范围选择（“全部提醒” / “仅需介入” / “关闭”）；
- 在提醒范围不为“关闭”时展示四类信号来源开关，并明确提示信号源优先级（`OSC 7501` 状态协议 > Warp Agent 事件 > 桌面通知 > 终端响铃；高优先级生效时自动忽略低优先级信号）；
- 在提醒范围为“全部提醒”时展示最短回合耗时与完成确认延迟设置。

#### Scenario: 默认勾选跟随全局默认并隐藏细分项

- **WHEN** 用户新建启动配置，或编辑一个 `attention` 未设置的启动配置
- **THEN** “完成与介入提醒”分组中的“跟随全局默认”复选框处于勾选状态，细分配置项不展开，保存时提交 `attention` 为 `undefined`/`null`

#### Scenario: 取消跟随全局默认并查看信号源优先级说明

- **WHEN** 用户在启动配置表单中取消勾选“跟随全局默认”
- **THEN** 表单展示提醒范围分段控件（默认选中“全部提醒”）、四项信号来源开关（`OSC 7501` 状态协议、Agent 事件协议、桌面通知 `OSC 9 / 777`、终端响铃 `BEL`）以及最短回合耗时与完成确认延迟输入框
- **AND** 信号来源区域显式展示优先级提示文案，说明信号按“`OSC 7501` 状态协议 > Agent 事件 > 桌面通知 > 终端响铃”优先级生效，关闭高优先级协议后会自动回退到下一优先级信号

#### Scenario: 切换提醒范围时按需隐藏无关子项

- **WHEN** 用户将提醒范围切换为“仅需介入”
- **THEN** 表单保留四项信号来源开关，但隐藏仅对完成类提醒生效的“最短回合耗时”与“完成确认延迟”输入框
- **WHEN** 用户将提醒范围切换为“关闭”
- **THEN** 表单同时隐藏信号来源开关与防误报阈值输入框
