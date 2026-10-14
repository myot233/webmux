package codeview

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (b *binding) changes(ctx context.Context) (Changes, error) {
	r := Changes{Entries: []Change{}, Checked: time.Now().UTC()}
	if !b.Diff {
		return r, problem(409, "此目录的 Git 差异不可用")
	}
	for _, group := range []string{"unstaged", "staged"} {
		args := []string{"diff", "--raw", "-z", "--no-abbrev", "--no-ext-diff", "--no-textconv", "--find-renames"}
		if group == "staged" {
			args = append(args, "--cached")
		}
		args = append(args, "--")
		out, e := git(ctx, b.Root, args...)
		if e != nil {
			return r, e
		}
		records := strings.Split(string(out), "\x00")
		for i := 0; i < len(records) && records[i] != ""; {
			fields := strings.Fields(records[i])
			if len(fields) != 5 || !strings.HasPrefix(fields[0], ":") || i+1 >= len(records) {
				return r, problem(500, "无效 Git 修改输出")
			}
			c := Change{Group: group, OldPath: records[i+1], NewPath: records[i+1], OldOID: fields[2], NewOID: fields[3], OldMode: strings.TrimPrefix(fields[0], ":"), NewMode: fields[1], Status: fields[4]}
			i += 2
			if strings.HasPrefix(c.Status, "R") || strings.HasPrefix(c.Status, "C") {
				if i >= len(records) {
					return r, problem(500, "无效重命名输出")
				}
				c.NewPath = records[i]
				i++
			}
			c.NodeID = nodeID(c.NewPath)
			c.ID = version(c)
			r.Entries = append(r.Entries, c)
		}
	}
	out, e := git(ctx, b.Root, "ls-files", "--others", "--exclude-standard", "-z")
	if e != nil {
		return r, e
	}
	for _, p := range strings.Split(string(out), "\x00") {
		if p == "" {
			continue
		}
		c := Change{Group: "untracked", OldPath: p, NewPath: p, Status: "A", OldMode: "000000", NewMode: "100644"}
		c.NodeID = nodeID(c.NewPath)
		c.ID = version(c)
		r.Entries = append(r.Entries, c)
	}
	if len(r.Entries) > 1000 {
		return r, problem(413, "修改超过 1000 项，请收窄仓库")
	}
	r.Version = version(r.Entries)
	return r, nil
}
func (s *Service) Changes(ctx context.Context, id, cid string) (Changes, error) {
	b, e := s.root(ctx, id, cid)
	if e != nil {
		return Changes{}, e
	}
	release, e := s.acquire(ctx)
	if e != nil {
		return Changes{}, e
	}
	defer release()
	return b.changes(ctx)
}
func (b *binding) blob(ctx context.Context, p, source, oid, mode string) (Document, error) {
	if mode == "000000" || strings.Trim(oid, "0") == "" {
		return document(b.Root, p, "empty", "", nil), nil
	}
	if mode != "100644" && mode != "100755" {
		d := document(b.Root, p, source, oid, nil)
		d.Kind = "unsupported"
		d.Reason = "链接、子模块或特殊模式仅显示元信息"
		return d, nil
	}
	sizeBytes, e := git(ctx, b.Root, "cat-file", "-s", oid)
	if e != nil {
		return Document{}, e
	}
	size, e := strconv.ParseInt(strings.TrimSpace(string(sizeBytes)), 10, 64)
	if e != nil {
		return Document{}, problem(500, "无效对象大小")
	}
	if size > MaxBytes {
		d := document(b.Root, p, source, oid, nil)
		d.Kind = "too-large"
		d.Reason = "超过 2 MiB"
		d.Version = oid
		return d, nil
	}
	data, e := git(ctx, b.Root, "cat-file", "blob", oid)
	if e != nil {
		return Document{}, e
	}
	return document(b.Root, p, source, oid, data), nil
}
func (b *binding) capture(ctx context.Context, c Change) (Document, Document, error) {
	if _, e := nodePath(nodeID(c.OldPath)); e != nil {
		return Document{}, Document{}, e
	}
	if _, e := nodePath(nodeID(c.NewPath)); e != nil {
		return Document{}, Document{}, e
	}
	oldSource := "index"
	if c.Group == "staged" {
		oldSource = "head"
	}
	old, e := b.blob(ctx, c.OldPath, oldSource, c.OldOID, c.OldMode)
	if e != nil {
		return old, Document{}, e
	}
	var next Document
	if c.Group == "staged" {
		next, e = b.blob(ctx, c.NewPath, "index", c.NewOID, c.NewMode)
	} else if c.NewMode == "000000" {
		next = document(b.Root, c.NewPath, "empty", "", nil)
	} else if c.NewMode != "100644" && c.NewMode != "100755" {
		next = document(b.Root, c.NewPath, "worktree", "", nil)
		next.Kind = "unsupported"
		next.Reason = "子模块、链接或特殊模式仅显示元信息"
	} else {
		next, e = b.read(c.NewPath)
	}
	return old, next, e
}
func parseDiff(patch string) ([]Line, error) {
	out := []Line{}
	old, newLine := 0, 0
	inHunk := false
	for _, raw := range strings.Split(patch, "\n") {
		if strings.HasPrefix(raw, "@@ ") {
			var oc, nc int
			n, _ := fmt.Sscanf(raw, "@@ -%d,%d +%d,%d @@", &old, &oc, &newLine, &nc)
			if n != 4 {
				fields := strings.Fields(raw)
				if len(fields) < 3 {
					return nil, problem(500, "无效 diff hunk")
				}
				old, _ = strconv.Atoi(strings.Split(strings.TrimPrefix(fields[1], "-"), ",")[0])
				newLine, _ = strconv.Atoi(strings.Split(strings.TrimPrefix(fields[2], "+"), ",")[0])
			}
			inHunk = true
			continue
		}
		if !inHunk || raw == "" {
			continue
		}
		l := Line{Text: strings.TrimSuffix(raw[1:], "\r")}
		switch raw[0] {
		case ' ':
			l.Kind = "context"
			l.Old = old
			l.New = newLine
			old++
			newLine++
		case '-':
			l.Kind = "delete"
			l.Old = old
			old++
		case '+':
			l.Kind = "add"
			l.New = newLine
			newLine++
		case '\\':
			l.Kind = "marker"
			l.Text = raw
		default:
			return nil, problem(500, "无效 diff 行")
		}
		out = append(out, l)
	}
	return out, nil
}
func blocks(all []Line) []Block {
	r := []Block{}
	add := func(start, end int, kind string) {
		b := Block{Kind: kind, Start: start, End: end, Count: end - start, Lines: []Line{}}
		if kind == "gap" {
			b.ID = fmt.Sprintf("%d:%d", start, end)
		} else {
			b.Lines = all[start:end]
		}
		r = append(r, b)
	}
	for i := 0; i < len(all); {
		if all[i].Kind != "context" {
			j := i + 1
			for j < len(all) && all[j].Kind != "context" {
				j++
			}
			add(i, j, "lines")
			i = j
			continue
		}
		j := i
		for j < len(all) && all[j].Kind == "context" {
			j++
		}
		start, end := i+3, j-3
		if i == 0 {
			start = i
		}
		if j == len(all) {
			end = j
		}
		if end > start {
			if start > i {
				add(i, start, "lines")
			}
			add(start, end, "gap")
			if end < j {
				add(end, j, "lines")
			}
		} else {
			add(i, j, "lines")
		}
		i = j
	}
	return r
}
func (s *Service) Diff(ctx context.Context, id, cid, changeID string) (Diff, error) {
	b, e := s.root(ctx, id, cid)
	if e != nil {
		return Diff{}, e
	}
	release, e := s.acquire(ctx)
	if e != nil {
		return Diff{}, e
	}
	defer release()
	changes, e := b.changes(ctx)
	if e != nil {
		return Diff{}, e
	}
	var c *Change
	for _, entry := range changes.Entries {
		if entry.ID == changeID {
			copy := entry
			c = &copy
			break
		}
	}
	if c == nil {
		return Diff{}, problem(409, "修改来源已改变，请刷新列表")
	}
	old, next, e := b.capture(ctx, *c)
	if e != nil {
		return Diff{}, e
	}
	after, e := b.changes(ctx)
	if e != nil {
		return Diff{}, e
	}
	if after.Version != changes.Version {
		return Diff{}, problem(409, "读取期间 Git 来源已改变")
	}
	r := Diff{Old: old, New: next, Blocks: []Block{}, Kind: "text", Checked: time.Now().UTC(), Change: *c}
	all := []Line{}
	if strings.HasPrefix(c.Status, "U") {
		r.Kind = "metadata"
		r.Reason = "冲突文件仅显示元信息"
	} else if old.Kind != "text" || next.Kind != "text" {
		r.Kind = "metadata"
		r.Reason = "非文本或受限内容不能显示行差异"
	} else {
		attrs, e := git(ctx, b.Root, "check-attr", "-z", "filter", "working-tree-encoding", "--", c.NewPath)
		if e != nil {
			return r, e
		}
		a := strings.Split(string(attrs), "\x00")
		for i := 0; i+2 < len(a); i += 3 {
			if a[i+2] != "unspecified" && a[i+2] != "unset" {
				r.Kind = "metadata"
				r.Reason = "需要内容转换，不能保证原始行映射"
			}
		}
		if r.Kind == "text" {
			dir, e := os.MkdirTemp("", "webmux-code-diff-")
			if e != nil {
				return r, e
			}
			defer os.RemoveAll(dir)
			a, z := filepath.Join(dir, "old"), filepath.Join(dir, "new")
			if e = os.WriteFile(a, []byte(old.Text), 0600); e != nil {
				return r, e
			}
			if e = os.WriteFile(z, []byte(next.Text), 0600); e != nil {
				return r, e
			}
			patch, e := git(ctx, dir, "diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-color", "--ignore-cr-at-eol", "--unified=20000", "--", a, z)
			if e != nil {
				if Status(e) == 413 {
					r.Kind = "metadata"
					r.Reason = "差异输出超过 2 MiB"
				} else {
					return r, e
				}
			} else {
				all, e = parseDiff(string(patch))
				if e != nil {
					return r, e
				}
				if len(all) == 0 {
					for i, text := range lines(old.Text) {
						all = append(all, Line{Kind: "context", Old: i + 1, New: i + 1, Text: strings.TrimSuffix(text, "\r")})
					}
				}
				r.Blocks = blocks(all)
			}
		}
	}
	size := documentSize(old) + documentSize(next) + len(all)*64 + len(r.Blocks)*128
	for _, l := range all {
		size += len(l.Text)
	}
	v := &snapshot{session: id, context: cid, diff: &r, all: all, size: size}
	r.Snapshot = s.put(v)
	return r, nil
}
func (s *Service) Expand(ctx context.Context, id, sid, gap, direction string, count int) ([]Block, error) {
	if _, e := s.bind(ctx, id); e != nil {
		return nil, e
	}
	v, e := s.get(id, sid)
	if e != nil {
		return nil, e
	}
	if v.diff == nil {
		return nil, problem(400, "不是 diff 快照")
	}
	if gap == "all" && direction == "all" {
		return []Block{{Kind: "lines", Start: 0, End: len(v.all), Count: len(v.all), Lines: v.all}}, nil
	}
	var start, end int
	if _, e := fmt.Sscanf(gap, "%d:%d", &start, &end); e != nil || start < 0 || end > len(v.all) || start >= end {
		return nil, problem(400, "无效 gap")
	}
	for _, l := range v.all[start:end] {
		if l.Kind != "context" {
			return nil, problem(400, "gap 不是连续上下文")
		}
	}
	count = min(max(count, 1), 30)
	a, z := start, end
	switch direction {
	case "up":
		a = max(start, end-count)
	case "down":
		z = min(end, start+count)
	case "all":
	default:
		return nil, problem(400, "无效展开方向")
	}
	r := []Block{}
	if a > start {
		r = append(r, Block{ID: fmt.Sprintf("%d:%d", start, a), Kind: "gap", Start: start, End: a, Count: a - start, Lines: []Line{}})
	}
	r = append(r, Block{Kind: "lines", Start: a, End: z, Count: z - a, Lines: v.all[a:z]})
	if z < end {
		r = append(r, Block{ID: fmt.Sprintf("%d:%d", z, end), Kind: "gap", Start: z, End: end, Count: end - z, Lines: []Line{}})
	}
	return r, nil
}

