// Package agentguide is the WebMux shell guide for scripts and CLI agents,
// served llms.txt-style: an index at /llms.txt, one Markdown page per topic
// at /llms/<topic>.md and everything at /llms-full.txt. It also writes the
// short brief that {{webmux}} expands to. The hand-written topics are
// embedded; the command directory is rendered from the CLI's command table
// by the caller (this shared package cannot import the CLI) and handed to
// New. Nothing is written to disk.
package agentguide

import (
	"embed"
	"fmt"
	"strings"
	"sync"
)

//go:embed llms/*.md
var pages embed.FS

// Topic is one page of the guide.
type Topic struct {
	Name  string
	Title string
	// Summary follows the link in the index.
	Summary string
	// Optional topics are listed under "## Optional".
	Optional bool
	// Section groups the index.
	Section string
}

// CommandsTopic is the topic whose page is the command directory.
const CommandsTopic = "commands"

// Topics lists every page in index order; /llms-full.txt joins them in
// this order.
var Topics = []Topic{
	{Name: "connect", Title: "连接与身份", Section: "基础", Summary: "找到 webmux 可执行文件与套接字，会话中的变量，会话内被禁止的操作与排错。"},
	{Name: "sessions", Title: "会话", Section: "基础", Summary: "会话状态与视图，列出（含已归档）与元数据，输入与取回回复，中断与切换视图，等待、输出与画面，提醒，结束、恢复与删除记录。"},
	{Name: "launch", Title: "启动会话", Section: "基础", Summary: "用 run 在目录、已有或新建 worktree 中启动会话，选择终端或聊天视图并下达首条消息，跟随或等待。"},
	{Name: "chat", Title: "聊天视图", Section: "基础", Summary: "聊天视图专有的操作：回答授权与提问、agent 设置与权限模式、队列、状态与观察。"},
	{Name: "workspace", Title: "仓库与工作区", Section: "基础", Summary: "worktree 的创建、删除与主页显示，仓库登记与初始化指令，启动配置的增删改，端口转发。"},
	{Name: "artifacts", Title: "临时文件夹", Section: "基础", Summary: "交付文件放在哪里、上传文件在哪里，以及怎样告诉用户。"},
	{Name: CommandsTopic, Title: "命令目录", Section: "参考", Summary: "每个命令一行的用法与摘要；选项、输出与示例用 `webmux <命令> --help` 查看。"},
	{Name: "exit-codes", Title: "退出码", Section: "参考", Summary: "退出码与输出约定。"},
	{Name: "rules", Title: "操作守则", Section: "参考", Summary: "操作其他会话、Token 与外部内容的规则。"},
	{Name: "recipes", Title: "常用流程", Optional: true, Summary: "分派任务给 agent、继续对话、跑测试取退出码、后台任务、提醒用户、了解当前环境、交付文件、查看帮助的完整例子。"},
}

// Instance is what the local socket adds to the guide and the brief. The
// TCP listener serves the guide without its facts.
type Instance struct {
	// Guide is the guide both listeners serve; only the local socket adds
	// the instance facts to it.
	Guide   Guide
	Version string
	// LocalURL is the token-free local origin, such as http://127.0.0.1:7681.
	LocalURL string
	// Bin is the server's executable (WEBMUX_BIN); empty when unknown.
	Bin string

	mu     sync.Mutex
	socket string
}

// SetSocket records the local API socket once it listens.
func (i *Instance) SetSocket(path string) {
	i.mu.Lock()
	i.socket = path
	i.mu.Unlock()
}

// Socket is the local API socket, or "" while there is none.
func (i *Instance) Socket() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.socket
}

// GuideURL is the address of the index on the local listener.
func (i *Instance) GuideURL() string {
	return strings.TrimSuffix(i.LocalURL, "/") + "/llms.txt"
}

// Guide is the shell guide: the embedded topics plus the command
// directory. The zero Guide has an empty directory.
type Guide struct {
	directory string
}

// New returns the guide whose commands topic is directory, the Markdown
// command directory rendered from the command table.
func New(directory string) Guide {
	return Guide{directory: directory}
}

// Page returns the Markdown of topic name.
func (g Guide) Page(name string) (string, bool) {
	for _, t := range Topics {
		if t.Name != name {
			continue
		}
		if name == CommandsTopic {
			return g.directory, true
		}
		data, err := pages.ReadFile("llms/" + name + ".md")
		if err != nil {
			return "", false
		}
		core := name
		switch name {
		case "sessions":
			core = "completion"
		case "chat":
			core = "requests"
		}
		rule := coreRules(core)
		if rule == "" {
			return string(data), true
		}
		var text strings.Builder
		capacity := len(data) + len(rule) + len("\n## 共用操作语义\n\n\n")
		if name == "sessions" {
			capacity += len(operationRules) + 2
		}
		text.Grow(capacity)
		text.Write(data)
		text.WriteString("\n## 共用操作语义\n\n")
		text.WriteString(rule)
		text.WriteByte('\n')
		if name == "sessions" {
			text.WriteByte('\n')
			text.WriteString(operationRules)
			text.WriteByte('\n')
		}
		return text.String(), true
	}
	return "", false
}

