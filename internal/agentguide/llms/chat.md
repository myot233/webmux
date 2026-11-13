# 聊天视图

带聊天视图的会话可以在终端 TUI 与聊天视图（ACP）之间移动同一个 agent 会话。输入、取回回复、中断、切换视图、列出、恢复与删除记录对所有会话通用，
见 [会话](/llms/sessions.md)；创建见 [启动会话](/llms/launch.md)。`webmux chat` 只包含聊天视图专有的操作，选项与示例用 `webmux chat <子命令> --help` 查看。

## 约定

- 会话没有聊天视图时以 1 结束；需要聊天视图而会话在终端视图时以 1 结束，并提示先 `webmux view` 切到聊天。

## 操作

- 查看：`webmux chat status` 输出一次快照（当前视图、回合、队列、权限模式、待回答请求及其选项、agent 设置）；`webmux chat watch` 持续输出消息、工具调用、请求与回合边界，需要外层 `timeout` 才会退出。
- 事后读取：会话已经跑完时，用 `webmux chat log SESSION` 一次性读取并退出，默认只输出最后一条消息（通常是 agent 的最后回复）；`--last N`（1 到 1000）取最近 N 条用户与 agent 消息，`--tools` 加上工具调用（每个一行：标题与最终状态，不重复进度帧），`--json` 输出一个 ChatFrame。它只读：不发送、不等待回合，与只返回自己那一轮回复的 `send --reply` 不同；服务端只保留有限历史，回合还在进行时最后一条可能不完整。
- 回答：先用 `webmux chat status` 查询真实 pending request，核对请求内容和用户授权范围。授权用当前请求真实 optionId，提问按其问题回答；没有 pending request 时不回答，不因失败就假定需要授权。`webmux chat answer` 只处理该当前请求，不扩大为永久 auto。
- 设置：`webmux chat set` 修改 agent 的设置项（模型、模式等），取值来自 `chat status`；`webmux chat permission` 选择授权请求是等待回答（ask）还是由服务端自动批准（auto）。
- 队列：回合进行中发送的提示进入服务端队列，所有设备看到同一队列；`webmux chat queue` 查看、编辑、删除、立即发送或恢复暂停的队列。

## 切换视图

`webmux view` 切换并等待完成。切到聊天时终端客户端（`attach`、浏览器终端页）被分离；切回终端时聊天视图关闭，排队的提示作为草稿还给发起的设备。
终端 TUI 忙时需要选择：不强制则取消切换并以 1 结束。服务端找不到 agent 会话时会列出候选，按候选重试。
