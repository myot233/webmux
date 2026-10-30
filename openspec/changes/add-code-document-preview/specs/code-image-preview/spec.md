## MODIFIED Requirements

### Requirement: 按扩展名与文件签名识别图片

系统 SHALL 按扩展名与文件签名识别图片，光栅图片的媒体类型 SHALL 取自签名而非扩展名。

- 光栅图片：仅在扩展名属于 `.png`、`.jpg`、`.jpeg`、`.gif`、`.webp`、`.avif`、`.bmp`、`.ico`（大小写不敏感）且文件头签名匹配任一受支持光栅格式时识别。
- SVG 图片：扩展名为 `.svg`、内容为合法 UTF-8 且首个 XML 元素为 `svg` 的文件 SHALL 识别为 SVG 图片。
- 其他文件 SHALL 沿用既有文本、二进制、PDF、编码与上限分类，不因内容碰巧像图片而改判。
- 图片扩展名但签名不匹配、为空或头部损坏的文件 SHALL 给出可区分的原因，不显示为图片。
- 文件树和标签 SHALL 仅凭扩展名为可能的图片显示图片图标。

#### Scenario: 常见格式打开即识别

- **WHEN** 根内存在有效的 PNG、JPEG、GIF、WebP、AVIF、BMP、ICO 和 SVG 文件，用户逐个打开
- **THEN** 每个文档都标识为图片并给出对应媒体类型和字节大小，不再显示“二进制内容”或“不支持的文本编码”

#### Scenario: 非图片二进制与改名文件

- **WHEN** 用户打开 ZIP、PDF，或扩展名为 `.bin` 的有效 PNG
- **THEN** 三者均不被识别为图片：ZIP 与 `.bin` 文件显示为二进制内容，PDF 按 `code-document-preview` 识别为 PDF 文档

#### Scenario: 扩展名与内容不一致

- **WHEN** `.png` 文件实际包含 JPEG 签名，另一个 `.png` 为空文件，第三个 `.png` 头部为随机字节
- **THEN** 第一个按 JPEG 图片预览；空文件提示“空文件”；随机字节提示图片格式无法识别，均不伪造图片

#### Scenario: 不是 SVG 的 .svg 文件

- **WHEN** `.svg` 文件不是合法 UTF-8，或首个元素不是 `svg`
- **THEN** 该文件按既有文本或二进制规则展示，不提供图片预览
