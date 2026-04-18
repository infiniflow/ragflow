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
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"ragflow/internal/cli/filesystem"
)

// RAGFlowFS implements the FUSE filesystem root
type RAGFlowFS struct {
	fs.Inode
	engine *filesystem.Engine
}

// NewRAGFlowFS creates a new RAGFlow FUSE filesystem
func NewRAGFlowFS(engine *filesystem.Engine) *RAGFlowFS {
	return &RAGFlowFS{engine: engine}
}

// OpendirHandle opens the root directory
func (rfs *RAGFlowFS) OpendirHandle(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	fmt.Fprintf(os.Stderr, "Debug: RAGFlowFS.OpendirHandle called\n")
	
	// Get root entries from engine
	entries, _ := rfs.readRootEntries(ctx)
	dh := &rootDirHandle{
		entries: entries,
		pos:     0,
	}
	
	fmt.Fprintf(os.Stderr, "Debug: OpendirHandle returning %d entries\n", len(entries))
	return dh, fuse.FOPEN_KEEP_CACHE, 0
}

// readRootEntries reads entries from the root
func (rfs *RAGFlowFS) readRootEntries(ctx context.Context) ([]fuse.DirEntry, uint64) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Always include . and .. entries first
	entries := []fuse.DirEntry{
		{Name: ".", Mode: fuse.S_IFDIR | 0755, Ino: 1},
		{Name: "..", Mode: fuse.S_IFDIR | 0755, Ino: 1},
	}
	
	// Track seen names to avoid duplicates
	seen := map[string]bool{".": true, "..": true}

	result, err := rfs.engine.List(ctx, ".", &filesystem.ListOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Debug: readRootEntries error: %v\n", err)
		return entries, 1
	}

	fmt.Fprintf(os.Stderr, "Debug: readRootEntries found %d entries\n", len(result.Nodes))
	
	for _, child := range result.Nodes {
		// Skip duplicates
		if seen[child.Name] {
			fmt.Fprintf(os.Stderr, "Debug: readRootEntries skipping duplicate: %s\n", child.Name)
			continue
		}
		seen[child.Name] = true
		
		entry := fuse.DirEntry{
			Name: child.Name,
			Mode: getMode(child.Type),
			Ino:  hashPath(child.Path),
		}
		entries = append(entries, entry)
		fmt.Fprintf(os.Stderr, "Debug: readRootEntries adding entry: %s (ino=%d)\n", child.Name, entry.Ino)
	}

	return entries, 1
}

// Lookup finds a child by name
func (rfs *RAGFlowFS) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Fprintf(os.Stderr, "Debug: RAGFlowFS.Lookup called for name=%q\n", name)
	
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result, err := rfs.engine.List(ctx, ".", &filesystem.ListOptions{})
	if err != nil {
		return nil, syscall.EIO
	}

	// Track seen names to avoid returning duplicates
	seen := map[string]bool{}
	
	for _, child := range result.Nodes {
		// Skip duplicates
		if seen[child.Name] {
			continue
		}
		seen[child.Name] = true
		
		if child.Name == name {
			childNode := newNode(rfs.engine, child.Path, child)
			mode := getMode(child.Type)
			ino := hashPath(child.Path)
			
			out.Attr.Mode = mode
			out.Attr.Size = uint64(child.Size)
			out.Attr.Ino = ino
			if !child.CreatedAt.IsZero() {
				out.Attr.Ctime = uint64(child.CreatedAt.Unix())
				out.Attr.Mtime = uint64(child.CreatedAt.Unix())
			}
			out.SetAttrTimeout(1)
			out.SetEntryTimeout(1)

			fmt.Fprintf(os.Stderr, "Debug: Lookup found child=%q, ino=%d\n", child.Name, ino)
			
			inode := rfs.NewInode(ctx, childNode, fs.StableAttr{
				Mode: uint32(mode),
				Ino:  ino,
			})
			return inode, 0
		}
	}

	return nil, syscall.ENOENT
}

// Getattr gets attributes for the filesystem root
func (rfs *RAGFlowFS) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Attr = fuse.Attr{
		Mode: fuse.S_IFDIR | 0755,
		Ino:  1,
	}
	return 0
}

// rootDirHandle implements FileReaddirenter for root directory
type rootDirHandle struct {
	entries []fuse.DirEntry
	pos     int
}

// Readdirent reads a single directory entry
func (dh *rootDirHandle) Readdirent(ctx context.Context) (*fuse.DirEntry, syscall.Errno) {
	if dh.pos >= len(dh.entries) {
		return nil, 0 // EOF
	}
	
	entry := &dh.entries[dh.pos]
	dh.pos++
	
	fmt.Fprintf(os.Stderr, "Debug: Readdirent returning %s\n", entry.Name)
	return entry, 0
}

// Statfs provides filesystem statistics
func (rfs *RAGFlowFS) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	out.Blocks = 0
	out.Bfree = 0
	out.Bavail = 0
	out.Files = 0
	out.Ffree = 0
	out.Bsize = 4096
	out.NameLen = 255
	return 0
}
