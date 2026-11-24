package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildObject returns a minimal 64-bit MH_OBJECT that defines the given names,
// in order, as external absolute symbols.
func buildObject(names []string) []byte {
	const (
		headerSize = 32
		symtabSize = 24
		nlistSize  = 16
		mhObject   = 1
		nExtAbs    = 0x01 // N_EXT
	)
	strtab := []byte{0}
	strx := make([]uint32, len(names))
	for i, name := range names {
		strx[i] = uint32(len(strtab))
		strtab = append(strtab, name...)
		strtab = append(strtab, 0)
	}
	symoff := headerSize + symtabSize
	stroff := symoff + len(names)*nlistSize

	obj := make([]byte, headerSize, stroff+len(strtab))
	binary.LittleEndian.PutUint32(obj[0:], magic64LE)
	binary.LittleEndian.PutUint32(obj[4:], 0x0100000c) // CPU_TYPE_ARM64
	binary.LittleEndian.PutUint32(obj[12:], mhObject)
	binary.LittleEndian.PutUint32(obj[16:], 1) // ncmds
	binary.LittleEndian.PutUint32(obj[20:], symtabSize)

	symtab := make([]byte, symtabSize)
	binary.LittleEndian.PutUint32(symtab[0:], lcSymtab)
	binary.LittleEndian.PutUint32(symtab[4:], symtabSize)
	binary.LittleEndian.PutUint32(symtab[8:], uint32(symoff))
	binary.LittleEndian.PutUint32(symtab[12:], uint32(len(names)))
	binary.LittleEndian.PutUint32(symtab[16:], uint32(stroff))
	binary.LittleEndian.PutUint32(symtab[20:], uint32(len(strtab)))
	obj = append(obj, symtab...)

	for _, x := range strx {
		entry := make([]byte, nlistSize)
		binary.LittleEndian.PutUint32(entry[0:], x)
		entry[4] = nExtAbs
		obj = append(obj, entry...)
	}
	return append(obj, strtab...)
}

// blankHeader returns a 60-byte ar member header without a name; the size field
// is filled in by writeArchive.
func blankHeader() []byte {
	h := make([]byte, arHeaderSize)
	copy(h[16:28], "0           ") // mtime
	copy(h[28:34], "0     ")       // uid
	copy(h[34:40], "0     ")       // gid
	copy(h[40:48], "100644  ")     // mode
	copy(h[58:60], "`\n")          // header magic
	return h
}

// testMember encodes one archive member the way ar does: names up to 16 bytes
// live in the header, longer ones are stored in front of the payload ("#1/<n>")
// and padded for alignment. The tool keeps those bytes verbatim.
func testMember(name string, object []byte) arMember {
	header := blankHeader()
	if len(name)+1 <= 16 {
		copy(header[0:16], fmt.Sprintf("%-16s", name+"/"))
		return arMember{header: header, name: name, object: object}
	}
	prefixLen := len(name)
	if rem := prefixLen % 8; rem != 4 {
		prefixLen += 4 - rem
		if prefixLen < len(name) {
			prefixLen += 8
		}
	}
	prefix := make([]byte, prefixLen)
	copy(prefix, name)
	copy(header[0:16], fmt.Sprintf("#1/%-13d", prefixLen))
	return arMember{header: header, name: name, prefix: prefix, object: object}
}

// symbolNames lists the names an object's symbol table refers to, following
// n_strx through the string table.
func symbolNames(t *testing.T, obj []byte) []string {
	t.Helper()
	ncmds := int(binary.LittleEndian.Uint32(obj[16:20]))
	p := 32
	for i := 0; i < ncmds; i++ {
		cmd := binary.LittleEndian.Uint32(obj[p : p+4])
		cmdsize := int(binary.LittleEndian.Uint32(obj[p+4 : p+8]))
		if cmd != lcSymtab {
			p += cmdsize
			continue
		}
		symoff := binary.LittleEndian.Uint32(obj[p+8 : p+12])
		nsyms := binary.LittleEndian.Uint32(obj[p+12 : p+16])
		stroff := binary.LittleEndian.Uint32(obj[p+16 : p+20])
		strsize := binary.LittleEndian.Uint32(obj[p+20 : p+24])
		if int(stroff)+int(strsize) > len(obj) {
			t.Fatalf("string table outside object: stroff=%d strsize=%d size=%d", stroff, strsize, len(obj))
		}
		strtab := obj[stroff : stroff+strsize]
		names := make([]string, 0, nsyms)
		for j := 0; j < int(nsyms); j++ {
			e := int(symoff) + j*nlist64Len
			x := binary.LittleEndian.Uint32(obj[e : e+4])
			if x >= strsize {
				t.Fatalf("symbol %d: n_strx %d outside a %d byte string table", j, x, strsize)
			}
			name, _, _ := strings.Cut(string(strtab[x:]), "\x00")
			names = append(names, name)
		}
		return names
	}
	t.Fatal("object has no LC_SYMTAB")
	return nil
}

