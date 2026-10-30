## ADDED Requirements

### Requirement: 格式预览入口

完整文本文件的扩展名（大小写不敏感）为 `.html`、`.htm`、`.csv`、`.tsv` 或 `.json` 时，代码工作区 SHALL 在文档栏提供“预览”入口并可“返回源码”，窄屏时入口位于“代码菜单”。这些文件默认显示源码；切换文件、打开 Diff 或刷新为新内容后 SHALL 回到源码，预览状态 SHALL NOT 写入设置。超过文本阅读上限、编码不支持或为二进制的同名文件 SHALL NOT 提供预览。Diff 视图 SHALL NOT 提供这些格式的预览。

#### Scenario: 切换预览

- **WHEN** 用户打开 `report.html`、`data.csv` 与 `result.json`，分别点击“预览”
- **THEN** 依次显示静态网页、表格与可折叠树，点击“返回源码”回到源码视图

#### Scenario: 扩展名不符不提供入口

- **WHEN** 用户打开 `notes.txt` 或超过 2 MiB 的 `big.json`
- **THEN** 文档栏不显示“预览”入口

### Requirement: HTML 静态预览

HTML 预览 SHALL 先在不执行脚本的解析文档中移除 `<script>`、`<base>`、`<link>`、`<template>`、`<meta http-equiv="refresh">`、`<object>`、`<embed>`、`<iframe>` 与 SVG 动画元素，移除所有事件处理属性、`<a>`/`<area>` 的 `href`、表单的 `action`、全部 `srcset`，以及经 URL 解析后协议不是 `data:` 的 `src`、`poster`、`background`、`href` 属性。

处理后的文档 SHALL 以 `srcdoc` 放入 `sandbox` 属性为空的框架，并在文档头部注入 CSP `default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:`。

预览 SHALL 保留内联样式与 `data:` 图片，SHALL NOT 执行脚本、提交表单、发起网络请求或导航框架与应用页面。预览上方 SHALL 显示“静态预览，脚本与外部资源已禁用”。框架 SHALL 无法访问应用的存储、Cookie 或认证令牌。

#### Scenario: 静态报告正常显示

- **WHEN** `report.html` 使用 `<style>` 与内联样式绘制表格，并含 `data:` 图标
- **THEN** 预览按样式显示表格与图标

#### Scenario: 脚本、外部资源与导航被阻止

- **WHEN** HTML 含 `<script>`、`<img src="https://example.com/x.png">`、`<link rel="stylesheet" href="a.css">`、`<meta http-equiv="refresh" content="0;url=https://example.com">`、`<a href="https://example.com">` 与 `<form action="/api/sessions">`
- **THEN** 脚本不执行，不发起任何外部或同源请求，框架不跳转，链接文字不可导航，表单无法提交

### Requirement: CSV 与 TSV 表格预览

表格预览 SHALL 对 `.csv` 使用逗号、对 `.tsv` 使用制表符分隔，按 RFC 4180 处理双引号字段、转义引号与字段内换行，首行作为表头，列数不齐的行以空单元格补齐。预览 SHALL 最多渲染前 2000 条数据行，超出时提示“仅显示前 2000 行，共 N 行”。表头 SHALL 在纵向滚动时保持可见，宽表 SHALL 可横向滚动。引号未闭合等无法解析的内容 SHALL 显示错误位置并提供“返回源码”，SHALL NOT 显示错误的表格。单元格内容 SHALL 作为文本显示。

#### Scenario: 引号与多行字段

- **WHEN** CSV 中有字段 `"a,b"`、`"他说""好"""` 与跨两行的引号字段
- **THEN** 表格分别显示 `a,b`、`他说"好"` 与含换行的单元格，列不错位

#### Scenario: 超长表格

- **WHEN** CSV 有 5000 条数据行
- **THEN** 表格显示前 2000 条并提示“仅显示前 2000 行，共 5000 行”

#### Scenario: 解析失败

- **WHEN** CSV 第 12 行存在未闭合的引号
- **THEN** 预览显示“第 12 行引号未闭合”并提供返回源码，不显示表格

### Requirement: JSON 树预览

JSON 预览 SHALL 把合法 JSON 显示为可折叠树，默认展开前两层；对象与数组 SHALL 显示成员数，超过 200 个成员的集合 SHALL 分批展开。数字 SHALL 按源码原文显示，SHALL NOT 因转换为浮点数丢失精度。字符串 SHALL 作为文本显示。非法 JSON SHALL 显示出错的行与列并提供返回源码。

