// Package filesystem provides file system abstractions for agent file operations.
package backend

// FileInfo provides information about a file.
type FileInfo struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time"`
}

// Backend defines the interface for file system operations.
type Backend interface {
	Read(path string) (string, error)
	Write(path, content string) error
	Edit(path, old, new string) error
	Glob(pattern string) ([]string, error)
	Grep(pattern, path string) (string, error)
	Stat(path string) (*FileInfo, error)
	Mkdir(path string) error
	Remove(path string) error
	List(dir string) ([]FileInfo, error)
}

// Shell defines an interface for shell execution.
type Shell interface {
	Execute(command string) (string, error)
	ExecuteStreaming(command string) (<-chan string, error)
}

// MultiModalReader is an optional interface that backends can implement
// to support reading with offset/limit and multi-modal content detection.
type MultiModalReader interface {
	ReadBytes(path string, offset, limit int64) ([]byte, error)
	MimeType(path string) string // Returns content type hint (e.g., "text/plain", "image/png")
}
