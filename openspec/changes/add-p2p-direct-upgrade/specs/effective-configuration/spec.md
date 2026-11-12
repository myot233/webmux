# effective-configuration Specification Delta

## ADDED Requirements

### Requirement: P2P configuration provenance and store health

配置诊断视图（`configured` 与 `effective`）SHALL 包含 `[p2p]` 字段（`p2p.enabled`、`p2p.port_mapping`、`p2p.stun_servers`）及其真实来源（`default`、`toml` 或 `web`），并将 `p2p-settings` 存储纳入 `stores` 健康检查列表。

当 `p2p-settings.json` 损坏导致安全关闭时，诊断视图 SHALL 反映降级后的关闭状态与存储错误原因，且不得在只读检查中创建或修改文件。

#### Scenario: P2P override from web settings

- **WHEN** 用户在网页设置中保存了 P2P 配置并查询有效配置
- **THEN** `p2p.enabled`、`p2p.port_mapping` 与 `p2p.stun_servers` 的来源均为 `web`，`p2p-settings` 存储状态为 `available`

#### Scenario: Damaged P2P settings store degrades safely

- **WHEN** `p2p-settings.json` 内容损坏，用户执行只读配置检查
- **THEN** `p2p.enabled` 显示为安全关闭后的 `false`，`p2p-settings` 存储标记损坏状态，磁盘文件保持原样不被改写
