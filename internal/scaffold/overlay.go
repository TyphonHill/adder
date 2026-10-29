package scaffold

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"
)

// overlay adds in-memory files to a file system, so scans see new files before they are written.
type overlay struct {
	base  fs.FS
	files map[string][]byte // by slash-separated path
}

func (o overlay) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if data, ok := o.files[name]; ok {
		return &memFile{info: fileInfo{name: path.Base(name), size: int64(len(data))}, Reader: bytes.NewReader(data)}, nil
	}
	if !o.hasFilesIn(name) {
		return o.base.Open(name)
	}
	info, err := o.Stat(name)
	if err != nil {
		return nil, err
	}
	entries, err := o.ReadDir(name)
	if err != nil {
		return nil, err
	}
	return &memDir{info: info, entries: entries}, nil
}

func (o overlay) Stat(name string) (fs.FileInfo, error) {
	if data, ok := o.files[name]; ok {
		return fileInfo{name: path.Base(name), size: int64(len(data))}, nil
	}
	if o.newDir(name) {
		return fileInfo{name: path.Base(name), dir: true}, nil
	}
	return fs.Stat(o.base, name)
}

func (o overlay) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(o.base, name)
	if err != nil && (!errors.Is(err, fs.ErrNotExist) || !o.hasFilesIn(name)) {
		return nil, err
	}

	prefix := name + "/"
	if name == "." {
		prefix = ""
	}
	for file, data := range o.files {
		rest, ok := strings.CutPrefix(file, prefix)
		if !ok {
			continue
		}
		child, _, nested := strings.Cut(rest, "/")
		entry := fileInfo{name: child, dir: true}
		if !nested {
			entry = fileInfo{name: child, size: int64(len(data))}
		}
		if i := slices.IndexFunc(entries, func(e fs.DirEntry) bool { return e.Name() == child }); i >= 0 {
			if !nested {
				entries[i] = entry
			}
			continue
		}
		entries = append(entries, entry)
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, nil
}

// newDir reports whether name is a directory that only exists in the overlay.
func (o overlay) newDir(name string) bool {
	if _, err := fs.Stat(o.base, name); err == nil {
		return false
	}
	return o.hasFilesIn(name)
}

func (o overlay) hasFilesIn(dir string) bool {
	if dir == "." {
		return len(o.files) > 0
	}
	for file := range o.files {
		if strings.HasPrefix(file, dir+"/") {
			return true
		}
	}
	return false
}

// fileInfo describes an in-memory file or directory, as both fs.FileInfo and fs.DirEntry.
type fileInfo struct {
	name string
	size int64
	dir  bool
}

func (fi fileInfo) Name() string               { return fi.name }
func (fi fileInfo) Size() int64                { return fi.size }
func (fi fileInfo) ModTime() time.Time         { return time.Time{} }
func (fi fileInfo) IsDir() bool                { return fi.dir }
func (fi fileInfo) Sys() any                   { return nil }
func (fi fileInfo) Type() fs.FileMode          { return fi.Mode().Type() }
func (fi fileInfo) Info() (fs.FileInfo, error) { return fi, nil }

func (fi fileInfo) Mode() fs.FileMode {
	if fi.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}

type memFile struct {
	info fileInfo
	*bytes.Reader
}

func (f *memFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *memFile) Close() error               { return nil }

// memDir is a directory with overlay files in it, listing the base's entries and the overlay's together.
type memDir struct {
	info    fs.FileInfo
	entries []fs.DirEntry
}

func (d *memDir) Stat() (fs.FileInfo, error) { return d.info, nil }
func (d *memDir) Close() error               { return nil }

func (d *memDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.info.Name(), Err: errors.New("is a directory")}
}

func (d *memDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if n <= 0 {
		entries := d.entries
		d.entries = nil
		return entries, nil
	}
	if len(d.entries) == 0 {
		return nil, io.EOF
	}
	n = min(n, len(d.entries))
	entries := d.entries[:n]
	d.entries = d.entries[n:]
	return entries, nil
}
