package backend

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// InMemoryBackend implements Backend using in-memory storage.
// Useful for testing and sandboxed environments.
type InMemoryBackend struct {
	mu     sync.RWMutex
	files  map[string]*memFile
}

type memFile struct {
	content  string
	modTime  time.Time
	isDir    bool
}

func NewInMemoryBackend() *InMemoryBackend {
	root := &memFile{content: "", modTime: time.Now(), isDir: true}
	return &InMemoryBackend{files: map[string]*memFile{"": root, ".": root}}
}

func (b *InMemoryBackend) Read(path string) (string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	f, ok := b.files[filepath.Clean(path)]
	if !ok { return "", fmt.Errorf("file not found: %s", path) }
	if f.isDir { return "", fmt.Errorf("is a directory: %s", path) }
	return f.content, nil
}

func (b *InMemoryBackend) Write(path, content string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.files[filepath.Clean(path)] = &memFile{content: content, modTime: time.Now()}
	return nil
}

func (b *InMemoryBackend) Edit(path, old, new string) error {
	content, err := b.Read(path)
	if err != nil { return err }
	updated := strings.Replace(content, old, new, 1)
	if updated == content { return fmt.Errorf("text not found in %s", path) }
	return b.Write(path, updated)
}

func (b *InMemoryBackend) Glob(pattern string) ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var matches []string
	for p := range b.files {
		if matched, _ := filepath.Match(pattern, p); matched { matches = append(matches, p) }
	}
	return matches, nil
}

func (b *InMemoryBackend) Grep(pattern, path string) (string, error) {
	content, err := b.Read(path)
	if err != nil { return "", err }
	var results []string
	for i, line := range strings.Split(content, "\n") {
		if strings.Contains(line, pattern) { results = append(results, fmt.Sprintf("%s:%d: %s", path, i+1, line)) }
	}
	return strings.Join(results, "\n"), nil
}

func (b *InMemoryBackend) Stat(path string) (*FileInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	f, ok := b.files[filepath.Clean(path)]
	if !ok { return nil, fmt.Errorf("not found: %s", path) }
	return &FileInfo{Name: path, Size: int64(len(f.content)), IsDir: f.isDir, ModTime: f.modTime.Format(time.RFC3339)}, nil
}

func (b *InMemoryBackend) Mkdir(path string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.files[filepath.Clean(path)] = &memFile{modTime: time.Now(), isDir: true}
	return nil
}

func (b *InMemoryBackend) Remove(path string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.files, filepath.Clean(path))
	return nil
}

func (b *InMemoryBackend) List(dir string) ([]FileInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var results []FileInfo
	clean := filepath.Clean(dir)
	for p, f := range b.files {
		if filepath.Dir(p) == clean {
			results = append(results, FileInfo{Name: p, Size: int64(len(f.content)), IsDir: f.isDir, ModTime: f.modTime.Format(time.RFC3339)})
		}
	}
	return results, nil
}

func (b *InMemoryBackend) Execute(command string) (string, error) {
	return fmt.Sprintf("executed (in-memory): %s", command), nil
}

func (b *InMemoryBackend) ReadBytes(path string, offset, limit int64) ([]byte, error) {
	content, err := b.Read(path)
	if err != nil {
		return nil, err
	}
	if offset < 0 {
		return nil, fmt.Errorf("negative offset %d", offset)
	}
	if limit < 0 {
		return nil, fmt.Errorf("negative limit %d", limit)
	}
	runes := []rune(content)
	if int(offset) >= len(runes) {
		return nil, fmt.Errorf("offset %d beyond content length %d", offset, len(runes))
	}
	end := int(offset) + int(limit)
	if end < int(offset) { // integer overflow
		end = len(runes)
	}
	if end > len(runes) {
		end = len(runes)
	}
	return []byte(string(runes[offset:end])), nil
}

func (b *InMemoryBackend) MimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt", ".go", ".py", ".js", ".ts", ".html", ".css", ".md", ".json", ".xml", ".yaml", ".yml":
		return "text/plain"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}
