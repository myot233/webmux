# 服务端偏好协议

`GET /api/settings` 返回 `{revision, updatedAt, values}`。`values` 始终包含 theme、preview、codeReader、quickCommands、homeWorktrees、sessionPins、forceSoftKeys、ignoreHighPorts 八个键。

## 值与默认

- theme：light / dark / null；null 跟随系统。
- preview：`{lines: 1..50, skip: 0..50}` 或 null（10/1）。
- codeReader：`{lineNumbers: "show"|"hide", diffOverflow: "scroll"|"wrap"}` 或 null（show/scroll）。
- quickCommands：最多 50 个 `{id, name, command, enter}`，唯一 ID；null 显示系统预设（“恢复会话” `/resume`、“新会话” `/new` 与“WebMux 工具” `{{webmux}}`；旧版本保存的“临时文件夹”原样保留），空数组表示明确清空。
  - 保存列表时每条 `name` 必填、去首尾空白后唯一、不含 `{`、`}` 与换行、不为 `user`、`dir` 或 `webmux`（初始化指令的内置变量），初始化指令用 `{{name}}` 引用它。
  - 校验只在 PATCH `quickCommands` 时进行；此前保存的未命名、重名、叫 `user`、`dir` 或 `webmux`、或含花括号的指令在读取时由服务端按正文命名，下一次保存列表时写回；正文里旧写法的 `{dir}` 同样在读取时改为 `{{dir}}`。
  - `command` 中的 `{{dir}}` 在运行时替换为会话临时文件夹的路径，`{{webmux}}` 替换为会话的 WebMux 工具说明（`POST /api/sessions/{id}/agent-guide`）。
- homeWorktrees：仓库 ID 到工作树 ID 数组的映射；最多 200 个仓库、每仓库 1000 个、合计 2000 个，数组内 ID 不重复；null 使用默认主页选择。
- sessionPins：按展示顺序排列的会话 ID，最多 50 个且不重复。null 或空数组表示无置顶。同步时清理已退出或删除的会话，并保留尚可恢复的中断混合会话。清理有变化时提交一个设置修订并广播。
- forceSoftKeys：布尔或 null（false）。
- ignoreHighPorts：布尔或 null（true，忽略 30000 及以上端口的转发提示与自动检测列表）。

## 原子更新

`PATCH /api/settings` 请求为 `{baseRevision, changes}`。changes 只包含本次修改的已知键；null 恢复默认，不提供的键保持不变。请求上限 2 MiB，拒绝重复键、未知字段、大小写变体、无效类型和多个 JSON 值。

修订号一致时持久提交后返回完整新快照；revision 增一，updatedAt 为 UTC 时间。冲突 409，非法请求 400，认证 401，Origin 403，存储不可用 503。错误不回显设置值。客户端重读后按字段/ID 操作意图最多重试一次，不上传整个旧快照；网络失败不会后台重放。

## 实时事件

`/ws/settings` 使用现有 WebSocket token 子协议与 Origin 校验。首帧为 `{type: "settings", settings: <完整快照>}`；随后为 `{type: "settings.update", revision, updatedAt, changes}`。

快照捕获和订阅注册与提交共用同一服务锁，确保无遗漏。每连接只有一个网络写者；队列有界，慢客户端断开后重连取新快照。客户端遇到修订号缺口重新读取；旧事件不回退快照。

## 持久化与设备边界

`settings.json` 与 `settings.json.lock` 通过统一 statefile handle 管理。缺失时 revision 0、updatedAt 空、全部值 null；读取不会创建数据文件。损坏原文件保留，禁用设置但不影响终端。目录同步不确定时冻结写入，不前进修订号或广播成功。

会话置顶在首次连接时从旧浏览器导入。其他旧浏览器键不迁移。

旧 `settings.json` 中遗留的历史键（`fontSize`、`chatIdleSleepMinutes`、`artifactPrompt`）读取时忽略，下次写入时移除。`webmux.settingsCache` 只缓存最后确认的快照；不能用作离线写入队列。Token、Web Push 身份、草稿、最近目录和临时命令不属于此协议。
