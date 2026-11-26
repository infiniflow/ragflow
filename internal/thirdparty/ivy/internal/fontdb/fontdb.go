// Package fontdb gives the font database of a program: the fonts that a
// format draws the text with when the document does not embed them.
//
// The database holds, in this order (D23, D24):
//
//  1. The faces of the font files of ivy.Config.Fonts, such as the fonts of
//     the module github.com/xo/ivy/fonts.
//  2. The Go fonts of golang.org/x/image, because okage bundles no font. Go
//     is the serif, the sans-serif, the cursive and the fantasy family, and
//     Go Mono the monospace family.
//  3. The fonts of the system, when ivy.Config.SystemFonts is set.
//
// The layout, the metafiles and the charts use the same database, so the
// text of a chart or a picture looks like the text around it. A format looks
// up the substitute of a font of Office, such as Carlito for Calibri, with
// font.Substitute of internal/font, among the faces of the font files.
//
// Find gives the face of a font that a document names and does not embed,
// such as a font of a PDF file, or its substitute. Covering gives the first
// face that draws a character, such as Droid Sans Fallback of the module
// github.com/xo/ivy/fonts for a CJK character. The layout finds the same
// fallback with the fallback selector of okage, which looks at each face
// of the database.
//
// EastAsian sorts the East Asian faces of a database, such as the Japanese,
// Chinese and Korean fonts of the system, by their kind (serif or sans) and
// their language. Find gives such a face for an East Asian font that the
// database does not hold, such as a Mincho face for MS Mincho. The layout
// does the same. It also draws an East Asian character that no font of the
// text draws with such a face, the language of the text first, before
// Droid Sans Fallback. The fonts module and the Go fonts hold no such face,
// so without the fonts of the system the pages do not change (D23).
//
// The formats share the database of the same font files, when the value of
// the fs.FS is comparable, such as the embed.FS of the module
// github.com/xo/ivy/fonts. They share at most 16 sets. Load reads the files
// of an fs.FS that is not comparable, such as an fstest.MapFS, again on each
// call. The faces read the data of their files on first use, and keep it.
package fontdb

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path"
	"reflect"
	"strings"
	"sync"

	"github.com/xo/okage/svg"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"

	"ragflow/internal/thirdparty/ivy"
)

// maxSets is the number of sets of font files that the formats share. A
// program that gives more distinct sets reads each one again for each
// document.
const maxSets = 16

// Default returns the database of the Go fonts, which a format uses when the
// program gives no font files. The callers share it, and must not change
// it. Clone it to add faces.
var Default = sync.OnceValue(func() *svg.FontDatabase {
	db := svg.NewFontDatabase()
	for _, data := range [][]byte{
		goregular.TTF, gobold.TTF, goitalic.TTF, gobolditalic.TTF,
		gomono.TTF, gomonobold.TTF, gomonoitalic.TTF, gomonobolditalic.TTF,
	} {
		db.LoadFontData(data)
	}
	setGoFamilies(db)
	return db
})

// System returns the database of the Go fonts and the fonts of the system.
// It reads the system once, with LoadOSFonts of okage (D23). The callers
// share it, and must not change it.
var System = sync.OnceValue(func() *svg.FontDatabase {
	db := Default().Clone()
	db.LoadOSFonts()
	return db
})

// setGoFamilies makes Go the serif, the sans-serif, the cursive and the
// fantasy family of db, and Go Mono its monospace family.
func setGoFamilies(db *svg.FontDatabase) {
	db.SetSerifFamily("Go")
	db.SetSansSerifFamily("Go")
	db.SetCursiveFamily("Go")
	db.SetFantasyFamily("Go")
	db.SetMonospaceFamily("Go Mono")
}

// Of returns the database of the fonts of cfg: the faces of cfg.Fonts, then
// the Go fonts, then the fonts of the system when cfg.SystemFonts is set.
func Of(cfg ivy.Config) *svg.FontDatabase {
	db, _ := Load(cfg.Fonts, cfg.SystemFonts, cfg.Limits.WithDefaults(), cfg.Log())
	return db
}

// Load returns the database of the fonts of fsys, and the number of the
// faces at its start that come from fsys. system adds the fonts of the
// system after the Go fonts. Without fsys, it returns Default or System.
// lim must have its defaults.
//
// Load reads at most lim.MaxObjects font files, of at most lim.MaxMemory
// bytes each, in folders at most lim.MaxDepth deep. It logs to log each
// file that it leaves out. The warnings of a shared set go to the log of
// the caller that reads it first.
func Load(fsys fs.FS, system bool, lim ivy.Limits, log *slog.Logger) (*svg.FontDatabase, int) {
	if fsys == nil {
		if system {
			return System(), 0
		}
		return Default(), 0
	}
	set := sharedSet(setKey{fsys: fsys, limits: lim, system: system})
	if set == nil {
		set = &fileSet{}
	}
	set.once.Do(func() {
		set.db, set.first = newFileSet(fsys, system, lim, log)
	})
	return set.db, set.first
}

