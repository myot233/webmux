package codeview

import (
	"context"
	"fmt"
	"github.com/rainchan/webmux/internal/workspace"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func (b *binding) listing(ctx context.Context, p string, include bool) (Listing, error) {
	r := Listing{Children: []Node{}, Complete: true, Checked: time.Now().UTC()}
	f, e := b.open(p)
	if e != nil {
		return r, e
	}
	defer f.Close()
	entries, e := f.ReadDir(10001)
	if e != nil && e != io.EOF {
		return r, e
	}
	if len(entries) > 10000 {
		entries = entries[:10000]
		r.Limited = true
		r.Complete = false
	}
	ignored := map[string]bool{}
	statuses := map[string]string{}
	if b.Ignore {
		var paths strings.Builder
		for _, entry := range entries {
			paths.WriteString(filepath.ToSlash(filepath.Join(p, entry.Name())))
			paths.WriteByte(0)
		}
		if paths.Len() > 0 {
			e := workspace.ReadGitRecordsInput(ctx, b.Root, strings.NewReader(paths.String()), func(path string) error {
				ignored[path] = true
				return nil
			}, "check-ignore", "-z", "--stdin")
			if e != nil {
				return r, e
			}
		}
		wanted := map[string]bool{}
		for _, entry := range entries {
			wanted[filepath.ToSlash(filepath.Join(p, entry.Name()))] = true
		}
		skipSource := false
		e := workspace.ReadGitRecords(ctx, b.Root, func(record string) error {
			if skipSource {
				skipSource = false
				return nil
			}
			if len(record) < 4 {
				return problem(500, "无效 Git 状态输出")
			}
			path := strings.TrimSuffix(record[3:], "/")
			if wanted[path] {
				statuses[path] = record[:2]
			}
			skipSource = strings.ContainsAny(record[:2], "RC")
			return nil
		}, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--", pOrDot(p))
		if e != nil {
			return r, e
		}
		if skipSource {
			return r, problem(500, "Git 状态输出不完整")
		}
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return r, ctx.Err()
		}
		name := entry.Name()
		if strings.EqualFold(name, ".git") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(p, name))
		full := filepath.Join(b.Root, rel)
		internal := false
		for _, g := range b.gitDirs {
			if full == g || strings.HasPrefix(full, g+"/") {
				internal = true
			}
		}
		if internal {
			continue
		}
		ig := ignored[rel] || ignored[rel+"/"]
		if ig && !include {
			continue
		}
		kind := "file"
		if entry.IsDir() {
			kind = "directory"
			fd, e := unix.Openat(int(f.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if e != nil {
				return r, e
			}
			var st unix.Stat_t
			if unix.Fstatat(fd, ".git", &st, unix.AT_SYMLINK_NOFOLLOW) == nil {
				kind = "boundary"
			} else if unix.Fstatat(fd, "HEAD", &st, unix.AT_SYMLINK_NOFOLLOW) == nil && unix.Fstatat(fd, "objects", &st, unix.AT_SYMLINK_NOFOLLOW) == nil {
				kind = "boundary"
			}
			unix.Close(fd)
		} else if entry.Type()&os.ModeSymlink != 0 {
			kind = "symlink"
		} else if !entry.Type().IsRegular() {
			kind = "special"
		}
		n := Node{ID: nodeID(rel), Path: displayPath(rel), Name: displayPath(name), Kind: kind, Ignored: ig, Status: statuses[rel], Referenceable: utf8.ValidString(rel)}
		if kind == "symlink" {
			data := make([]byte, 4096)
			count, e := unix.Readlinkat(int(f.Fd()), name, data)
			if e == nil {
				n.Link = string(data[:count])
			}
		}
		r.Children = append(r.Children, n)
	}
	sort.Slice(r.Children, func(i, j int) bool {
		a, b := r.Children[i], r.Children[j]
		if (a.Kind == "directory") != (b.Kind == "directory") {
			return a.Kind == "directory"
		}
		return a.Name < b.Name
	})
	r.Version = version(r.Children)
	return r, nil
}
func pOrDot(p string) string {
	if p == "" {
		return "."
	}
	return p
}
func (s *Service) Tree(ctx context.Context, id, cid, nid string, include bool, cursor string) (Listing, error) {
	release, e := s.acquire(ctx)
	if e != nil {
		return Listing{}, e
	}
	defer release()
	b, e := s.root(ctx, id, cid)
	if e != nil {
		return Listing{}, e
	}
	p, e := nodePath(nid)
	if e != nil {
		return Listing{}, e
	}
	r, e := b.listing(ctx, p, include)
	if e != nil {
		return r, e
	}
	start := 0
	if cursor != "" {
		v, n, ok := strings.Cut(cursor, ":")
		start, e = strconv.Atoi(n)
		if !ok || e != nil || start < 0 {
			return r, problem(400, "无效目录游标")
		}
		if v != r.Version {
			return r, problem(409, "目录已改变，请重新读取")
		}
	}
	if start > len(r.Children) {
		return r, problem(400, "无效目录页")
	}
	end := min(start+200, len(r.Children))
	if end < len(r.Children) {
		r.Next = fmt.Sprintf("%s:%d", r.Version, end)
	}
	r.Children = r.Children[start:end]
	return r, nil
}
func (s *Service) Search(ctx context.Context, id, cid, q string, include bool) (Listing, error) {
	release, e := s.acquire(ctx)
	if e != nil {
		return Listing{}, e
	}
	defer release()
	b, e := s.root(ctx, id, cid)
	if e != nil {
		return Listing{}, e
	}
	r := Listing{Children: []Node{}, Complete: true, Checked: time.Now().UTC()}
	queue := []string{""}
	visited := 0
	for len(queue) > 0 {
		if e := ctx.Err(); e != nil {
			return r, e
		}
		p := queue[0]
		queue = queue[1:]
		page, e := b.listing(ctx, p, include)
		if e != nil {
			return r, e
		}
		if page.Limited {
			r.Limited = true
			r.Complete = false
		}
		for _, n := range page.Children {
			visited++
			if visited >= 100000 || len(r.Children) >= 200 {
				r.Limited = true
				r.Complete = false
				queue = nil
				break
			}
			if n.Kind == "directory" {
				queue = append(queue, n.Path)
			} else if strings.Contains(strings.ToLower(n.Path), strings.ToLower(q)) {
				r.Children = append(r.Children, n)
			}
		}
	}
	r.Version = version(r.Children)
	return r, nil
}
