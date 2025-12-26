package adapters

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// Ensure implementations satisfy interface
var (
	_ core.FS = (*OSFS)(nil)
	_ core.FS = (*MemoryFS)(nil)
)

// OSFS implements core.FS using the real filesystem
type OSFS struct {
	root string // optional root directory for relative paths
}

// NewOSFS creates a filesystem adapter for the real OS filesystem
func NewOSFS(root string) *OSFS {
	return &OSFS{root: root}
}

func (f *OSFS) path(name string) string {
	if f.root != "" && !filepath.IsAbs(name) {
		return filepath.Join(f.root, name)
	}
	return name
}

func (f *OSFS) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(f.path(path), perm)
}

func (f *OSFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(f.path(name), data, perm)
}

func (f *OSFS) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(f.path(name))
}

func (f *OSFS) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(f.path(name))
}

func (f *OSFS) Remove(name string) error {
	return os.Remove(f.path(name))
}

func (f *OSFS) Symlink(oldname, newname string) error {
	return os.Symlink(oldname, f.path(newname))
}

// MemoryFS implements core.FS using an in-memory filesystem for testing
type MemoryFS struct {
	mu    sync.RWMutex
	files map[string]*memFile
	dirs  map[string]bool
}

type memFile struct {
	data    []byte
	mode    os.FileMode
	modTime time.Time
}

// NewMemoryFS creates an in-memory filesystem for testing
func NewMemoryFS() *MemoryFS {
	return &MemoryFS{
		files: make(map[string]*memFile),
		dirs:  make(map[string]bool),
	}
}

func (f *MemoryFS) MkdirAll(path string, perm os.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	path = filepath.Clean(path)
	// Build all parent paths
	current := ""
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == "" {
			continue
		}
		if current == "" {
			current = part
		} else {
			current = filepath.Join(current, part)
		}
		f.dirs[current] = true
	}
	return nil
}

func (f *MemoryFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	name = filepath.Clean(name)
	f.files[name] = &memFile{
		data:    append([]byte(nil), data...), // copy data
		mode:    perm,
		modTime: time.Now(),
	}
	return nil
}

func (f *MemoryFS) ReadFile(name string) ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	name = filepath.Clean(name)
	file, ok := f.files[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), file.data...), nil // copy data
}

func (f *MemoryFS) Stat(name string) (fs.FileInfo, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	name = filepath.Clean(name)

	if file, ok := f.files[name]; ok {
		return &memFileInfo{name: filepath.Base(name), file: file, isDir: false}, nil
	}

	if f.dirs[name] {
		return &memFileInfo{name: filepath.Base(name), file: nil, isDir: true}, nil
	}

	return nil, os.ErrNotExist
}

func (f *MemoryFS) Remove(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	name = filepath.Clean(name)
	if _, ok := f.files[name]; ok {
		delete(f.files, name)
		return nil
	}
	if f.dirs[name] {
		delete(f.dirs, name)
		return nil
	}
	return os.ErrNotExist
}

func (f *MemoryFS) Symlink(oldname, newname string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	newname = filepath.Clean(newname)
	// In memory filesystem, we just record the symlink as a file with special content
	// For testing purposes, we store the target path
	f.files[newname] = &memFile{
		data:    []byte(oldname),
		mode:    os.ModeSymlink,
		modTime: time.Now(),
	}
	return nil
}

// Files returns all files in the memory filesystem (for test assertions)
func (f *MemoryFS) Files() map[string][]byte {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make(map[string][]byte)
	for name, file := range f.files {
		result[name] = append([]byte(nil), file.data...)
	}
	return result
}

// Dirs returns all directories in the memory filesystem (for test assertions)
func (f *MemoryFS) Dirs() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make([]string, 0, len(f.dirs))
	for dir := range f.dirs {
		result = append(result, dir)
	}
	return result
}

type memFileInfo struct {
	name  string
	file  *memFile
	isDir bool
}

func (fi *memFileInfo) Name() string       { return fi.name }
func (fi *memFileInfo) IsDir() bool        { return fi.isDir }
func (fi *memFileInfo) Mode() os.FileMode  {
	if fi.isDir {
		return os.ModeDir | 0755
	}
	if fi.file != nil {
		return fi.file.mode
	}
	return 0644
}
func (fi *memFileInfo) ModTime() time.Time {
	if fi.file != nil {
		return fi.file.modTime
	}
	return time.Time{}
}
func (fi *memFileInfo) Size() int64 {
	if fi.file != nil {
		return int64(len(fi.file.data))
	}
	return 0
}
func (fi *memFileInfo) Sys() any { return nil }
