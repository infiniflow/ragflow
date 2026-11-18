// Command machorename rewrites symbol names inside a Mach-O static archive.
//
// It is the macOS stand-in for `objcopy --redefine-syms`, which the build needs
// to keep two copies of the same library out of each other's namespace in one
// link: the tokenizer's bundled re2 vs onnxruntime's re2, and libpdf_oxide.a's
// bundled office_oxide vs liboffice_oxide.a. macOS ships no objcopy at all
// (llvm-objcopy comes only with a Homebrew LLVM install), and ld64 has no
// equivalent of GNU ld's --allow-multiple-definition, so the rename has to be
// done on the archive.
//
// Usage:
//
//	go run ./tools/machorename --redefine-syms=rename.map librag_tokenizer_c_api.a
//
// The map file holds one `old new` pair per line, exactly like objcopy's
// --redefine-syms. Every renamed symbol is pointed at its new name appended to
// the member's string table, so no section, relocation or symbol index moves.
// Member sizes change, therefore the archive's table of contents is dropped
// and regenerated with ranlib (part of the base macOS toolchain).
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// arMagic starts every ar archive; each member then has a fixed 60-byte header
// ending in "`\n".
const (
	arMagic      = "!<arch>\n"
	arHeaderSize = 60
)

// Mach-O magic numbers and load commands this tool understands. Only thin
// object files appear inside a static archive we rename.
const (
	magic32LE    = 0xfeedface
	magic32BE    = 0xcefaedfe
	magic64LE    = 0xfeedfacf
	magic64BE    = 0xcffaedfe
	fatMagicLE   = 0xbebafeca // FAT_MAGIC read as little endian
	fatMagic64LE = 0xbfbafeca
	lcSymtab     = 0x2
	nlist64Len   = 16
	nlist32Len   = 12
)

func main() {
	mapPath := flag.String("redefine-syms", "", "file holding one `old new` symbol pair per line")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: machorename --redefine-syms=rename.map <archive>\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *mapPath == "" || flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	archivePath := flag.Arg(0)

	renames, err := readRenameMap(*mapPath)
	if err != nil {
		fatal(err)
	}
	renamed, err := renameArchiveFile(archivePath, renames)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("machorename: renamed %d symbol(s) in %s\n", renamed, archivePath)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "machorename: %v\n", err)
	os.Exit(1)
}

// readRenameMap parses objcopy's --redefine-syms format: two whitespace
// separated names per line, blank lines and `#` comments ignored.
func readRenameMap(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	renames := make(map[string]string)
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s:%d: expected `old new`, got %q", path, i+1, line)
		}
		renames[fields[0]] = fields[1]
	}
	return renames, nil
}

// renameArchiveFile rewrites the archive in place. The new archive is built
// next to the original and only replaces it after ranlib succeeded, so a
// failure leaves the input usable.
func renameArchiveFile(path string, renames map[string]string) (int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	out, renamed, err := renameArchive(data, renames)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".renaming-*")
	if err != nil {
		return 0, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if err := rebuildTableOfContents(tmpPath); err != nil {
		return 0, err
	}
	// Replace the archive without changing who may read it.
	if err := os.Chmod(tmpPath, info.Mode().Perm()); err != nil {
		return 0, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return 0, err
	}
	return renamed, nil
}

// rebuildTableOfContents regenerates the archive's symbol index. Member sizes
// (and with them member offsets) change, so the index cannot be carried over.
// ld refuses an archive without one.
func rebuildTableOfContents(archivePath string) error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	ranlib, err := exec.LookPath("ranlib")
	if err != nil {
		return errors.New("ranlib not found (part of the Xcode command line tools)")
	}
	if out, err := exec.Command(ranlib, archivePath).CombinedOutput(); err != nil {
		return fmt.Errorf("ranlib %s: %v: %s", archivePath, err, bytes.TrimSpace(out))
	}
	return nil
}

// arMember is one archive member. prefix holds the BSD long-name bytes that
// ar stores at the front of the payload, object the member's own content.
type arMember struct {
	header []byte
	name   string
	prefix []byte
	object []byte
}