const summary = "WebMux 是单用户的 Web 终端：会话是服务端持有的 PTY 与进程，浏览器断开不会结束会话。" +
	"`webmux` 命令经本机套接字控制它，不需要 Token；命令以服务用户的身份运行，不是沙箱。"

const prefix = "下文的 `webmux` 指 WebMux 可执行文件：优先使用 `\"$WEBMUX_BIN\"`，" +
	"其次是 `{{webmux}}` 简述或本实例信息中的绝对路径，最后才是 `PATH` 中的 `webmux`。" +
	"路径只定位文件，不保证版本一致；比较客户端 version 与服务 status。环境变量丢失时加 `--socket <套接字>`，并用会话 ID 代替 `self`。详见 [连接与身份](/llms/connect.md)。"

const when = "需要查看或控制 WebMux 会话、启动命令或 agent、等待会话、驱动聊天视图、准备 worktree、转发端口、提醒用户、" +
	"或把文件交给用户时阅读本手册。先读本索引，再只读需要的主题；任务与 WebMux 无关时不必读取。" +
	"单个命令的选项、输出与示例不在手册中：运行 `webmux <命令> --help` 或 `webmux help --json`。"

// Index renders /llms.txt. A nil instance leaves out everything specific
// to the running server, for the unauthenticated TCP listener. session is
// the session whose process asked, or "" when the caller is in none.
func (g Guide) Index(instance *Instance, session string) string {
	var b strings.Builder
	b.WriteString("# WebMux\n\n> " + summary + "\n\n")
	if instance != nil {
		b.WriteString("本实例：\n\n")
		fmt.Fprintf(&b, "- 版本：%s\n", instance.Version)
		if instance.Bin != "" {
			fmt.Fprintf(&b, "- 可执行文件：`%s`\n", instance.Bin)
		}
		if socket := instance.Socket(); socket != "" {
			fmt.Fprintf(&b, "- 套接字：`%s`\n", socket)
		}
		if session != "" {
			fmt.Fprintf(&b, "- 本会话 ID：`%s`（环境变量 WEBMUX_SESSION_ID 丢失时用它代替 `self`）\n", session)
		}
		b.WriteString("\n")
	}
	b.WriteString(prefix + "\n\n" + when + "\n")
	b.WriteString("\n" + connectionRules + "\n\n" + safetyRules + "\n")
	var section string
	for _, t := range Topics {
		heading := t.Section
		if t.Optional {
			heading = "Optional"
		}
		if heading != section {
			section = heading
			b.WriteString("\n## " + heading + "\n\n")
		}
		fmt.Fprintf(&b, "- [%s](/llms/%s.md): %s\n", t.Title, t.Name, t.Summary)
	}
	return b.String()
}

// Full renders /llms-full.txt: the index, then every topic in order.
func (g Guide) Full(instance *Instance, session string) string {
	var b strings.Builder
	b.WriteString(g.Index(instance, session))
	for _, t := range Topics {
		if page, ok := g.Page(t.Name); ok {
			b.WriteString("\n---\n\n" + page)
		}
	}
	return b.String()
}

// BriefInput is the session-specific part of the {{webmux}} brief.
type BriefInput struct {
	SessionID string
	// Dir is the session's artifacts folder.
	Dir string
}

// Brief is the text {{webmux}} expands to: who the session is, where the
// executable, socket and guide are, and the one artifacts rule every task
// needs. Paths are inserted verbatim, without shell quoting.
func (i *Instance) Brief(in BriefInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "本会话运行在 WebMux 中（会话 ID：%s）。\n", in.SessionID)
	bin, socket := i.Bin, i.Socket()
	if bin != "" {
		fmt.Fprintf(&b, "- WebMux 命令：%s", bin)
		if socket != "" {
			fmt.Fprintf(&b, "（套接字：%s）", socket)
		}
		b.WriteString("\n  `webmux` 不在 PATH 中时用这个绝对路径；环境变量 WEBMUX_* 丢失时加 `--socket <套接字>`，并用上面的会话 ID 代替 self。\n")
	}
	reader := "webmux guide"
	if bin != "" {
		reader = bin + " guide"
	}
	fmt.Fprintf(&b, "- WebMux shell 工具手册（llms.txt）：%s\n", i.GuideURL())
	fmt.Fprintf(&b, "  也可以运行 `%s`", reader)
	if socket != "" {
		fmt.Fprintf(&b, "（等价于 `curl -s --unix-socket %s http://webmux/llms.txt`）", socket)
	}
	b.WriteString("，不需要 Token。需要查看或控制会话、启动命令或 agent、了解当前仓库与分支、给会话打标签或交付文件时，先读索引再按需读主题；任务与此无关时不必读取。\n")
	fmt.Fprintf(&b, "- 本会话的临时文件夹：%s\n", in.Dir)
	b.WriteString("  交付给我的截图、报告、导出数据等文件保存到这里而不是项目仓库，规则见 /llms/artifacts.md。")
	b.WriteString("\n" + artifactRules + "\n" + safetyRules + "\n")
	return b.String()
}
