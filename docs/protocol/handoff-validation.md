# Portable handoff 验收记录

本页记录 #506–#515 的故障边界与 2026-11-11 的实测口径。
交互说明见[交接操作](../hybrid-sessions/handoff.md)，协议分别见[历史](http-conversation.md)、[预览与投递](http-handoff.md)、[原生注入](handoff-native.md)和[只读 MCP](handoff-mcp.md)。

## 故障矩阵

测试使用私有临时状态、独立端口、fake provider 和真实 SQLite/stdio。
部分故障通过事务回调或阶段暂停注入；标有“子进程”的项目实际终止进程，再重开数据库。
断言覆盖固定身份、资源、调用次数与 unknown，不能用最终出现回复代替幂等证明。

| 边界                           | 注入方式与可观察结果                                                                                | 自动化依据                                                 |
| ------------------------------ | --------------------------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| Activity chunk / position 提交 | 提交回调失败；草稿不可见，重开标 crash gap，下次完成项无 position 空洞                              | conversation/activity_test.go                              |
| handoff 事务提交               | 子进程在提交前退出；重开无半条 handoff 或 receipt                                                   | conversation/store_test.go                                 |
| worktree / session 资源创建    | receipt、worktree_created、session_created 阶段暂停并重开；使用相同 runtime/工作树，不重复创建      | handoff/delivery_test.go                                   |
| inline prompt 调用前后         | before_prompt、after_prompt；未 claim 可继续，已 claim 不能重新调用，结果不明保留 unknown           | handoff/delivery_test.go                                   |
| 成功状态提交前                 | 子进程在 FinishExecution 提交前退出；原 receipt 和 execution 保留，状态 unknown，再 claim 被拒绝    | conversation/hardening_test.go                             |
| native inject 前后             | 独立 fake native 共享线程存储；明确 MethodNotFound 才回退，注入后丢确认保持 unknown                 | handoff/native_test.go                                     |
| 原生 load 对账                 | 错误线程、部分或错序证明拒绝；完整有序证明只确认历史，不推断用户 prompt 已发送                      | conversation/injection_test.go                             |
| inline load 对账               | 精确 native ID、完整 packet hash 和稳定 user ID；正常用户引用不被全局 hash 去除                     | conversation/execution_test.go、hybrid/activity_test.go    |
| 旧连接与后台工作               | epoch 拒绝旧连接；原 Run 的工具/compaction 迟到完成仍归原 Run，仅完成后发布 position                | hybrid/activity_test.go、hybrid/switch_test.go             |
| 同一对话 A→B→A                 | 保留原 native/runtime；按 committed position 取 delta，gap 回退 full 仍排除已交付项目               | handoff/provider_switch_test.go                            |
| 跨设备编辑与发送               | revision 409；旧轮询不覆盖 autosave，未审阅 revision 不发送，等待保存时锁定编辑                     | web/test/e2e/handoff.spec.ts                               |
| 来源删除与 unknown             | 来源操作 404，目标最小凭据继续阻止发送；实际 target replay 可解锁，不重建来源操作                   | handoff/delivery_test.go、conversation/hardening_test.go   |
| 存储不可用                     | 损坏、未来版本、锁冲突保留原库；query_only / max_page_count 模拟只读与磁盘满，失败前不发布 Activity | conversation/store_test.go、conversation/hardening_test.go |

Go 测试路径相对 [`internal/`](../../internal)，浏览器测试路径相对仓库根目录。
磁盘满测试限制 SQLite 页数；没有对宿主机磁盘实际写满。
普通终端在对话库损坏时仍可创建；存储错误不输出 SQL 值或正文。

## 迁移、保留与删除