// sourceFile resolves only the current worktree, stage-zero index or HEAD.
// Arbitrary revisions are deliberately not accepted from clients.
func (b *binding) sourceFile(ctx context.Context, p, source string) (Document, error) {
	if source == "worktree" || source == "" {
		return b.read(p)
	}
	if source != "index" && source != "head" {
		return Document{}, problem(400, "无效文件来源")
	}
	if !b.Diff {
		return Document{}, problem(409, "Git 来源不可用")
	}
	if _, e := nodePath(nodeID(p)); e != nil {
		return Document{}, e
	}
	resolve := func() (string, string, error) {
		args := []string{"ls-files", "--stage", "-z", "--", p}
		if source == "head" {
			args = []string{"ls-tree", "-z", "HEAD", "--", p}
		}
		raw, e := git(ctx, b.Root, args...)
		if e != nil {
			return "", "", e
		}
		for _, record := range strings.Split(string(raw), "\x00") {
			header, path, ok := strings.Cut(record, "\t")
			if !ok || path != p {
				continue
			}
			fields := strings.Fields(header)
			if len(fields) != 3 {
				return "", "", problem(500, "无效来源记录")
			}
			if source == "index" {
				if fields[2] != "0" {
					return "", "", problem(409, "索引处于冲突状态")
				}
				return fields[0], fields[1], nil
			}
			return fields[0], fields[2], nil
		}
		return "", "", problem(404, "此来源中没有该文件")
	}
	mode, oid, e := resolve()
	if e != nil {
		return Document{}, e
	}
	d, e := b.blob(ctx, p, source, oid, mode)
	if e != nil {
		return d, e
	}
	nextMode, nextOID, e := resolve()
	if e != nil || nextMode != mode || nextOID != oid {
		return d, problem(409, "读取期间来源已改变")
	}
	return d, nil
}