#### Scenario: 大整数保持原样

- **WHEN** JSON 为 `{"id": 12345678901234567890, "ratio": 1.10}`
- **THEN** 树中显示 `12345678901234567890` 与 `1.10`

#### Scenario: 大数组分批

- **WHEN** 顶层数组含 1000 个对象，用户展开该数组
- **THEN** 先显示前 200 项与“再显示 200 项”，点击后继续追加

#### Scenario: 非法 JSON

- **WHEN** 文件第 3 行缺少逗号
- **THEN** 预览显示“第 3 行第 N 列：JSON 语法错误”，并提供返回源码

### Requirement: PDF 识别

系统 SHALL 仅在扩展名为 `.pdf`（大小写不敏感）且文件前 1024 字节内出现 `%PDF-` 时，将常规文件识别为文档种类 `pdf`，媒体类型为 `application/pdf`。大小超过 20 MiB 时 SHALL 标为超过预览上限并保留媒体类型与下载能力。空文件、签名缺失的 `.pdf` 文件 SHALL 显示为二进制内容并说明原因。其他扩展名的 PDF 内容 SHALL 仍显示为二进制内容。PDF 的原始字节响应 SHALL 使用 `application/pdf` 并带 CSP `sandbox`。

#### Scenario: 有效 PDF

- **WHEN** 用户打开 3 MiB 的 `design.pdf`
- **THEN** 文档种类为 `pdf`，媒体类型为 `application/pdf`，代码面板直接显示 PDF 预览

#### Scenario: 无效或改名文件

- **WHEN** 用户打开空的 `a.pdf`、内容为文本的 `b.pdf` 和扩展名为 `.bin` 的有效 PDF
- **THEN** `a.pdf` 提示“空文件”，`b.pdf` 提示“PDF 格式无法识别”，`.bin` 文件显示为二进制内容

#### Scenario: 超过预览上限

- **WHEN** 用户打开 40 MiB 的 `scan.pdf`
- **THEN** 面板显示“超过 20 MiB 预览上限”、大小与下载入口，不加载 PDF 渲染器

### Requirement: PDF 预览

PDF 预览 SHALL 在首次需要时加载随应用打包的渲染器与 worker，SHALL NOT 访问公共 CDN；PDF 字节 SHALL 通过认证请求从快照原始字节端点读取。

预览 SHALL 默认按面板宽度适应显示所有页面，并提供页码与跳页、适应宽度、放大、缩小与下载；触摸设备 SHALL 支持双指缩放，窄屏工具栏保持单行。页面 SHALL 只在接近可视区时渲染，并释放远离可视区页面的画布。

文本 SHALL 可选择与复制；文档内链接 SHALL 跳转到目标页，HTTP(S) 与 `mailto:` 链接 SHALL 在新标签页打开并带 `noopener noreferrer`，其他链接 SHALL 无效。预览 SHALL NOT 执行 PDF 内脚本、显示可交互表单或加载外部资源。中文、日文与韩文字体 SHALL 能正确显示。

受密码保护、损坏、渲染失败或浏览器不支持渲染器的 PDF SHALL 显示原因并保留下载入口。切换文件或关闭标签页时 SHALL 取消尚未完成的读取与渲染并释放资源。

#### Scenario: 阅读多页文档

- **WHEN** 用户在手机上打开 30 页的中文 PDF，滚动到第 20 页并在页码框输入 5
- **THEN** 中文正常显示，跳到第 5 页；只有可视区附近的页面保有画布

#### Scenario: 链接与脚本

- **WHEN** PDF 含目录内链、指向 `https://example.com` 的链接与文档打开时执行的脚本
- **THEN** 内链跳到对应页，外链在新标签页打开，脚本不执行

#### Scenario: 受密码保护或损坏

- **WHEN** 用户打开加密的 `secret.pdf` 与截断的 `broken.pdf`
- **THEN** 分别显示“PDF 受密码保护，请下载后查看”与“PDF 无法解析”，下载入口仍可用

#### Scenario: 渲染器按需加载

- **WHEN** 用户打开应用但从未打开 PDF
- **THEN** 浏览器不请求 PDF 渲染器、worker、CMap 或字体资源