// fileSet is the database of a set of font files: the faces of the files
// first, then the Go fonts, then the fonts of the system when the program
// asks for them.
type fileSet struct {
	once  sync.Once
	db    *svg.FontDatabase
	first int
	// fsys keeps the files of a set that is shared by the identity of its
	// map, so that the map stays in memory and its address names it.
	fsys fs.FS
}

// setKey is the key of a fileSet: the files, and what changes the faces
// that Load reads from them.
type setKey struct {
	fsys   fs.FS
	limits ivy.Limits
	system bool
}

// sets holds the sets of font files that the formats share, by the value
// of their fs.FS, or by the identity of a map, such as fstest.MapFS, which
// is not comparable. A set of another fs.FS that is not comparable is not
// shared.
var sets = struct {
	sync.Mutex
	m map[setKey]*fileSet
}{m: map[setKey]*fileSet{}}

// mapIdentity names a map by its type and its address, for the key of a
// set whose fs.FS is a map.
type mapIdentity struct {
	typ reflect.Type
	ptr uintptr
}

// Open satisfies fs.FS, so that a mapIdentity can be the fsys of a key. It
// is never called.
func (mapIdentity) Open(string) (fs.File, error) {
	return nil, fs.ErrNotExist
}

// sharedSet returns the shared set of key, or nil when it cannot be shared.
func sharedSet(key setKey) (set *fileSet) {
	var pin fs.FS
	if t := reflect.TypeOf(key.fsys); !t.Comparable() {
		// A document reads the fonts for each chart, so a map of fonts,
		// such as fstest.MapFS, is shared by its identity.
		if t.Kind() != reflect.Map {
			return nil
		}
		pin = key.fsys
		key.fsys = mapIdentity{t, reflect.ValueOf(key.fsys).Pointer()}
	}
	defer func() {
		// A comparable type can hold a value that is not comparable, such
		// as a map in a field of an interface type. The comparison panics,
		// and the set is not shared.
		if recover() != nil {
			set = nil
		}
	}()
	sets.Lock()
	defer sets.Unlock()
	if set, ok := sets.m[key]; ok {
		return set
	}
	if len(sets.m) >= maxSets {
		return nil
	}
	set = &fileSet{fsys: pin}
	sets.m[key] = set
	return set
}

// newFileSet reads the fonts of fsys, and returns the database of the faces
// of fsys followed by the default fonts, and the number of the faces of
// fsys.
func newFileSet(fsys fs.FS, system bool, lim ivy.Limits, log *slog.Logger) (*svg.FontDatabase, int) {
	db := svg.NewFontDatabase()
	loadFiles(db, fsys, lim, log)
	first := db.Len()
	rest := Default()
	if system {
		rest = System()
	}
	for face := range rest.Faces() {
		// The copy shares the bytes of the face.
		db.PushFaceInfo(*face)
	}
	setGoFamilies(db)
	setFileFamilies(db, first)
	return db, first
}

// genericFamilies are the families of the font files that become the
// generic families when the files hold them: the fonts of the module fonts,
// which have the metrics of the fonts of Office, and which LibreOffice and
// calibre also use for these generic families.
var genericFamilies = []struct {
	family string
	set    func(*svg.FontDatabase, string)
}{
	{"Liberation Serif", (*svg.FontDatabase).SetSerifFamily},
	{"Liberation Sans", (*svg.FontDatabase).SetSansSerifFamily},
	{"Liberation Mono", (*svg.FontDatabase).SetMonospaceFamily},
}

// setFileFamilies makes the families of genericFamilies that the first
// faces of db hold, which come from the font files, the generic families.
func setFileFamilies(db *svg.FontDatabase, first int) {
	have := map[string]bool{}
	i := 0
	for face := range db.Faces() {
		if i >= first {
			break
		}
		i++
		for _, f := range face.Families {
			have[f.Name] = true
		}
	}
	for _, g := range genericFamilies {
		if have[g.family] {
			g.set(db, g.family)
		}
	}
}

