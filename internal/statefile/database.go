package statefile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// Database owns a registered SQLite file and its process lifetime lock. SQL
// schema, migrations and transactions belong to the domain service.
type Database struct {
	SQL  *sql.DB
	lock *os.File
}

// OpenDatabase opens only registered database storage. SQLite inherits the
// database's 0600 mode for rollback journals; the parent is private as well.
// DELETE journaling avoids retaining deleted conversation bodies in a WAL.
func OpenDatabase(ctx context.Context, dir string, id ID) (*Database, error) {
	d := Describe(id)
	if id != Conversation || d.ID == "" {
		return nil, errors.New("unregistered database")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, d.Path)
	lock, err := openPrivate(path+".lock", true)
	if err != nil {
		return nil, err
	}
	db := &Database{lock: lock}
	fail := func(err error) (*Database, error) { _ = db.Close(); return nil, err }
	if err := lock.Chmod(Perm); err != nil {
		return fail(err)
	}
	if err := lockFile(lock); err != nil {
		return fail(fmt.Errorf("database locked: %w", err))
	}
	// Refuse links and special files, including crash-recovery sidecars, before
	// handing their names to SQLite. The same-user process is not a sandbox.
	for _, suffix := range []string{"", "-journal", "-wal", "-shm"} {
		f, err := openPrivate(path+suffix, suffix == "")
		if errors.Is(err, os.ErrNotExist) && suffix != "" {
			continue
		}
		if err != nil {
			return fail(err)
		}
		err = f.Chmod(Perm)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return fail(err)
		}
	}
	u := url.URL{Scheme: "file", Path: path}
	q := url.Values{}
	for _, p := range []string{"foreign_keys(1)", "busy_timeout(1000)", "synchronous(FULL)", "secure_delete(1)"} {
		q.Add("_pragma", p)
	}
	u.RawQuery = q.Encode()
	db.SQL, err = sql.Open("sqlite", u.String())
	if err != nil {
		return fail(err)
	}
	// One connection serializes transactions without holding a Go state lock
	// across disk I/O. Callers retain context cancellation while waiting.
	db.SQL.SetMaxOpenConns(1)
	if err := db.SQL.PingContext(ctx); err != nil {
		return fail(err)
	}
	return db, nil
}

func (d *Database) Close() error {
	if d == nil {
		return nil
	}
	var err error
	if d.SQL != nil {
		err = d.SQL.Close()
	}
	if d.lock != nil {
		unlockFile(d.lock)
		err = errors.Join(err, d.lock.Close())
		d.lock = nil
	}
	return err
}