- 旧 `hybrid-sessions.json` 只迁移身份；原 argv、native ID 与恢复语义保留，历史为 unknown，不读取 agent transcript 猜测补全。
- v9 已删除来源的目标从独立 receipt / consumed Run 重建最小 admission 事实；prompt 或 native 结果未决时允许精确目标 load 对账，不能当作成功或重发。没有 prompt receipt 且无未决原生注入时取消已删除的未发送操作。
- release / rebuild 标记、启动配置删除和重启保留 Activity 与未发送草稿；配置失效使草稿 stale。
- forget / MaxRecords 淘汰清理最后运行绑定所属的历史、来源 handoff 与命令记录；尚有同一 Conversation 的其他 provider 绑定时保留共享历史。
- 来源删除后，目标持有的 native 注入摘要、inline receipt、附件引用、占用估算与 unknown gate 按目标生命周期保留。
- 未开始执行的孤立 fork 草稿清理；已 claim 或有原生/Activity 事实的目标不能当成空草稿删除。
- runtime 未发布标记保留创建窗口；删除意图使 JSON 删除后崩溃可由下一次启动清理，避免 managed runtime 永久残留。
- 资源准备持有 Conversation claim，forget 在关闭 writer 之前拒绝竞争，避免后台任务复活已删除资源。

依据：`hybrid/conversation_test.go`、`handoff/retention_test.go` 与 `conversation/hardening_test.go`。
数据库、锁和 sidecar 的权限/符号链接边界沿用 `statefile` 契约测试。
完整生命周期、备份与敏感数据分类见[持久化清单](../persistence.md)。

## 全局关闭

`history_capture=false` 关闭新规范化正文写入、历史 REST、handoff 和 MCP；相关 API 返回 503。
正常聊天继续工作；身份和未决投递保护仍持久化，既有正文不会自动删除。
关闭区间留下 gap，重新开启后不从 load replay 回填该区间正文，新实时消息可以继续捕获。
这不是外部 agent transcript 的关闭开关，也不删除旧备份、文件系统快照或浏览器离线草稿。
配置方式见[部署配置](../deployment/configuration.md)。

## 读写与 selector 实测

环境：Linux amd64，Intel Core Ultra 7 270K Plus，Go 1.26.10，`GOMAXPROCS=4`，纯 Go SQLite。
每项使用新建私有目录。下表主结果在 `/home/myot/.cache` 所在的 btrfs 上测量；`/tmp` 是 tmpfs，不能当作真实磁盘吞吐量。
RSS 为整个独立基准子进程的 `ru_maxrss`，包括 Go 运行时、SQLite 缓存与 fixture；分配量为 Go benchmark 的 B/op。

| 基准          | 次数与数据                                    | 时间 / op        | 分配 / op               | 峰值 RSS   |
| ------------- | --------------------------------------------- | ---------------- | ----------------------- | ---------- |
| Activity 写入 | 10,000 项，每项 2,000 B，FULL 同步            | 3.229 ms         | 9,371 B，194 allocs     | 24,020 KiB |
| 历史分页      | 10,000 项中从 position 5,000 读 20 项，500 次 | 0.999 ms         | 373,243 B，2,972 allocs | 24,576 KiB |
| 长流式消息    | 1 MiB，256 个 4 KiB chunk                     | 2.562 ms / chunk | 7,054 B，75 allocs      | 19,660 KiB |
| selector      | 10,000 个 32 字符逻辑 ID，500 次              | 1.185 ms         | 3,890,623 B，213 allocs | 33,352 KiB |

写入为每秒约 0.62 MB，流式约 1.60 MB。分页是重复读取同一页的暖缓存结果；没有宣称随机冷盘性能。
selector fixture 使用预先计算的 2,000 B 正文成本，不载入全文，与生产 metadata 选择路径一致。
分页 setup 会写入全部 10,000 项，未计入 ns/op；其独立进程总耗时包含 setup。
SQL 分块追加、position 索引和页面聚合上限避免每个 chunk 重写整条消息或一次读取整库。

作为环境对照，tmpfs 写入为 0.117 ms/op、分页 1.047 ms/op、流式 0.072 ms/chunk；这些数字不用于估计部署磁盘速度。
基准入口为 `conversation/benchmark_test.go`、`conversation/activity_test.go` 与 `handoff/budget_test.go`。

