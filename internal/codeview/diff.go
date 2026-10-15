package codeview

import (
	"context"
	"strconv"
	"strings"
	"time"
)

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
	var c *Change
	before, e := b.scanChanges(ctx, func(entry Change) error {
		if entry.ID == changeID {
			copy := entry
			c = &copy
		}
		return nil
	})
	if e != nil {
		return Diff{}, e
	}
	if c == nil {
		return Diff{}, problem(409, "修改来源已改变，请刷新列表")
	}
	old, next, e := b.capture(ctx, *c)
	if e != nil {
		return Diff{}, e
	}
	after, e := b.scanChanges(ctx, nil)
	if e != nil {
		return Diff{}, e
	}
	if after != before {
		return Diff{}, problem(409, "读取期间 Git 来源已改变")
	}
	r := Diff{Old: old, New: next, Kind: "text", Checked: time.Now().UTC(), Change: *c}
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
	}
	v := &snapshot{session: id, context: cid, diff: &r, size: documentSize(old) + documentSize(next)}
	r.Snapshot = s.put(v)
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