// renameArchive renames the mapped symbols in every object member and returns
// the rewritten archive. The old table of contents is dropped: it is rebuilt
// by ranlib once the new file is on disk.
func renameArchive(data []byte, renames map[string]string) ([]byte, int, error) {
	members, err := parseArchive(data)
	if err != nil {
		return nil, 0, err
	}
	kept := make([]arMember, 0, len(members))
	renamed := 0
	for _, m := range members {
		if isTableOfContents(m.name) {
			continue
		}
		object, n, err := renameObject(m.object, renames)
		if err != nil {
			if errors.Is(err, errNotMachO) {
				fmt.Fprintf(os.Stderr, "machorename: warning: %s: %v\n", m.name, err)
				object = m.object
			} else {
				return nil, 0, fmt.Errorf("%s: %w", m.name, err)
			}
		}
		renamed += n
		m.object = object
		kept = append(kept, m)
	}
	return writeArchive(kept), renamed, nil
}

// isTableOfContents reports whether the member is the archive's symbol index:
// BSD names it __.SYMDEF[ SORTED], GNU "/" (symbols) or "//" (long names).
func isTableOfContents(name string) bool {
	return name == "/" || name == "//" || strings.HasPrefix(name, "__.SYMDEF")
}

func parseArchive(data []byte) ([]arMember, error) {
	if len(data) < len(arMagic) || string(data[:len(arMagic)]) != arMagic {
		return nil, errors.New("not an ar archive")
	}
	var members []arMember
	for off := len(arMagic); off < len(data); {
		if off+arHeaderSize > len(data) {
			return nil, fmt.Errorf("truncated member header at offset %d", off)
		}
		header := data[off : off+arHeaderSize]
		if string(header[58:60]) != "`\n" {
			return nil, fmt.Errorf("bad member header at offset %d", off)
		}
		size, err := strconv.Atoi(strings.TrimSpace(string(header[48:58])))
		if err != nil || size < 0 || off+arHeaderSize+size > len(data) {
			return nil, fmt.Errorf("bad member size at offset %d", off)
		}
		payload := data[off+arHeaderSize : off+arHeaderSize+size]
		name, prefixLen, err := memberName(header[:16], payload)
		if err != nil {
			return nil, err
		}
		members = append(members, arMember{
			header: bytes.Clone(header),
			name:   name,
			prefix: payload[:prefixLen],
			object: payload[prefixLen:],
		})
		off += arHeaderSize + size
		if size%2 == 1 { // member data is padded to an even offset
			off++
		}
	}
	return members, nil
}

// memberName resolves the stored name and reports how many payload bytes hold
// it: BSD keeps long names in a "#1/<len>" field with the bytes inline, GNU
// keeps them in a separate table. Only BSD encoding is produced on macOS, so a
// GN"-style "/<offset>" reference is rejected rather than mis-parsed.
func memberName(field, payload []byte) (name string, prefixLen int, err error) {
	f := strings.TrimRight(string(field), " \x00")
	switch {
	case strings.HasPrefix(f, "#1/"):
		n, err := strconv.Atoi(f[len("#1/"):])
		if err != nil || n < 0 || n > len(payload) {
			return "", 0, fmt.Errorf("bad BSD long name %q", f)
		}
		return strings.TrimRight(string(payload[:n]), "\x00"), n, nil
	case f == "/" || f == "//":
		return f, 0, nil
	case strings.HasPrefix(f, "/"):
		return "", 0, fmt.Errorf("unsupported GNU long-name reference %q", f)
	default:
		return strings.TrimRight(f, "/"), 0, nil
	}
}

func writeArchive(members []arMember) []byte {
	var buf bytes.Buffer
	buf.WriteString(arMagic)
	for _, m := range members {
		header := bytes.Clone(m.header)
		size := len(m.prefix) + len(m.object)
		copy(header[48:58], fmt.Sprintf("%-10d", size))
		buf.Write(header)
		buf.Write(m.prefix)
		buf.Write(m.object)
		if size%2 == 1 {
			buf.WriteByte('\n')
		}
	}
	return buf.Bytes()
}

var errNotMachO = errors.New("not a thin Mach-O object")

