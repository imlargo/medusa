package postgres

import (
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"
)

// MergeMigrations returns one file system holding the .sql migrations found
// at the root, or in the migrations directory, of each of sources. It is how
// an application whose features each own their migrations gives [Migrate]
// one history: goose orders the files by the version their names start
// with, whichever feature they came from.
//
//	var All = postgres.MustMergeMigrations(user.Migrations, billing.Migrations)
//
// Two files with the same name are an error, and so are two with the same
// version, which goose reports when it loads them. Name migrations by
// timestamp (20261128153000_create_orders.sql), so two features never pick
// the same version.
func MergeMigrations(sources ...fs.FS) (fs.FS, error) {
	merged := mergedFS{}

	for _, source := range sources {
		dir := "."
		if _, err := fs.Stat(source, "migrations"); err == nil {
			dir = "migrations"
		}

		entries, err := fs.ReadDir(source, dir)
		if err != nil {
			return nil, fmt.Errorf("merge migrations: %w", err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
				continue
			}
			if _, taken := merged[entry.Name()]; taken {
				return nil, fmt.Errorf("merge migrations: two migrations are named %s", entry.Name())
			}

			content, err := fs.ReadFile(source, path.Join(dir, entry.Name()))
			if err != nil {
				return nil, fmt.Errorf("merge migrations: %w", err)
			}
			merged[entry.Name()] = content
		}
	}

	return merged, nil
}

// MustMergeMigrations is MergeMigrations for a package variable. It panics on
// an error, which is a mistake in the files found when the program starts.
func MustMergeMigrations(sources ...fs.FS) fs.FS {
	merged, err := MergeMigrations(sources...)
	if err != nil {
		panic(err.Error())
	}
	return merged
}

// mergedFS is a flat, read-only file system in memory: file name to content.
type mergedFS map[string][]byte

func (m mergedFS) Open(name string) (fs.File, error) {
	if name == "." {
		names := make([]string, 0, len(m))
		for name := range m {
			names = append(names, name)
		}
		slices.Sort(names)
		return &mergedDir{fsys: m, names: names}, nil
	}

	content, ok := m[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &mergedFile{info: mergedInfo{name: name, size: int64(len(content))}, reader: strings.NewReader(string(content))}, nil
}

type mergedFile struct {
	info   mergedInfo
	reader *strings.Reader
}

func (f *mergedFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *mergedFile) Read(p []byte) (int, error) { return f.reader.Read(p) }
func (f *mergedFile) Close() error               { return nil }

type mergedDir struct {
	fsys  mergedFS
	names []string
	next  int
}

func (d *mergedDir) Stat() (fs.FileInfo, error) { return mergedInfo{name: ".", dir: true}, nil }
func (d *mergedDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: ".", Err: fs.ErrInvalid}
}
func (d *mergedDir) Close() error { return nil }

func (d *mergedDir) ReadDir(n int) ([]fs.DirEntry, error) {
	remaining := d.names[d.next:]
	if n > 0 && len(remaining) > n {
		remaining = remaining[:n]
	}
	if n > 0 && len(remaining) == 0 {
		return nil, io.EOF
	}

	entries := make([]fs.DirEntry, len(remaining))
	for i, name := range remaining {
		entries[i] = fs.FileInfoToDirEntry(mergedInfo{name: name, size: int64(len(d.fsys[name]))})
	}
	d.next += len(remaining)
	return entries, nil
}

type mergedInfo struct {
	name string
	size int64
	dir  bool
}

func (i mergedInfo) Name() string       { return i.name }
func (i mergedInfo) Size() int64        { return i.size }
func (i mergedInfo) ModTime() time.Time { return time.Time{} }
func (i mergedInfo) IsDir() bool        { return i.dir }
func (i mergedInfo) Sys() any           { return nil }
func (i mergedInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
