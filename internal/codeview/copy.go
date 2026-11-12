package codeview

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// Copy copies (ModeCopy) or moves (ModeMove) nodes nids into directory
// target of the same context. Every node keeps its name, or gets the
// smallest free " (n)" suffix; nothing is replaced. A copy of a directory
// recreates its files, directories and symlinks, skipping .git entries,
// nested repositories and special files, within MaxDownloadBytes and
// MaxTransferEntries in total. A move is a rename, so it fails on another
// filesystem; moving a node into its own directory leaves it in place.
func (s *Service) Copy(ctx context.Context, id, cid string, nids []string, target, mode string) (Mutation, error) {
	return withRoot(ctx, s, id, cid, func(b *binding) (Mutation, error) {
		if err := b.requireWrite(); err != nil {
			return Mutation{}, err
		}
		if mode != ModeCopy && mode != ModeMove {
			return Mutation{}, problemf(ErrInvalid, "请选择 copy/move")
		}
		if len(nids) == 0 || len(nids) > MaxCopyNodes {
			return Mutation{}, problemf(ErrInvalid, "一次可粘贴 1–%d 项", MaxCopyNodes)
		}
		dst, err := nodePath(target)
		if err != nil {
			return Mutation{}, err
		}
		paths := make([]string, len(nids))
		for i, nid := range nids {
			if paths[i], err = nodePath(nid); err != nil {
				return Mutation{}, err
			}
			if paths[i] == "" {
				return Mutation{}, problemf(ErrForbidden, "根目录不可复制或移动")
			}
			if dst == paths[i] || strings.HasPrefix(dst, paths[i]+"/") {
				return Mutation{}, problemf(ErrInvalid, "不能粘贴到自身或其子目录")
			}
		}
		dir, err := b.dir(dst)
		if err != nil {
			return Mutation{}, err
		}
		defer func() { _ = dir.Close() }()
		if mode == ModeCopy {
			if err := b.copyBudget(ctx, paths); err != nil {
				return Mutation{}, err
			}
		}
		op := map[string]string{ModeCopy: "复制", ModeMove: "移动"}[mode]
		var out Mutation
		budget := int64(MaxDownloadBytes)
		for _, p := range paths {
			e, err := b.entry(p)
			if err != nil {
				return out, err
			}
			if err := writableKind(e.kind); err != nil {
				_ = e.parent.Close()
				return out, err
			}
			var placed string
			switch {
			case mode == ModeMove && parentOf(p) == dst:
				placed = e.name
			case mode == ModeMove:
				placed, err = moveAt(int(e.parent.Fd()), e.name, int(dir.Fd()), e.name, e.kind == "directory")
			default:
				placed, err = copyAt(ctx, e.parent, e.name, e.kind, dir, &budget)
			}
			_ = e.parent.Close()
			if err != nil {
				return out, fsError(op, err)
			}
			out.Nodes = append(out.Nodes, b.node(join(dst, placed), e.kind))
		}
		return out, nil
	})
}

// copyBudget refuses copies above the byte or entry bound before any write.
func (b *binding) copyBudget(ctx context.Context, paths []string) error {
	var total Usage
	for _, p := range paths {
		u, err := b.usage(ctx, p, MaxTransferEntries+1)
		if err != nil {
			return err
		}
		total.Entries += u.Entries
		total.Bytes += u.Bytes
		if total.Entries > MaxTransferEntries {
			return problemf(ErrTooLarge, "超过 %d 个条目的复制上限", MaxTransferEntries)
		}
		if total.Bytes > MaxDownloadBytes {
			return problemf(ErrTooLarge, "超过 100 MiB 复制上限")
		}
	}
	return nil
}

