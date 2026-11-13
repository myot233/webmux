# signed-release-updates Specification

## Purpose

定义所有 WebMux 更新程序共用的规则：如何发现最新发布版本、如何以内置公钥验证发布签名与版本绑定、从哪些来源下载，以及哪些版本可以安装，使镜像只影响可用性而不能改变内容。

## Requirements

### Requirement: 仅发布构建参与更新

只有由发布流水线构建的二进制 SHALL 参与更新检查与自动更新。本地 `make build`、`make tray` 或 `go build` 的产物即使报告 `MAJOR.MINOR.PATCH` 版本号，也 SHALL 视为非发布构建：不发起检查，并在被要求检查时报告“此构建不是发布构建，不支持更新”。

#### Scenario: 本地构建

- **WHEN** 用户运行本地 `make build` 产出的 `bin/webmux update check`，该二进制报告 `webmux 0.1.0`
- **THEN** 命令以非零退出码结束并报告“此构建不是发布构建，不支持更新”，没有发起网络请求

#### Scenario: 发布构建

- **WHEN** 用户运行 Release 中的 `webmux-linux-amd64 update check`
- **THEN** 命令发起更新检查

### Requirement: 内置公钥验证

发布构建 SHALL 内置一组发布公钥，只接受由其中某个公钥签名的 `SHA256SUMS`。签名 SHALL 为 minisign 格式；验证 SHALL 覆盖文件内容与可信注释。验证失败、签名文件缺失或公钥 ID 不在内置集合中时，该来源 SHALL 视为失败，SHALL NOT 使用其中任何摘要。下载得到的 `SHA256SUMS` 不经签名验证 SHALL NOT 用作校验依据。

#### Scenario: 镜像篡改

- **WHEN** 某镜像返回被修改的二进制和与之匹配、但未经有效签名的 `SHA256SUMS`
- **THEN** 该来源被判定失败并记录“签名无效”，更新程序尝试下一个来源，不安装该二进制

#### Scenario: 未知公钥

- **WHEN** `SHA256SUMS.minisig` 由不在内置集合中的密钥签名
- **THEN** 该来源被判定失败并记录“未知签名密钥”

### Requirement: 签名绑定版本

签名的可信注释 SHALL 为 `webmux <版本>`，`<版本>` 为不带 `v` 的 `MAJOR.MINOR.PATCH`。更新程序 SHALL 以可信注释中的版本作为该清单的版本；可信注释格式不符时 SHALL 视为签名无效。随后下载的安装产物 SHALL 来自标签 `v<该版本>`，SHALL NOT 来自 `latest` 路径。

#### Scenario: 旧清单冒充新版本

- **WHEN** 来源在 `v0.5.0` 路径下返回 `v0.4.0` 的已签名 `SHA256SUMS`
- **THEN** 更新程序把该清单视为 0.4.0，不把它当作 0.5.0 使用

### Requirement: 最新版本发现

更新程序 SHALL 从 `releases/latest/download/` 路径读取 `SHA256SUMS` 与 `SHA256SUMS.minisig`，以验证通过的可信注释版本作为最新版本。

- 来源顺序 SHALL 为 GitHub，然后依次为内置镜像。
- GitHub 返回验证通过的清单时 SHALL 直接采用，SHALL NOT 再访问镜像。
- GitHub 失败时 SHALL 读取所有镜像，采用验证通过的清单中版本最高的一份（版本相同取靠前的镜像）；某个镜像失败或签名无效 SHALL NOT 使其他镜像验证通过的结果作废。
- 连接 10 秒无响应或 20 秒没有数据时 SHALL 改用下一个来源。
- 所有来源失败时 SHALL 报告每个来源的失败原因，且不改变任何已安装文件。

#### Scenario: GitHub 不可达

- **WHEN** GitHub 连接超时，只有第一个镜像返回有效签名的清单
- **THEN** 更新程序以该清单的版本作为最新版本，日志记录使用的来源

#### Scenario: 镜像缓存了旧清单

- **WHEN** GitHub 连接超时，第一个镜像返回有效签名的 0.5.0 清单，第二个镜像返回有效签名的 0.6.0 清单
- **THEN** 更新程序以 0.6.0 作为最新版本，日志记录第二个镜像为来源

#### Scenario: 全部失败

- **WHEN** 所有来源都超时或签名无效
- **THEN** 检查失败并列出每个来源的原因，已安装文件不变

### Requirement: 只升级不降级

更新程序 SHALL 只在最新版本按 `MAJOR.MINOR.PATCH` 比较严格高于当前版本时安装。最新版本等于或低于当前版本时 SHALL 报告“已是最新版本”，SHALL NOT 下载安装产物。

#### Scenario: 来源返回旧版本

- **WHEN** 当前版本为 0.5.0，镜像返回有效签名的 0.4.0 清单
- **THEN** 更新程序报告已是最新版本，不下载也不安装

### Requirement: 安装产物校验

安装产物 SHALL 在其 SHA-256 与已验证清单中同名一行一致后才可使用；不一致或清单中没有该文件时 SHALL 丢弃并尝试下一个来源。下载 SHALL 写入与目标同一文件系统的临时文件，校验通过后再交给安装步骤；失败时 SHALL 删除临时文件。

#### Scenario: 产物被截断

- **WHEN** 第一个来源返回的安装包被截断，摘要与清单不符
- **THEN** 更新程序删除该文件，改从下一个来源下载并校验

### Requirement: 检查更新命令

`webmux update check` SHALL 在任意安装方式下执行一次检查，输出当前版本、最新版本与是否有可用更新，SHALL NOT 下载安装产物或改写任何文件。

- `--json` 时 SHALL 只向标准输出写入一个 JSON 对象，包含 `v`、`current`、`latest`、`available` 与 `installMethod`（`deb`、`rpm` 或 `other`）。
- 检查成功时无论是否有更新，退出码 SHALL 为 0；检查失败时 SHALL 为非零。

#### Scenario: 手动安装检查

- **WHEN** 用户以 `install.sh` 安装 0.4.0，最新版本为 0.5.0，执行 `webmux update check`
- **THEN** 输出显示 0.4.0 与 0.5.0、有可用更新，并提示重新运行 `install.sh` 升级；`~/.local/bin/webmux` 不变

#### Scenario: JSON 输出

- **WHEN** deb 安装的 0.5.0 执行 `webmux update check --json` 且最新版本为 0.5.0
- **THEN** 标准输出为 `{"v":1,"current":"0.5.0","latest":"0.5.0","available":false,"installMethod":"deb"}`