// loadFiles loads the font files of fsys into db: TrueType, OpenType and
// their collections, and WOFF and WOFF2. The files of a folder load in the
// order of their names, the WOFF and WOFF2 files first. It leaves out each
// file past the limits, and logs it.
func loadFiles(db *svg.FontDatabase, fsys fs.FS, lim ivy.Limits, log *slog.Logger) {
	defer func() {
		// The fonts are input. A font that panics the reader stops the
		// reading, and the database keeps the faces that it read before.
		if r := recover(); r != nil {
			log.Warn("fontdb: font files left out", "err", fmt.Errorf("panic: %v: %w", r, ivy.ErrFormat))
		}
	}()
	db.LoadFontsFS(&files{fsys: fsys, db: db, lim: lim, log: log}, ".")
}

// files is an fs.FS that gives FontDatabase.LoadFontsFS the font files of
// fsys within the limits. Its ReadDir also loads the WOFF and WOFF2 files
// into db, which LoadFontsFS does not read. The faces of the database keep
// it, and read the data of their files with Open on first use.
//
// ReadDir is not safe for concurrent use, and only LoadFontsFS calls it.
// Open is safe for concurrent use.
type files struct {
	fsys fs.FS
	db   *svg.FontDatabase
	lim  ivy.Limits
	log  *slog.Logger
	// n counts the font files that ReadDir gave.
	n int
}

// The interfaces that files implements.
var _ fs.ReadDirFS = (*files)(nil)

// Open satisfies the fs.FS interface. It returns an error that wraps
// ivy.ErrLimit for a file larger than MaxMemory.
func (f *files) Open(name string) (fs.File, error) {
	file, err := f.fsys.Open(name)
	if err != nil {
		return nil, fmt.Errorf("opening font file: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("reading font file %s: %w", name, err)
	}
	if info.Size() > f.lim.MaxMemory {
		_ = file.Close()
		return nil, fmt.Errorf("font file %s of %d bytes: %w", name, info.Size(), ivy.ErrLimit)
	}
	return file, nil
}

// ReadDir satisfies the fs.ReadDirFS interface. It gives the folders within
// MaxDepth, and the TrueType and OpenType files within MaxMemory and
// MaxObjects. It loads the WOFF and WOFF2 files within these limits.
func (f *files) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(f.fsys, name)
	if err != nil {
		return nil, fmt.Errorf("reading font folder: %w", err)
	}
	depth := 1
	if name != "." {
		depth += strings.Count(name, "/") + 1
	}
	var out []fs.DirEntry
	for _, e := range entries {
		p := path.Join(name, e.Name())
		if e.IsDir() {
			if depth < f.lim.MaxDepth {
				out = append(out, e)
			} else {
				f.log.Warn("fontdb: font folder left out", "path", p, "err", ivy.ErrLimit)
			}
			continue
		}
		ext := strings.ToLower(path.Ext(p))
		switch ext {
		case ".ttf", ".ttc", ".otf", ".otc", ".woff", ".woff2":
		default:
			continue
		}
		if err := f.check(e); err != nil {
			f.log.Warn("fontdb: font file left out", "path", p, "err", err)
			continue
		}
		f.n++
		if ext == ".woff" || ext == ".woff2" {
			f.loadWebFont(p)
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// check returns an error when the font file of e is past the limits.
func (f *files) check(e fs.DirEntry) error {
	if f.n >= f.lim.MaxObjects {
		return fmt.Errorf("more than %d font files: %w", f.lim.MaxObjects, ivy.ErrLimit)
	}
	info, err := e.Info()
	if err != nil {
		return fmt.Errorf("reading font file: %w", err)
	}
	if info.Size() > f.lim.MaxMemory {
		return fmt.Errorf("font file of %d bytes: %w", info.Size(), ivy.ErrLimit)
	}
	return nil
}

// loadWebFont loads the WOFF or WOFF2 file p into the database. The
// database converts it to TrueType or OpenType and keeps the data.
func (f *files) loadWebFont(p string) {
	file, err := f.Open(p)
	if err != nil {
		f.log.Warn("fontdb: font file left out", "path", p, "err", err)
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, f.lim.MaxMemory+1))
	switch {
	case err != nil:
	case int64(len(data)) > f.lim.MaxMemory:
		err = fmt.Errorf("font file of more than %d bytes: %w", f.lim.MaxMemory, ivy.ErrLimit)
	case len(f.db.LoadFontData(data)) == 0:
		err = fmt.Errorf("no font face: %w", ivy.ErrFormat)
	}
	if err != nil {
		f.log.Warn("fontdb: font file left out", "path", p, "err", err)
	}
}
