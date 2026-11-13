# session-artifacts Specification

## Purpose

TBD - created by archiving change add-session-artifacts-folder. Update Purpose after archive.

## Requirements

### Requirement: 交互物临时文件夹的位置与权限

系统 SHALL 为每个会话提供一个交互物临时文件夹，用于用户与会话内程序（CLI agent）交换文件。

- 实例根目录 SHALL 位于规范化（解析符号链接）后的系统临时目录下，名称为 `webmux-artifacts-<uid>-<实例摘要>`，其中实例摘要由规范化后的 `state_dir` 派生，使共用同一临时目录的不同 WebMux 实例互不影响。
- 会话文件夹 SHALL 为 `<实例根目录>/<会话ID>`，上传子目录为 `<会话文件夹>/uploads`。
- 实例根目录与会话文件夹 SHALL 以 0700 权限创建。使用前系统 SHALL 用不跟随符号链接的检查确认它们是目录、属于服务进程有效用户且不允许其他用户访问；任一条件不满足时 SHALL 拒绝使用并报告文件夹不可用，SHALL NOT 修正或删除不属于自己的条目。
- 文件夹 SHALL 按需创建：会话创建时 SHALL NOT 自动创建。

#### Scenario: 两个实例共用临时目录

- **WHEN** 两个使用不同 `state_dir` 的 WebMux 实例在同一系统临时目录下各自为会话创建临时文件夹
- **THEN** 两者位于不同的实例根目录，一方的启动清扫不会删除另一方的文件

#### Scenario: 预置的符号链接

- **WHEN** 其他用户预先在系统临时目录下创建了同名符号链接或不属于服务用户的目录
- **THEN** 确保接口返回文件夹不可用，系统不跟随该链接写入，也不删除该条目

### Requirement: 确保临时文件夹接口

服务端 SHALL 提供已认证的 `POST /api/sessions/{id}/artifacts`，继承 `/api/*` 现有的认证、Host 与 Origin 校验。

- 会话存在（包括已退出但尚未删除）时 SHALL 幂等地创建文件夹并返回 `200` 与 `{path}`，`path` 为会话文件夹的绝对规范路径。
- 会话不存在时 SHALL 返回 404；文件夹不可用（权限、属主或文件系统错误）时 SHALL 返回 503 并给出不含路径以外敏感信息的原因。
- 该接口 SHALL NOT 列出或返回文件夹内容。

#### Scenario: 首次确保

- **WHEN** 客户端对一个运行中的会话首次调用确保接口
- **THEN** 服务端创建 0700 的会话文件夹并返回其绝对路径；再次调用返回相同路径且不改动已有内容

#### Scenario: 已删除的会话

- **WHEN** 客户端对已删除的会话调用确保接口
- **THEN** 服务端返回 404，不创建任何目录

### Requirement: 临时文件夹的生命周期与清扫

- 普通会话被删除（`DELETE /api/sessions/{id}?remove=1` 或其他使会话离开会话列表的删除路径）后，系统 SHALL 删除该会话文件夹及其全部内容。
- 混合会话离开运行中列表但仍保留可恢复记录时，系统 SHALL 保留其文件夹；`DELETE /api/hybrid-sessions/{id}` 成功删除记录后 SHALL 删除该文件夹。恢复后的混合会话 SHALL 沿用原文件夹。
- 会话仅关闭或进程退出而未被删除时，文件夹 SHALL 保留。
- 服务启动时，在开始接受请求之前，系统 SHALL 清扫本实例根目录：删除名称不对应任何可恢复混合会话记录的会话文件夹。混合会话记录不可读取（例如记录文件损坏）时 SHALL 跳过整次清扫并记录警告，SHALL NOT 删除任何文件夹。
- 删除 SHALL 只作用于本实例根目录内的直接子目录，SHALL NOT 跟随符号链接。删除失败 SHALL 记录不含文件内容的警告，SHALL NOT 影响会话删除本身的结果。

#### Scenario: 删除普通会话

- **WHEN** 用户删除一个已退出的普通会话，其临时文件夹中有 agent 生成的报告和一个上传文件
- **THEN** 会话从列表消失后，其会话文件夹被删除

#### Scenario: 混合会话关闭后恢复

- **WHEN** 混合会话被归档并出现在所属工作树的“已归档”区，随后服务重启，用户再恢复该会话
- **THEN** 关闭与重启都不删除其临时文件夹，恢复后代码栏的临时文件夹仍显示原有文件

#### Scenario: 重启清扫残留

- **WHEN** 服务异常退出后重启，实例根目录中残留若干普通会话的文件夹
- **THEN** 启动时这些文件夹被删除，可恢复混合会话的文件夹保留

#### Scenario: 混合记录损坏时不清扫

- **WHEN** 服务启动时 `hybrid-sessions.json` 无法解析
- **THEN** 实例根目录中的所有文件夹保持原样，日志记录跳过清扫

### Requirement: 上传文件保存到临时文件夹