// copyAt copies src/name (a file or directory) into dst under a free name.
// A failed copy removes what it created.
func copyAt(ctx context.Context, src *os.File, name, kind string, dst *os.File, budget *int64) (string, error) {
	if kind == "file" {
		var out *os.File
		placed, err := reserve(name, func(c string) error {
			fd, err := unix.Openat(int(dst.Fd()), c, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
			if err == nil {
				out = os.NewFile(uintptr(fd), c)
			}
			return err
		})
		if err != nil {
			return "", err
		}
		if err := copyFile(ctx, src, name, out, budget); err != nil {
			removeAt(int(dst.Fd()), placed, false)
			return "", err
		}
		return placed, nil
	}
	placed, err := reserve(name, func(c string) error { return unix.Mkdirat(int(dst.Fd()), c, 0o700) })
	if err != nil {
		return "", err
	}
	if err := copyDir(ctx, src, name, dst, placed, budget); err != nil {
		if out, oe := openDirAt(dst, placed, placed); oe == nil {
			_ = removeContents(context.WithoutCancel(ctx), out)
			_ = out.Close()
		}
		removeAt(int(dst.Fd()), placed, true)
		return "", err
	}
	return placed, nil
}

// copyFile copies the regular file src/name into out, closes out and gives
// it the source's permission bits.
func copyFile(ctx context.Context, src *os.File, name string, out *os.File, budget *int64) error {
	defer func() { _ = out.Close() }()
	fd, err := unix.Openat(int(src.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	in := os.NewFile(uintptr(fd), name)
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return problemf(ErrConflict, "复制期间文件已替换")
	}
	n, err := io.Copy(out, contextReader{ctx, io.LimitReader(in, *budget+1)})
	if err != nil {
		return err
	}
	if *budget -= n; *budget < 0 {
		return problemf(ErrTooLarge, "超过 100 MiB 复制上限")
	}
	if err := out.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	return out.Close()
}

// copyDir fills the new directory dst/placed with the contents of src/name.
func copyDir(ctx context.Context, src *os.File, name string, dst *os.File, placed string, budget *int64) error {
	from, err := openDirAt(src, name, name)
	if err != nil {
		return err
	}
	defer func() { _ = from.Close() }()
	info, err := from.Stat()
	if err != nil {
		return err
	}
	to, err := openDirAt(dst, placed, placed)
	if err != nil {
		return err
	}
	defer func() { _ = to.Close() }()
	entries := 0
	for {
		list, err := from.ReadDir(256)
		for _, de := range list {
			if e := ctx.Err(); e != nil {
				return e
			}
			child := de.Name()
			if strings.EqualFold(child, ".git") {
				continue
			}
			kind, _, se := statAt(int(from.Fd()), child)
			if se != nil {
				return se
			}
			if entries++; entries > MaxTransferEntries {
				return problemf(ErrTooLarge, "超过 %d 个条目的复制上限", MaxTransferEntries)
			}
			switch kind {
			case "file":
				out, oe := unix.Openat(int(to.Fd()), child, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
				if oe != nil {
					return oe
				}
				if ce := copyFile(ctx, from, child, os.NewFile(uintptr(out), child), budget); ce != nil {
					return ce
				}
			case "directory":
				if me := unix.Mkdirat(int(to.Fd()), child, 0o700); me != nil {
					return me
				}
				if ce := copyDir(ctx, from, child, to, child, budget); ce != nil {
					return ce
				}
			case "symlink":
				if le := copyLink(int(from.Fd()), child, int(to.Fd())); le != nil {
					return le
				}
			}
		}
		if errors.Is(err, io.EOF) || (err == nil && len(list) == 0) {
			break
		}
		if err != nil {
			return err
		}
	}
	return to.Chmod(info.Mode().Perm())
}

// copyLink recreates a symlink with the same target; the target is never read.
func copyLink(from int, name string, to int) error {
	buf := make([]byte, unix.PathMax)
	n, err := unix.Readlinkat(from, name, buf)
	if err != nil {
		return err
	}
	if n >= len(buf) {
		return problemf(ErrInvalid, "链接目标过长")
	}
	return unix.Symlinkat(string(buf[:n]), to, name)
}

// walkEntry is one entry below a walked directory. Kind is "git" for any
// case of a .git entry, which is never classified or opened.
type walkEntry struct {
	parent *os.File
	rel    string
	name   string
	kind   string
	stat   unix.Stat_t
}

// walk visits every entry below directory dir (root-relative path rel) depth
// first, parents before children. A directory visitor may return errSkip.
func walk(ctx context.Context, dir *os.File, rel string, visit func(walkEntry) error) error {
	for {
		list, err := dir.ReadDir(256)
		for _, de := range list {
			if e := ctx.Err(); e != nil {
				return e
			}
			e := walkEntry{parent: dir, name: de.Name(), rel: join(rel, de.Name()), kind: "git"}
			if !strings.EqualFold(e.name, ".git") {
				var se error
				e.kind, e.stat, se = statAt(int(dir.Fd()), e.name)
				if se != nil {
					return se
				}
			}
			switch ve := visit(e); {
			case errors.Is(ve, errSkip):
				continue
			case ve != nil:
				return ve
			}
			if e.kind != "directory" {
				continue
			}
			child, oe := openDirAt(dir, e.name, e.rel)
			if oe != nil {
				return oe
			}
			we := walk(ctx, child, e.rel, visit)
			_ = child.Close()
			if we != nil {
				return we
			}
		}
		if errors.Is(err, io.EOF) || (err == nil && len(list) == 0) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// usage counts node p: one entry for itself plus, for a directory, every
// entry below it, stopping after limit entries.
func (b *binding) usage(ctx context.Context, p string, limit int) (Usage, error) {
	var u Usage
	if p != "" {
		e, err := b.entry(p)
		if err != nil {
			return u, err
		}
		_ = e.parent.Close()
		u.Entries = 1
		if e.kind != "directory" {
			if e.kind == "file" {
				u.Bytes = e.stat.Size
			}
			return u, nil
		}
	}
	dir, err := b.dir(p)
	if err != nil {
		return u, err
	}
	defer func() { _ = dir.Close() }()
	stop := errors.New("usage limit")
	err = walk(ctx, dir, p, func(e walkEntry) error {
		if u.Entries >= limit {
			u.Limited = true
			return stop
		}
		u.Entries++
		if e.kind == "file" {
			u.Bytes += e.stat.Size
		}
		return nil
	})
	if errors.Is(err, stop) {
		err = nil
	}
	return u, fsErrorOrNil("统计", err)
}

func fsErrorOrNil(op string, err error) error {
	if err == nil {
		return nil
	}
	return fsError(op, err)
}

// Usage counts the entries and bytes at and below node nid, for confirming
// deletion of a directory.
func (s *Service) Usage(ctx context.Context, id, cid, nid string) (Usage, error) {
	return withRoot(ctx, s, id, cid, func(b *binding) (Usage, error) {
		p, err := nodePath(nid)
		if err != nil {
			return Usage{}, err
		}
		return b.usage(ctx, p, maxWalkEntries)
	})
}
