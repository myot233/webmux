package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CopyStagedUpload preserves a validated quick-start upload in a live or
// retained source runtime's existing ownership namespace. It never moves the
// staged file: other forms may still use it until the normal startup cleanup.
func (s *Service) CopyStagedUpload(id, source string, size int64) (string, error) {
	clean, err := s.StagedPath(source)
	if err != nil || size < 0 || size > 10<<20 {
		return "", ErrInvalid
	}
	file, err := os.OpenInRoot(filepath.Dir(clean), filepath.Base(clean))
	if err != nil {
		return "", ErrInvalid
	}
	hash := sha256.New()
	n, copyErr := io.Copy(hash, io.LimitReader(file, size+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || n != size {
		return "", ErrInvalid
	}
	dir, err := s.ensure(id, UploadsDir, true)
	if err != nil {
		return "", err
	}
	return copyUpload(dir, clean, filepath.Base(clean), hex.EncodeToString(hash.Sum(nil)), size)
}

// CopyUpload atomically publishes a target-owned copy under a deterministic
// name. The source has already passed its owner's upload validator; the saved
// digest makes both an interrupted copy and a repeated request safe to resume.
func (s *Service) CopyUpload(id, source, name, digest string, size int64) (string, error) {
	if name == "" || filepath.Base(name) != name || strings.HasPrefix(name, ".") || len(digest) != 64 || size < 0 || size > 10<<20 {
		return "", ErrInvalid
	}
	dir, err := s.EnsureUploads(id)
	if err != nil {
		return "", err
	}
	return copyUpload(dir, source, name, digest, size)
}

func copyUpload(dir, source, name, digest string, size int64) (string, error) {
	target := filepath.Join(dir, name)
	matches := func(path string) bool {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != size {
			return false
		}
		file, err := os.OpenInRoot(filepath.Dir(path), filepath.Base(path))
		if err != nil {
			return false
		}
		defer func() { _ = file.Close() }()
		hash := sha256.New()
		n, err := io.Copy(hash, io.LimitReader(file, size+1))
		return err == nil && n == size && hex.EncodeToString(hash.Sum(nil)) == digest
	}
	if _, err := os.Lstat(target); err == nil {
		if matches(target) {
			return target, nil
		}
		return "", ErrInvalid
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", ErrUnavailable
	}
	input, err := os.OpenInRoot(filepath.Dir(source), filepath.Base(source))
	if err != nil {
		return "", ErrInvalid
	}
	defer func() { _ = input.Close() }()
	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return "", ErrInvalid
	}
	output, err := os.CreateTemp(dir, ".handoff-")
	if err != nil {
		return "", ErrUnavailable
	}
	defer func() { _ = os.Remove(output.Name()) }()
	defer func() { _ = output.Close() }()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(output, hash), io.LimitReader(input, size+1))
	if err != nil || n != size || hex.EncodeToString(hash.Sum(nil)) != digest {
		return "", ErrInvalid
	}
	if err := output.Sync(); err != nil {
		return "", ErrUnavailable
	}
	// Link publishes without replacing an existing upload. Another attempt can
	// only win with the exact same bytes and saved identity.
	if err := os.Link(output.Name(), target); err != nil {
		if !matches(target) {
			return "", ErrUnavailable
		}
	}
	folder, err := os.Open(dir)
	if err != nil {
		return "", ErrUnavailable
	}
	syncErr := folder.Sync()
	closeErr := folder.Close()
	if syncErr != nil || closeErr != nil {
		return "", ErrUnavailable
	}
	return target, nil
}