终端与聊天的文件上传（`POST /api/sessions/{id}/upload`）SHALL 把文件保存到该会话临时文件夹的 `uploads/` 子目录（按需创建），返回的 `path` 为该文件的绝对路径。

- 现有的文件名清理、100 MiB 上限、5 分钟期限、退出会话拒绝上传、失败或取消时删除未完成文件的行为 SHALL 保持不变。
- 失败或取消 SHALL 只删除本次文件，SHALL NOT 删除 `uploads/` 中的其他文件。
- 文件 SHALL 以 0600 权限独占创建，SHALL NOT 覆盖已有文件。
- 同名文件已存在时，系统 SHALL 在扩展名前追加 ` (2)`、` (3)` 等最小可用序号；无扩展名时追加在末尾。
- 系统 SHALL NOT 再创建 `webmux-upload-*` 目录。

#### Scenario: 同名上传

- **WHEN** 用户先后在同一会话上传两个名为 `shot.png` 的文件
- **THEN** 两个文件分别保存为 `uploads/shot.png` 与 `uploads/shot (2).png`，各自插入对应的绝对路径

#### Scenario: 取消上传

- **WHEN** `uploads/` 中已有 `a.txt`，用户上传 `b.bin` 途中取消
- **THEN** 未完成的 `b.bin` 被删除，`a.txt` 保留

### Requirement: 聊天附件只接受本会话上传

混合会话发送带附件的消息时，服务端 SHALL 只接受位于该会话临时文件夹 `uploads/` 中、直接子项、非符号链接的普通文件；路径中途经过的目录 SHALL NOT 为符号链接。其他路径（包括其他会话的上传文件、临时文件夹中 agent 生成的文件）SHALL 以无效附件拒绝。附件数量、大小与类型规则保持不变。

#### Scenario: 引用其他会话的上传文件

- **WHEN** 客户端在会话 A 的消息中附上会话 B 的上传文件路径
- **THEN** 服务端以无效附件拒绝该消息，不读取该文件

### Requirement: 代码栏的临时文件夹根

代码栏导航顶部 SHALL 提供根切换，选项为“工作区”与“临时文件夹”，默认“工作区”，现有行为保持不变。

- 切换到“临时文件夹”时，客户端 SHALL 先调用确保接口，再以 `GET /api/sessions/{id}/code/context?root=artifacts` 获取上下文；`root` 缺省或为 `workspace` 时 SHALL 返回现有工作区上下文，其他取值 SHALL 返回 400。
- 临时文件夹上下文的 `rootKind` SHALL 为 `artifacts`，`browseAvailable` 为真，`diffAvailable` 与 `ignoreAvailable` 为假。
- 后续目录、搜索、文件、定位、快照与原始字节请求 SHALL 由 `contextId` 区分所属根，并沿用现有的只读、不跟随符号链接、按会话校验与大小上限规则。
- 对该上下文请求修改列表或 Diff SHALL 被拒绝。
- 处于临时文件夹根时，导航 SHALL 隐藏“修改”页签，根路径行 SHALL 显示文件夹绝对路径；文件树、路径搜索、图片/Markdown/HTML/表格/JSON/PDF 预览与下载 SHALL 与工作区一致。
- 两个根打开的文件标签 SHALL 可以并存，标签 SHALL 标明来自“临时文件夹”。外部变化检测（约每 5 秒）SHALL 同样适用于临时文件夹根。
- 确保接口失败时，导航 SHALL 显示原因并提供重试，SHALL NOT 回退显示工作区内容冒充临时文件夹。

#### Scenario: 查看 agent 生成的报告

- **WHEN** agent 把 `report.md` 写入会话临时文件夹，用户在代码栏切换到“临时文件夹”
- **THEN** 文件树列出 `report.md`，打开后可切换 Markdown 预览，导航中没有“修改”页签

#### Scenario: 打开后新增文件

- **WHEN** 用户正在浏览临时文件夹根，agent 随后写入新文件 `chart.png`
- **THEN** 导航提示目录有更新，用户刷新后看到并可预览 `chart.png`

#### Scenario: 非法根参数

- **WHEN** 客户端请求 `code/context?root=home`
- **THEN** 服务端返回 400

### Requirement: 临时文件夹的 shell 访问

会话临时文件夹 SHALL 可以从 shell 获取：`webmux dir SESSION` 调用确保接口并输出路径（见 `shell-session-control`）。
`{{webmux}}` 简述 SHALL 包含本会话临时文件夹的路径，以及交付物保存规则（见 `agent-shell-guide`）。

- shell 工具手册以 llms.txt 形式经 HTTP 提供（见 `agent-shell-guide`），SHALL NOT 写入实例临时根或任何会话的临时文件夹。

#### Scenario: agent 获取交付位置

- **WHEN** 会话内的 agent 执行 `webmux dir self`
- **THEN** 输出与代码栏“临时文件夹”根相同的绝对路径，该目录已存在

#### Scenario: 临时根中没有手册

- **WHEN** 用户在会话中使用 `{{webmux}}` 后，查看代码栏的“临时文件夹”根和实例临时根
- **THEN** 只有会话自己的文件，没有手册文件
