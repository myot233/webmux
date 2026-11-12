package codeview

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

func lines(text string) []string {
	if text == "" {
		return []string{}
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}
func document(root, p, source, oid string, data []byte) Document {
	d := Document{NodeID: nodeID(p), rawPath: p, Path: displayPath(p), Absolute: displayPath(filepath.Join(root, p)), Source: source, OID: oid, Version: digest(data), Kind: "text", Referenceable: utf8.ValidString(p) && utf8.ValidString(root)}
	switch {
	case len(data) > MaxBytes:
		d.Kind = "too-large"
		d.Reason = "超过 2 MiB"
	case strings.IndexByte(string(data), 0) >= 0:
		d.Kind = "binary"
		d.Reason = "二进制内容"
	case !utf8.Valid(data):
		d.Kind = "unsupported"
		d.Reason = "不支持的文本编码"
	default:
		d.Lines = len(lines(string(data)))
		if d.Lines > MaxLines {
			d.Kind = "too-large"
			d.Lines = 0
			d.Reason = "超过 20000 行"
		} else {
			d.Text = string(data)
			if extension(p) == ".svg" && svg(d.Text) {
				d.MediaType = svgMediaType
			}
		}
	}
	return d
}
func fileIdentity(info os.FileInfo) *identity {
	id := &identity{size: info.Size(), mtime: info.ModTime().UnixNano()}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		id.dev, id.ino = uint64(st.Dev), uint64(st.Ino)
	}
	return id
}
func (b *binding) read(p string) (Document, error) {
	f, err := b.open(p)
	if err != nil {
		return Document{}, err
	}
	// Read-only descriptor cleanup cannot affect the captured contents.
	defer func() { _ = f.Close() }()
	before, err := f.Stat()
	if err != nil {
		return Document{}, err
	}
	if !before.Mode().IsRegular() {
		return Document{}, problemf(ErrInvalid, "目标不是常规文件")
	}
	candidate := headerCandidate(p)
	limit := int64(MaxBytes + 1)
	if candidate {
		limit = headerBytes
	}
	data, err := io.ReadAll(io.LimitReader(f, limit))
	if err != nil {
		return Document{}, err
	}
	after, err := f.Stat()
	if err != nil {
		return Document{}, err
	}
	current, e := b.open(p)
	if e != nil {
		return Document{}, problemf(ErrConflict, "读取期间文件已替换")
	}
	defer func() { _ = current.Close() }()
	now, e := current.Stat()
	if e != nil || !os.SameFile(before, now) || before.Size() != after.Size() || before.ModTime() != after.ModTime() {
		return Document{}, problemf(ErrConflict, "读取期间文件发生变化，请刷新")
	}
	ident := fileIdentity(before)
	var d Document
	if candidate {
		d = headerDocument(b.Root, p, "worktree", "", before.Size(), data)
		d.Version = version([]any{ident.dev, ident.ino, ident.size, ident.mtime, digest(data)})
	} else {
		d = document(b.Root, p, "worktree", "", data)
		if before.Size() > MaxBytes {
			d.Version = version([]any{d.Version, before.Size(), before.ModTime()})
		}
		d.Size = before.Size()
		d.Downloadable = d.Size <= MaxDownloadBytes
	}
	d.identity = ident
	return d, nil
}
func (s *Service) put(v *snapshot) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, old := range s.snapshots {
		if now.Sub(old.created) > Lifetime {
			s.bytes -= old.size
			delete(s.snapshots, id)
		}
	}
	for s.bytes+v.size > CacheBytes {
		var oldest string
		for id, old := range s.snapshots {
			if oldest == "" || old.timeUsed.Before(s.snapshots[oldest].timeUsed) {
				oldest = id
			}
		}
		if oldest == "" {
			break
		}
		s.bytes -= s.snapshots[oldest].size
		delete(s.snapshots, oldest)
	}
	id := random()
	v.created = now
	v.timeUsed = now
	s.snapshots[id] = v
	s.bytes += v.size
	return id
}
func (s *Service) get(id, sid string) (*snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.snapshots[sid]
	if v == nil || time.Since(v.created) > Lifetime {
		return nil, problemf(ErrGone, "内容快照已过期，请刷新")
	}
	if v.session != id {
		return nil, problemf(ErrInvalid, "快照不属于此会话")
	}
	v.timeUsed = time.Now()
	return v, nil
}
func (s *Service) File(ctx context.Context, id, cid, nid string) (File, error) {
	return s.FileSource(ctx, id, cid, nid, "worktree")
}
func (s *Service) FileSource(ctx context.Context, id, cid, nid, source string) (File, error) {
	return withRoot(ctx, s, id, cid, func(b *binding) (File, error) {
		p, e := nodePath(nid)
		if e != nil {
			return File{}, e
		}
		d, e := b.sourceFile(ctx, p, source)
		if e != nil {
			return File{}, e
		}
		f := File{Document: d, Checked: time.Now().UTC()}
		f.Snapshot = s.put(&snapshot{session: id, context: cid, file: &f, size: documentSize(d) + 512})
		return f, nil
	})
}
func (s *Service) SnapshotFile(ctx context.Context, id, sid, side string) (File, error) {
	return withBinding(ctx, s, id, s.snapshotRoot(id, sid), func(*binding) (File, error) {
		v, e := s.get(id, sid)
		if e != nil {
			return File{}, e
		}
		if v.file != nil {
			return *v.file, nil
		}
		d := v.diff.New
		if side == "old" {
			d = v.diff.Old
		} else if side != "new" {
			return File{}, problemf(ErrInvalid, "请选择 old/new 来源")
		}
		return File{Snapshot: sid, Document: d, Checked: v.diff.Checked}, nil
	})
}
func documentSize(d Document) int {
	return len(d.Text) + len(d.Path) + len(d.Absolute) + len(d.NodeID) + len(d.OID) + len(d.Reason) + 256
}

func version(v any) string { b, _ := json.Marshal(v); return digest(b) }