// renameObject rewrites the symbol names of one Mach-O object. New names are
// appended to the string table; when the string table is not the last thing in
// the file, an appended copy is used so nothing already referenced by a load
// command moves.
func renameObject(obj []byte, renames map[string]string) ([]byte, int, error) {
	if len(renames) == 0 || len(obj) < 32 {
		return obj, 0, nil
	}
	var order binary.ByteOrder
	var is64 bool
	switch magic := binary.LittleEndian.Uint32(obj[:4]); magic {
	case magic64LE:
		order, is64 = binary.LittleEndian, true
	case magic64BE:
		order, is64 = binary.BigEndian, true
	case magic32LE:
		order, is64 = binary.LittleEndian, false
	case magic32BE:
		order, is64 = binary.BigEndian, false
	case fatMagicLE, fatMagic64LE:
		return nil, 0, errors.New("universal (fat) archive: pass a thin Mach-O archive")
	default:
		return nil, 0, errNotMachO
	}

	headerSize := 28
	nlistSize := nlist32Len
	if is64 {
		headerSize = 32
		nlistSize = nlist64Len
	}
	if len(obj) < headerSize {
		return nil, 0, errors.New("truncated Mach-O header")
	}
	ncmds := int(order.Uint32(obj[16:20]))
	p := headerSize
	symtabOff := -1
	var symoff, nsyms, stroff, strsize uint32
	for i := 0; i < ncmds; i++ {
		if p+8 > len(obj) {
			return nil, 0, errors.New("truncated load commands")
		}
		cmd := order.Uint32(obj[p : p+4])
		cmdsize := int(order.Uint32(obj[p+4 : p+8]))
		if cmdsize < 8 || p+cmdsize > len(obj) {
			return nil, 0, fmt.Errorf("bad load command size %d at offset %d", cmdsize, p)
		}
		if cmd == lcSymtab && cmdsize >= 24 {
			symtabOff = p
			symoff = order.Uint32(obj[p+8 : p+12])
			nsyms = order.Uint32(obj[p+12 : p+16])
			stroff = order.Uint32(obj[p+16 : p+20])
			strsize = order.Uint32(obj[p+20 : p+24])
		}
		p += cmdsize
	}
	if symtabOff < 0 {
		return obj, 0, nil
	}
	if int(stroff)+int(strsize) > len(obj) || int(symoff)+int(nsyms)*nlistSize > len(obj) {
		return nil, 0, errors.New("symbol table outside the object")
	}

	oldStr := obj[stroff : stroff+strsize]
	newStr := bytes.Clone(oldStr)
	newOffsets := make(map[string]uint32)
	// Renamed symbols get an n_strx into the appended part of the table; every
	// other symbol keeps the offset it already had.
	offsets := make([]uint32, nsyms)
	isRenamed := make([]bool, nsyms)
	renamed := 0
	for i := 0; i < int(nsyms); i++ {
		e := int(symoff) + i*nlistSize
		strx := order.Uint32(obj[e : e+4])
		if strx >= strsize {
			continue
		}
		name, _, _ := strings.Cut(string(oldStr[strx:]), "\x00")
		newName, ok := renames[name]
		if !ok {
			continue
		}
		off, seen := newOffsets[newName]
		if !seen {
			off = uint32(len(newStr))
			newStr = append(newStr, newName...)
			newStr = append(newStr, 0)
			newOffsets[newName] = off
		}
		offsets[i] = off
		isRenamed[i] = true
		renamed++
	}
	if renamed == 0 {
		return obj, 0, nil
	}

	var out []byte
	strtabAt := int(stroff)
	if int(stroff)+int(strsize) == len(obj) {
		out = make([]byte, 0, len(obj)+len(newStr))
		out = append(out, obj[:stroff]...)
		out = append(out, newStr...)
	} else {
		// Bytes follow the string table; keep them where they are and point
		// LC_SYMTAB at an appended copy of the table instead.
		out = make([]byte, 0, len(obj)+len(newStr))
		out = append(out, obj...)
		out = append(out, newStr...)
		strtabAt = len(obj)
	}
	for i, off := range offsets {
		if !isRenamed[i] {
			continue
		}
		e := int(symoff) + i*nlistSize
		order.PutUint32(out[e:e+4], off)
	}
	order.PutUint32(out[symtabOff+16:symtabOff+20], uint32(strtabAt))
	order.PutUint32(out[symtabOff+20:symtabOff+24], uint32(len(newStr)))
	return out, renamed, nil
}