```sh
GOMAXPROCS=4 go test ./internal/conversation -run '^$' -bench '^BenchmarkActivityWriteTenThousand$' -benchtime=10000x -count=1 -benchmem
GOMAXPROCS=4 go test ./internal/conversation -run '^$' -bench '^BenchmarkHistoryPageTenThousand$' -benchtime=500x -count=1 -benchmem
GOMAXPROCS=4 go test ./internal/conversation -run '^$' -bench '^BenchmarkLongActivityStream$' -benchtime=256x -count=1 -benchmem
GOMAXPROCS=4 go test ./internal/handoff -run '^$' -bench '^BenchmarkSelectorTenThousand$' -benchtime=500x -count=1 -benchmem
```

运行磁盘基准时，把 `TMPDIR` 指向待测文件系统内的新建私有目录；测试自行清理。
峰值 RSS 可用外层进程等待单个基准子进程后读取 `resource.getrusage(resource.RUSAGE_CHILDREN).ru_maxrss`。

## 二进制与启动

使用同一份前端 `embed.zip`，以 Go 1.26.10、`CGO_ENABLED=0`、`-trimpath -buildvcs=false -ldflags='-s -w'` 构建。
对照为 `origin/main` 的 `f5b14acd` 与整个 handoff 栈；不是把全部差值归给 SQLite。
两份归档 SHA-256 均为 `6311a968f2946d6b3d591e4ce9b93a2cd6b925f87c6f42886848b84b4928497e`。

| 目标                  | 字节数     | 说明                         |
| --------------------- | ---------- | ---------------------------- |
| main，Linux amd64     | 50,643,106 | 同一前端归档                 |
| handoff，Linux amd64  | 55,566,498 | 增加 4,923,392 B，4.6953 MiB |
| handoff，Darwin arm64 | 54,535,458 | CGO=0 交叉构建通过           |

SQLite 初次引入层的独立历史对照增加 4,243,456 B，约 4.05 MiB；后续层还增加协议、CLI 和业务代码。
没有原生 macOS 运行测试；交叉构建和 Linux Go race 不证明 macOS runtime 行为。

启动从创建服务进程计到随机 loopback 端口的 `/api/health` 返回 200；每次使用新的 btrfs 状态目录，关闭后清理。
每组先预热 2 次，再测 8 次，未创建 agent 会话。可执行文件为暖缓存，未测 1,000 条旧记录迁移启动或冷盘启动。

| 配置              | 中位数    | 最小–最大        |
| ----------------- | --------- | ---------------- |
| main              | 10.620 ms | 10.260–11.611 ms |
| handoff，捕获开启 | 18.165 ms | 17.917–18.914 ms |
| handoff，捕获关闭 | 18.386 ms | 17.522–18.667 ms |

捕获关闭仍打开身份/投递保护数据库，不是缩短启动时间的配置。约 7.5 ms 的差异包含整个 handoff 栈，不能单独归因于 SQLite 驱动。

## 完整链路与边界

- 完整 inline、新 Conversation、A→B→A、native inject / 不支持回退、MCP 取回省略历史均有真实 stdio fake provider 集成测试。
- 历史 MCP 经过 SQLite → Unix HTTP API → CLI → provider；45 项分页无重复，中文/emoji 按 UTF-8 字节边界重建。
- UI 前后截图、浅深色、桌面/平板/手机和正常速度 GIF 见[核对记录](../user-stories/verification-log-2026-11-handoff.md)。
- 认证、Origin、PTY replay continuity、视图切换、归档恢复与 writer occupancy 沿用全仓测试；不连接开发者已有服务。
- Ubuntu CI 负责自动测试；没有原生 macOS、真实手机、硬件键盘/触屏或真实 provider 账号计费请求的运行结论。

全量门禁的实际完成结果在[交接核对记录](../user-stories/verification-log-2026-11-handoff.md)追加；本页的协议版本核实不等于真实模型请求验收。
