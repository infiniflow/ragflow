//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package fuse

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"ragflow/internal/cli/filesystem"
)

// node represents a file or directory in the FUSE filesystem
type node struct {
	fs.Inode
	engine   *filesystem.Engine
	path     string
	nodeInfo *filesystem.Node
}

// newNode creates a new FUSE node
func newNode(engine *filesystem.Engine, path string, info *filesystem.Node) *node {
	n := &node{
		engine:   engine,
		path:     path,
		nodeInfo: info,
	}
	return n
}

// Ensure node implements required interfaces
var _ fs.NodeOpendirHandler = (*node)(nil)
var _ fs.FileReaddirenter = (*dirHandle)(nil)

// readDirEntries returns directory entries and parent inode
func (n *node) readDirEntries(ctx context.Context) ([]fuse.DirEntry, uint64) {
	// Use a timeout context to prevent hanging
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Normalize path for listing
	listPath := n.path
	if listPath == "" {
		listPath = "."
	}
	
	parentIno := hashPath(n.path)
	if n.path == "" {
		parentIno = 1
	}

	// Always include . and .. entries first
	entries := []fuse.DirEntry{
		{Name: ".", Mode: fuse.S_IFDIR | 0755, Ino: parentIno},
		{Name: "..", Mode: fuse.S_IFDIR | 0755, Ino: 1},
	}

	result, err := n.engine.List(ctx, listPath, &filesystem.ListOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Debug: readDirEntries error: %v\n", err)
		return entries, parentIno
	}

	fmt.Fprintf(os.Stderr, "Debug: readDirEntries found %d entries\n", len(result.Nodes))
	
	for _, child := range result.Nodes {
		entry := fuse.DirEntry{
			Name: child.Name,
			Mode: getMode(child.Type),
			Ino:  hashPath(child.Path),
		}
		entries = append(entries, entry)
		fmt.Fprintf(os.Stderr, "Debug: readDirEntries adding entry: %s (ino=%d)\n", child.Name, entry.Ino)
	}

	fmt.Fprintf(os.Stderr, "Debug: readDirEntries returning %d total entries\n", len(entries))
	return entries, parentIno
}

// Lookup finds a child node by name
func (n *node) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Printf("Debug: Lookup called for parent=%q, name=%q\n", n.path, name)
	
	childPath := n.path + "/" + name
	if n.path == "" {
		childPath = name
	}

	result, err := n.engine.List(ctx, n.path, &filesystem.ListOptions{})
	if err != nil {
		fmt.Printf("Debug: Lookup error: %v\n", err)
		return nil, syscall.EIO
	}

	for _, child := range result.Nodes {
		if child.Name == name {
			childNode := newNode(n.engine, childPath, child)
			mode := getMode(child.Type)
			ino := hashPath(childPath)
			
			// Set entry out attributes
			out.Attr.Mode = mode
			out.Attr.Size = uint64(child.Size)
			out.Attr.Ino = ino
			if !child.CreatedAt.IsZero() {
				out.Attr.Ctime = uint64(child.CreatedAt.Unix())
				out.Attr.Mtime = uint64(child.CreatedAt.Unix())
			}
			out.SetAttrTimeout(1)
			out.SetEntryTimeout(1)

			fmt.Printf("Debug: Lookup found child=%q, ino=%d\n", child.Name, ino)
			
			// Create stable attr with unique inode based on path hash
			inode := n.NewInode(ctx, childNode, fs.StableAttr{
				Mode: uint32(mode),
				Ino:  ino,
			})
			return inode, 0
		}
	}

	fmt.Printf("Debug: Lookup not found: %q\n", name)
	return nil, syscall.ENOENT
}

// hashPath generates a simple hash for inode numbers
func hashPath(path string) uint64 {
	h := uint64(0)
	for i := 0; i < len(path); i++ {
		h = h*31 + uint64(path[i])
	}
	if h == 0 {
		h = 1
	}
	return h
}

// OnAdd is called when the node is added to the tree
func (n *node) OnAdd(ctx context.Context) {
	fmt.Fprintf(os.Stderr, "Debug: OnAdd called for path: %q\n", n.path)
}

