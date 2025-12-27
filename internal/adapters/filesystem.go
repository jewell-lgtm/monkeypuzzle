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
		// Clean the path first to resolve any .. components
		cleaned := filepath.Clean(name)

		// If cleaning resulted in an absolute path, it means the original path
		// contained enough .. to escape to root (e.g., ../../../etc becomes /etc)
		// Reject it immediately
		if filepath.IsAbs(cleaned) {
			// Path escapes root, return a path that will cause safe failure
			return filepath.Join(f.root, ".invalid-path-traversal-detected")
		}

		// Join with root to get the full path
		fullPath := filepath.Join(f.root, cleaned)

		// Ensure the resolved path is within root to prevent path traversal
		// Compute relative path from root - if it contains "..", the path escaped
		rel, err := filepath.Rel(f.root, fullPath)
		if err != nil {
			// If we can't compute relative path, the path is invalid
			// Return a path guaranteed to fail safely (non-existent path within root)
			return filepath.Join(f.root, ".invalid-path-traversal-detected")
		}

		// Check if the relative path tries to escape root (starts with ..)
		// Note: filepath.Rel can return paths starting with .. if the target is outside root
		if strings.HasPrefix(rel, "..") {
			// Path escapes root, return a path that will cause safe failure
			// This path is guaranteed to be within root but won't exist
			return filepath.Join(f.root, ".invalid-path-traversal-detected")
		}

		return fullPath
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

func (f *OSFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(f.path(name))
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
	// Normalize path to match how ReadFile/Stat look up paths
	if filepath.IsAbs(name) && len(name) > 1 {
		name = name[1:] // Remove leading slash to match lookup format
	}
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
	// Normalize path to match how paths are stored (without leading slash for absolute paths)
	if filepath.IsAbs(name) && len(name) > 1 {
		name = name[1:] // Remove leading slash to match storage format
	}
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
	// Normalize path to match how MkdirAll stores it (without leading slash for absolute paths)
	// MkdirAll splits by separator and rebuilds, which removes leading slash
	if filepath.IsAbs(name) && len(name) > 1 {
		name = name[1:] // Remove leading slash to match MkdirAll storage
	}

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
	// Normalize path to match how paths are stored (without leading slash for absolute paths)
	if filepath.IsAbs(name) && len(name) > 1 {
		name = name[1:] // Remove leading slash to match storage format
	}
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
	// Normalize path to match how ReadFile/Stat look up paths
	if filepath.IsAbs(newname) && len(newname) > 1 {
		newname = newname[1:] // Remove leading slash to match lookup format
	}
	// In memory filesystem, we just record the symlink as a file with special content
	// For testing purposes, we store the target path
	f.files[newname] = &memFile{
		data:    []byte(oldname),
		mode:    os.ModeSymlink,
		modTime: time.Now(),
	}
	return nil
}

func (f *MemoryFS) ReadDir(name string) ([]fs.DirEntry, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	name = filepath.Clean(name)
	// Normalize path
	if filepath.IsAbs(name) && len(name) > 1 {
		name = name[1:]
	}

	// Check if directory exists
	if !f.dirs[name] {
		return nil, os.ErrNotExist
	}

	// Find all direct children (files and subdirs)
	var entries []fs.DirEntry
	seen := make(map[string]bool)

	prefix := name + string(filepath.Separator)
	if name == "" || name == "." {
		prefix = ""
	}

	// Check files
	for path, file := range f.files {
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		rel := strings.TrimPrefix(path, prefix)
		// Only direct children (no separator in relative path)
		if rel != "" && !strings.Contains(rel, string(filepath.Separator)) {
			if !seen[rel] {
				seen[rel] = true
				entries = append(entries, &memDirEntry{
					name:  rel,
					isDir: false,
					file:  file,
				})
			}
		}
	}

	// Check subdirectories
	for dir := range f.dirs {
		if !strings.HasPrefix(dir, prefix) {
			continue
		}
		rel := strings.TrimPrefix(dir, prefix)
		// Only direct children
		if rel != "" && !strings.Contains(rel, string(filepath.Separator)) {
			if !seen[rel] {
				seen[rel] = true
				entries = append(entries, &memDirEntry{
					name:  rel,
					isDir: true,
					file:  nil,
				})
			}
		}
	}

	return entries, nil
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

func (fi *memFileInfo) Name() string { return fi.name }
func (fi *memFileInfo) IsDir() bool  { return fi.isDir }
func (fi *memFileInfo) Mode() os.FileMode {
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

type memDirEntry struct {
	name  string
	isDir bool
	file  *memFile
}

func (de *memDirEntry) Name() string { return de.name }
func (de *memDirEntry) IsDir() bool  { return de.isDir }
func (de *memDirEntry) Type() fs.FileMode {
	if de.isDir {
		return fs.ModeDir
	}
	return 0
}
func (de *memDirEntry) Info() (fs.FileInfo, error) {
	return &memFileInfo{name: de.name, file: de.file, isDir: de.isDir}, nil
}
