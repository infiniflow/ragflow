package skill

import (
	"os"
	"path/filepath"
)

// OSFileSystemBackend implements FileSystemBackend using the OS filesystem.
type OSFileSystemBackend struct {
	baseDir string
}

// NewOSFileSystemBackend creates a new OSFileSystemBackend.
func NewOSFileSystemBackend(baseDir string) *OSFileSystemBackend {
	return &OSFileSystemBackend{baseDir: baseDir}
}

func (b *OSFileSystemBackend) Read(path string) (string, error) {
	data, err := os.ReadFile(b.resolve(path))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (b *OSFileSystemBackend) List() ([]string, error) {
	entries, err := os.ReadDir(b.baseDir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".md" {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func (b *OSFileSystemBackend) Exists(path string) bool {
	_, err := os.Stat(b.resolve(path))
	return err == nil
}

func (b *OSFileSystemBackend) resolve(path string) string {
	if b.baseDir == "" {
		return path
	}
	return b.baseDir + "/" + path
}