// Getattr gets file attributes
func (n *node) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	fmt.Fprintf(os.Stderr, "Debug: Getattr called for path: %q\n", n.path)
	
	mode := uint32(fuse.S_IFDIR | 0755)
	size := uint64(0)
	var createdAt int64
	ino := hashPath(n.path)
	if n.path == "" {
		ino = 1 // Root inode is 1
	}

	if n.nodeInfo != nil {
		mode = getMode(n.nodeInfo.Type)
		size = uint64(n.nodeInfo.Size)
		if !n.nodeInfo.CreatedAt.IsZero() {
			createdAt = n.nodeInfo.CreatedAt.Unix()
		}
	}

	out.Attr = fuse.Attr{
		Mode: mode,
		Size: size,
		Ino:  ino,
	}
	if createdAt > 0 {
		out.Attr.Ctime = uint64(createdAt)
		out.Attr.Mtime = uint64(createdAt)
	}
	fmt.Fprintf(os.Stderr, "Debug: Getattr returning mode=%o, size=%d, ino=%d\n", mode, size, ino)
	return 0
}

// OpendirHandle opens a directory and returns a file handle for reading
// Implements NodeOpendirHandler interface
func (n *node) OpendirHandle(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	fmt.Fprintf(os.Stderr, "Debug: OpendirHandle called for path: %q\n", n.path)
	
	entries, _ := n.readDirEntries(ctx)
	dh := &dirHandle{
		entries: entries,
		pos:     0,
	}
	
	fmt.Fprintf(os.Stderr, "Debug: OpendirHandle returning dirHandle with %d entries\n", len(entries))
	return dh, fuse.FOPEN_KEEP_CACHE, 0
}

// dirHandle implements FileReaddirenter for directory reading
type dirHandle struct {
	entries []fuse.DirEntry
	pos     int
}

// Readdirent reads a single directory entry (implements FileReaddirenter)
// Note: go-fuse uses *fuse.Context, not context.Context
func (dh *dirHandle) Readdirent(ctx context.Context) (*fuse.DirEntry, syscall.Errno) {
	if dh.pos >= len(dh.entries) {
		return nil, 0 // EOF
	}
	
	entry := &dh.entries[dh.pos]
	dh.pos++
	
	fmt.Fprintf(os.Stderr, "Debug: Readdirent returning %s\n", entry.Name)
	return entry, 0
}

// Open opens a file for reading
func (n *node) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	if n.nodeInfo == nil || n.nodeInfo.Type != filesystem.NodeTypeFile {
		return nil, 0, syscall.EISDIR
	}

	content, err := n.engine.Cat(ctx, n.path)
	if err != nil {
		return nil, 0, syscall.EIO
	}

	return &fileHandle{content: content}, fuse.FOPEN_KEEP_CACHE, 0
}

// fileHandle implements fs.FileHandle for reading file content
type fileHandle struct {
	content []byte
}

// Read reads file content
func (fh *fileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	if off >= int64(len(fh.content)) {
		return fuse.ReadResultData(nil), 0
	}

	end := int(off) + len(dest)
	if end > len(fh.content) {
		end = len(fh.content)
	}

	return fuse.ReadResultData(fh.content[off:end]), 0
}

// getMode converts NodeType to fuse mode
func getMode(t filesystem.NodeType) uint32 {
	switch t {
	case filesystem.NodeTypeDirectory, filesystem.NodeTypeDataset:
		return fuse.S_IFDIR | 0755
	case filesystem.NodeTypeFile, filesystem.NodeTypeDocument:
		return fuse.S_IFREG | 0644
	default:
		return fuse.S_IFDIR | 0755
	}
}

// getModeInt returns mode as uint32 for fs.StableAttr.Mode (which is actually uint32 in go-fuse v2)
func getModeInt(t filesystem.NodeType) uint32 {
	return getMode(t)
}

// normalizePath normalizes a path for consistent lookups
func normalizePath(path string) string {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return ""
	}
	return path
}