func memberObject(t *testing.T, archive []byte, name string) []byte {
	t.Helper()
	members, err := parseArchive(archive)
	if err != nil {
		t.Fatalf("parseArchive: %v", err)
	}
	for _, m := range members {
		if m.name == name {
			return m.object
		}
	}
	names := make([]string, len(members))
	for i, m := range members {
		names[i] = m.name
	}
	t.Fatalf("member %q not found in %v", name, names)
	return nil
}

func TestRenameArchive(t *testing.T) {
	longName := "office_oxide-26d4149dda6fc9fd.office_oxide.eb28bbc9b2850596-cgu.0.rcgu.o"
	archive := writeArchive([]arMember{
		testMember("__.SYMDEF SORTED", []byte("stale table of contents")),
		testMember("a.o", buildObject([]string{"_keep", "_office_document_open", "_dup"})),
		testMember(longName, buildObject([]string{"_office_document_open", "_other"})),
	})

	out, renamed, err := renameArchive(archive, map[string]string{
		"_office_document_open": "pdfoxide__office_document_open",
		"_absent":               "pdfoxide__absent",
	})
	if err != nil {
		t.Fatalf("renameArchive: %v", err)
	}
	if renamed != 2 {
		t.Errorf("renamed = %d, want 2 (one per object)", renamed)
	}

	members, err := parseArchive(out)
	if err != nil {
		t.Fatalf("parseArchive(out): %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("members = %d, want 2: the table of contents is rebuilt by ranlib", len(members))
	}

	if got, want := symbolNames(t, memberObject(t, out, "a.o")), []string{"_keep", "pdfoxide__office_document_open", "_dup"}; !equal(got, want) {
		t.Errorf("a.o symbols = %v, want %v", got, want)
	}
	if got, want := symbolNames(t, memberObject(t, out, longName)), []string{"pdfoxide__office_document_open", "_other"}; !equal(got, want) {
		t.Errorf("long-name member symbols = %v, want %v", got, want)
	}
	for _, name := range []string{"_keep", "_dup", "_other"} {
		if !bytes.Contains(out, append([]byte(name), 0)) {
			t.Errorf("string table lost %q", name)
		}
	}
}

func TestRenameObjectWithoutMatchesIsUnchanged(t *testing.T) {
	original := buildObject([]string{"_untouched"})
	obj, renamed, err := renameObject(original, map[string]string{"_missing": "pre__missing"})
	if err != nil {
		t.Fatalf("renameObject: %v", err)
	}
	if renamed != 0 {
		t.Errorf("renamed = %d, want 0", renamed)
	}
	if !bytes.Equal(obj, original) {
		t.Error("object changed although no symbol matched")
	}
}

func TestRenameObjectSharedNameRenamedOnce(t *testing.T) {
	obj := buildObject([]string{"_dup", "_dup", "_keep"})
	out, renamed, err := renameObject(obj, map[string]string{"_dup": "pre__dup"})
	if err != nil {
		t.Fatalf("renameObject: %v", err)
	}
	if renamed != 2 {
		t.Errorf("renamed = %d, want 2", renamed)
	}
	want := []string{"pre__dup", "pre__dup", "_keep"}
	if got := symbolNames(t, out); !equal(got, want) {
		t.Errorf("symbols = %v, want %v", got, want)
	}
}

func TestRenameArchiveRejectsNonArchive(t *testing.T) {
	if _, _, err := renameArchive([]byte("not an archive"), nil); err == nil {
		t.Fatal("expected an error for non-archive input")
	}
}

func TestReadRenameMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rename.map")
	content := "# leading comment\n\n" +
		"__ZN3re23RE2C1Ev ragtokre2___ZN3re23RE2C1Ev\n" +
		"_office_x pdfoxide__office_x\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write map: %v", err)
	}
	renames, err := readRenameMap(path)
	if err != nil {
		t.Fatalf("readRenameMap: %v", err)
	}
	want := map[string]string{
		"__ZN3re23RE2C1Ev": "ragtokre2___ZN3re23RE2C1Ev",
		"_office_x":        "pdfoxide__office_x",
	}
	if len(renames) != len(want) {
		t.Fatalf("renames = %v, want %v", renames, want)
	}
	for k, v := range want {
		if renames[k] != v {
			t.Errorf("renames[%q] = %q, want %q", k, renames[k], v)
		}
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
