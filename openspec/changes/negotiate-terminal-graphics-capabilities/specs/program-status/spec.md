## MODIFIED Requirements

### Requirement: 服务端应答协议探测

会话在实时读取到探测序列 `ESC ] 7501 ; ? (BEL | ESC \)` 时，系统 SHALL 向该会话的 PTY 写入且只写入一次应答：

```
ESC ] 7501 ; ? : states=idle,working,done,blocked,error : kinds=permission,question,auth ESC \
```

应答 SHALL 满足：

- **无需客户端**：没有任何客户端附着时也会写入，不等待浏览器或原生 attach。
- **顺序**：应答 SHALL 先于该探测被发布给订阅者之后进入 PTY 的任何客户端输入，包括对紧随其后的 DA1 的会话级应答；也 SHALL NOT 与另一份输入交错写入。
- **不当作用户输入**：应答 SHALL NOT 取得控制权、改变尺寸、清除关注或计为用户输入。
- **只响应实时输出**：只有实时 PTY 读取会触发应答；浏览器或原生 attach 回放历史、重新同步 SHALL NOT 触发应答。
- **有界、不阻塞**：待写应答 SHALL 有固定上限，超出上限的探测 SHALL 被丢弃而不重试。应答 SHALL NOT 阻塞 PTY 读取，也 SHALL NOT 在持有会话状态锁时等待 PTY 可写。
- **取消**：会话退出 SHALL 取消尚未写入的应答。
- **不改输出**：探测序列本身 SHALL 照常写入回放并发送给订阅者，不被修改或删除。

#### Scenario: 无浏览器时启动 Claude Code

- **WHEN** 通过启动配置创建 Claude Code 会话，没有任何客户端附着，Claude Code 启动时输出 `ESC ] 7501 ; ? ESC \` 与 `ESC [ c`
- **THEN** PTY 输入收到一次 7501 探测应答和一次由会话写入的 DA1 应答，且 7501 应答在 DA1 之前，Claude Code 能据此判定支持 7501

#### Scenario: 应答先于浏览器的 DA1

- **WHEN** 浏览器控制端已附着，程序在同一次写入中输出 7501 探测和 DA1
- **THEN** PTY 输入中 7501 应答完整出现在会话级 DA1 应答之前，浏览器不对该 DA1 产生第二份应答

#### Scenario: 回放不重答

- **WHEN** 回放缓冲区中含有程序启动时的 7501 探测，用户打开或重新打开该会话页
- **THEN** PTY 不再收到新的 7501 应答

#### Scenario: 应答不清除关注

- **WHEN** 会话已有 `question` 关注，随后程序发出 7501 探测并得到服务端应答
- **THEN** 关注仍为 `question`，控制端不变
