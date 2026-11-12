# HTTP API：持久对话历史

使用 Conversation ID 读取已提交的规范化 Activity，不需要启动或恢复 agent。归档后和服务重启后仍可读取。
本接口供历史集成与后续交接使用；实时聊天继续使用原有 WebSocket 缓存。

## 读取一页 Activity

```http
GET /api/conversations/{id}/activities?afterPosition=0&limit=20&maxBytesPerItem=4000
Authorization: Bearer <token>
```

- `afterPosition` 缺省为 `0`，不含当前位置；`limit` 缺省 20，范围 1–100。
- `maxBytesPerItem` 缺省 4000，范围 4–65536；与 limit 的乘积不能超过 256 KiB。
- 返回 `items`、`nextPosition`、`gaps`、`reconcileRequired` 和 `historyOrigin`。
- 无结果时 `items=[]`，`nextPosition` 保持请求游标；追加的新项使用更大的 position，已返回位置不会重排。
- position 只在完整项提交时分配。尚未完成的草稿不会在游标之前留下日后补入的空洞；position 是提交顺序，Run ordinal 保留工作所属回合。

## 读取长项余文

```http
GET /api/conversations/{id}/activities/{activityId}?textOffset=0&maxBytes=4000
```

`textOffset` 缺省 0，`maxBytes` 缺省 4000，范围 4–65536。
偏移按 UTF-8 **字节**计算；返回不切断 code point，`nextTextOffset` 是下一段的起始偏移。
偏移处于多字节字符内部或超出原文时返回 400。只允许读取路径中指定 Conversation 自己的 Activity。

每项包含稳定 `activityId/conversationId/providerThreadId/runId`、`position/runOrdinal`，以及 `kind/role/status/text/source/providerItemId/createdAt`。
`byteLength` 表示完整规范化文本的字节长度，`nextTextOffset` 和 `truncated` 描述本次读取；附件仅返回元数据与来源引用。

## 完整性与导出

- `truncated=true` 仅表示本页未返回全部文字，**不表示数据库原文缺失**。
- `historyOrigin=unknown|bounded_import|complete` 说明捕获来源；升级前记录不伪造完整历史。
- `gaps` 和 `reconcileRequired` 表示 TUI 缺口、崩溃尾部或无法确认的 replay，不能与分页截断混淆。
- `exportable` 和 `exclusionReason` 明确标记 portable projection；生命周期和权限控制项不可导出为用户指令。
- 消息保持 user/agent 角色；命令、工具、错误、中断、文件变化、计划与 compaction 保留明确类型。reasoning、附件正文、raw ACP 和原生工具状态不通过本接口导出。
- 持久 position、ordinal、字节长度和偏移都使用十进制字符串，避免 JavaScript 浮点数损失精度；它们不是实时 chat generation/seq。

## 校验与错误

游标必须是规范的非负十进制 int64 字符串；不接受符号、前导零、指数、溢出、重复参数或未知参数。

| 状态      | 含义                                                              |
| --------- | ----------------------------------------------------------------- |
| 400       | ID、游标、字节边界或分页上限无效                                  |
| 401 / 403 | 缺少认证或 Origin 被拒绝，沿用统一 API 边界                       |
| 404       | Conversation / Activity 不存在，或 Activity 不属于该 Conversation |
| 503       | 对话数据库未启用或不可用                                          |

读取限时 10 秒，响应使用 `Cache-Control: no-store`，正文不写访问日志。删除记录或淘汰会清理关联数据，释放/重建 worktree 不隐式删除；备份与存储边界见[配置与持久化清单](../persistence.md)。
