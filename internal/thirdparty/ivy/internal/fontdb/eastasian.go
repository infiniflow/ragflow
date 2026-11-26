package fontdb

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/xo/okage/svg"
)

// Lang is a language of East Asian text. The language selects the forms of
// the Han characters, which differ between Japan, China, Taiwan and Korea.
type Lang uint8

// The East Asian languages.
const (
	// NoLang is a text with no East Asian language, or with a language that
	// the document does not give.
	NoLang Lang = iota
	Japanese
	SimplifiedChinese
	TraditionalChinese
	Korean
)

// String satisfies the fmt.Stringer interface. It returns the tag of BCP 47
// of the language, or the empty string for NoLang.
func (l Lang) String() string {
	switch l {
	case Japanese:
		return "ja"
	case SimplifiedChinese:
		return "zh-Hans"
	case TraditionalChinese:
		return "zh-Hant"
	case Korean:
		return "ko"
	}
	return ""
}

// ParseLang returns the East Asian language of a language tag of BCP 47,
// such as ja-JP, zh-CN, zh-Hant or ko. It returns NoLang for any other
// language. A tag of Chinese without a script or a region is Simplified
// Chinese.
func ParseLang(tag string) Lang {
	parts := strings.FieldsFunc(strings.ToLower(tag), func(r rune) bool { return r == '-' || r == '_' })
	if len(parts) == 0 {
		return NoLang
	}
	switch parts[0] {
	case "ja", "jpn":
		return Japanese
	case "ko", "kor":
		return Korean
	case "yue":
		return TraditionalChinese
	case "zh", "zho", "chi", "cmn":
	default:
		return NoLang
	}
	for _, p := range parts[1:] {
		switch p {
		case "hant", "tw", "hk", "mo":
			return TraditionalChinese
		case "hans", "cn", "sg":
			return SimplifiedChinese
		}
	}
	return SimplifiedChinese
}

// LangOfLCID returns the East Asian language of a Windows language ID of
// MS-LCID, such as 0x0411 for Japanese, which the name table of a font and
// the binary Office formats use. It returns NoLang for any other language.
func LangOfLCID(id uint16) Lang {
	switch id & 0x3ff {
	case 0x11:
		return Japanese
	case 0x12:
		return Korean
	case 0x04:
		// The sublanguage is in the high 6 bits: 1 is Taiwan, 2 is China, 3
		// is Hong Kong, 4 is Singapore, 5 is Macao, and 0x1f is zh-Hant.
		switch id >> 10 {
		case 1, 3, 5, 0x1f:
			return TraditionalChinese
		}
		return SimplifiedChinese
	}
	return NoLang
}

// IsCJK reports whether r is a character of East Asian text: a Han
// character, kana, hangul, bopomofo, or a punctuation mark, a symbol or a
// full-width form of the East Asian blocks.
func IsCJK(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x11ff, // Hangul Jamo
		r >= 0x2e80 && r <= 0x2fdf,   // CJK Radicals, Kangxi Radicals
		r >= 0x2ff0 && r <= 0x9fff,   // Ideographic Description to CJK Unified Ideographs
		r >= 0xa960 && r <= 0xa97f,   // Hangul Jamo Extended-A
		r >= 0xac00 && r <= 0xd7ff,   // Hangul Syllables, Hangul Jamo Extended-B
		r >= 0xf900 && r <= 0xfaff,   // CJK Compatibility Ideographs
		r >= 0xfe30 && r <= 0xfe4f,   // CJK Compatibility Forms
		r >= 0xff00 && r <= 0xffef,   // Halfwidth and Fullwidth Forms
		r >= 0x1b000 && r <= 0x1b16f, // Kana Supplement, Kana Extended-A, Small Kana Extension
		r >= 0x1f200 && r <= 0x1f2ff, // Enclosed Ideographic Supplement
		r >= 0x20000 && r <= 0x323af: // CJK Unified Ideographs Extension B to H
		return true
	}
	return false
}

// LangOfRune returns the language that the script of r implies: Japanese
// for kana, Korean for hangul, and Traditional Chinese for bopomofo. It
// returns NoLang for a Han character and for any other character.
func LangOfRune(r rune) Lang {
	switch {
	case unicode.In(r, unicode.Hiragana, unicode.Katakana):
		return Japanese
	case unicode.Is(unicode.Hangul, r):
		return Korean
	case unicode.Is(unicode.Bopomofo, r):
		return TraditionalChinese
	}
	return NoLang
}

// Kind is the kind of the design of an East Asian font, as the generic
// families serif and sans-serif of CSS divide the Latin fonts.
type Kind uint8

// The kinds of East Asian fonts.
const (
	// NoKind is a font of a kind that ivy does not know.
	NoKind Kind = iota
	// Serif is a font with strokes of varying width and serifs, such as a
	// Mincho, Song, Ming or Batang font.
	Serif
	// Sans is a font with strokes of even width, such as a Gothic, Hei,
	// Gulim or Dotum font.
	Sans
)

// String satisfies the fmt.Stringer interface.
func (k Kind) String() string {
	switch k {
	case Serif:
		return "serif"
	case Sans:
		return "sans-serif"
	}
	return ""
}

// EastAsianFont is a font of Microsoft Office for East Asian text.
type EastAsianFont struct {
	// Family is the family in English, such as MS Mincho.
	Family string
	// Names are the other names of the family, such as the name in its own
	// language, ＭＳ 明朝.
	Names []string
	Kind  Kind
	Lang  Lang
}

// eastAsianFonts are the common fonts of Office for East Asian text. In
// each language and kind, the font that ivy prefers as a substitute comes
// first. The languages come in the order of preference for a Han character
// of a text with no language.
var eastAsianFonts = []EastAsianFont{
	{"Yu Mincho", []string{"游明朝"}, Serif, Japanese},
	{"MS PMincho", []string{"ＭＳ Ｐ明朝"}, Serif, Japanese},
	{"MS Mincho", []string{"ＭＳ 明朝"}, Serif, Japanese},
	{"Yu Gothic", []string{"游ゴシック"}, Sans, Japanese},
	{"Meiryo", []string{"メイリオ"}, Sans, Japanese},
	{"MS PGothic", []string{"ＭＳ Ｐゴシック"}, Sans, Japanese},
	{"MS Gothic", []string{"ＭＳ ゴシック"}, Sans, Japanese},
	{"MS UI Gothic", nil, Sans, Japanese},
	{"SimSun", []string{"宋体"}, Serif, SimplifiedChinese},
	{"NSimSun", []string{"新宋体"}, Serif, SimplifiedChinese},
	{"FangSong", []string{"仿宋", "FangSong_GB2312"}, Serif, SimplifiedChinese},
	{"KaiTi", []string{"楷体", "KaiTi_GB2312"}, Serif, SimplifiedChinese},
	{"Microsoft YaHei", []string{"微软雅黑"}, Sans, SimplifiedChinese},
	{"DengXian", []string{"等线"}, Sans, SimplifiedChinese},
	{"SimHei", []string{"黑体"}, Sans, SimplifiedChinese},
	{"PMingLiU", []string{"新細明體"}, Serif, TraditionalChinese},
	{"MingLiU", []string{"細明體"}, Serif, TraditionalChinese},
	{"Microsoft JhengHei", []string{"微軟正黑體"}, Sans, TraditionalChinese},
	{"Batang", []string{"바탕"}, Serif, Korean},
	{"Malgun Gothic", []string{"맑은 고딕"}, Sans, Korean},
	{"Gulim", []string{"굴림"}, Sans, Korean},
	{"Dotum", []string{"돋움"}, Sans, Korean},
}

// eastAsianIndex maps the key of each name of eastAsianFonts to its index.
var eastAsianIndex = func() map[string]int {
	m := map[string]int{}
	for i, f := range eastAsianFonts {
		m[familyKey(f.Family)] = i
		for _, n := range f.Names {
			m[familyKey(n)] = i
		}
	}
	return m
}()

// styleSuffixes are the keys of the words of a style that can end the name
// of a family, such as Light in DengXian Light, or UI in Yu Gothic UI.
var styleSuffixes = []string{
	"extralight", "semilight", "light", "regular", "medium", "semibold",
	"demibold", "bold", "heavy", "ui",
}

// LookupEastAsian returns the font of Office for East Asian text that name
// names, such as MS Mincho, ＭＳ 明朝, MSMincho or DengXian Light, and
// true. It compares the letters and the digits of the names only, in any
// case and any width. It returns false for any other name.
func LookupEastAsian(name string) (EastAsianFont, bool) {
	i, ok := lookupEastAsian(name)
	if !ok {
		return EastAsianFont{}, false
	}
	return eastAsianFonts[i], true
}

// lookupEastAsian returns the index of the font of eastAsianFonts that name
// names.
func lookupEastAsian(name string) (int, bool) {
	for _, c := range familyCandidates(name) {
		k := familyKey(c)
		for k != "" {
			if i, ok := eastAsianIndex[k]; ok {
				return i, true
			}
			trimmed := false
			for _, s := range styleSuffixes {
				if t, ok := strings.CutSuffix(k, s); ok && t != "" {
					k, trimmed = t, true
					break
				}
			}
			if !trimmed {
				break
			}
		}
	}
	return 0, false
}

// serifWords and sansWords are the words of the names of East Asian fonts
// that give the kind of the font, in lower case.
var (
	serifWords = []string{
		"mincho", "明朝", "ming", "明體", "明体", "song", "宋", "batang", "바탕",
		"myeongjo", "myungjo", "명조", "serif", "kai", "楷", "fangsong", "仿宋",
		"gungsuh", "궁서",
	}
	sansWords = []string{
		"gothic", "ゴシック", "ｺﾞｼｯｸ", "hei", "黑", "黒", "gulim", "굴림", "dotum",
		"돋움", "고딕", "sans", "meiryo", "メイリオ", "dengxian", "等线", "yuan",
		"圆", "丸",
	}
	// cjkWords are the words that mark the name of an East Asian font that
	// has no name in an East Asian script.
	cjkWords = []string{
		"cjk", "mincho", "batang", "gulim", "dotum", "gungsuh", "myeongjo",
		"myungjo", "simsun", "songti", "heiti", "simhei", "yahei", "jhenghei",
		"mingliu", "kaiti", "fangsong",
	}
)

// NameKind returns the kind of an East Asian font from its name: Serif for
// a name with Mincho, Song or Ming, and Sans for a name with Gothic, Hei or
// Gulim. It returns NoKind when the name gives no kind.
func NameKind(name string) Kind {
	if i, ok := lookupEastAsian(name); ok {
		return eastAsianFonts[i].Kind
	}
	lower := strings.ToLower(foldWidth(name))
	for _, w := range sansWords {
		if strings.Contains(lower, w) {
			return Sans
		}
	}
	for _, w := range serifWords {
		if strings.Contains(lower, w) {
			return Serif
		}
	}
	return NoKind
}

// cjkName reports whether name has a character of an East Asian script,
// and returns the language that the script implies.
func cjkName(name string) (Lang, bool) {
	found := false
	for _, r := range name {
		if r >= 0xff01 && r <= 0xff5e {
			// A full-width Latin letter is in the names of Japanese
			// fonts, such as ＭＳ 明朝, but gives no language.
			continue
		}
		if IsCJK(r) {
			found = true
			if l := LangOfRune(r); l != NoLang {
				return l, true
			}
		}
	}
	return NoLang, found
}

// ClassifyName returns the kind and the language of the font that name
// names, and true when name names an East Asian font: a font of
// LookupEastAsian, or a name with a character of an East Asian script. It
// returns false for any other name.
func ClassifyName(name string) (Kind, Lang, bool) {
	if f, ok := LookupEastAsian(name); ok {
		return f.Kind, f.Lang, true
	}
	l, ok := cjkName(name)
	if !ok {
		return NoKind, NoLang, false
	}
	return NameKind(name), l, true
}

// regionWords maps the last word of the name of a free East Asian font, in
// lower case, to its language, such as JP in Noto Sans CJK JP.
var regionWords = map[string]Lang{
	"jp": Japanese, "ja": Japanese, "j": Japanese, "japanese": Japanese,
	"sc": SimplifiedChinese, "cn": SimplifiedChinese, "gb": SimplifiedChinese,
	"tc": TraditionalChinese, "tw": TraditionalChinese, "hk": TraditionalChinese,
	"kr": Korean, "k": Korean, "korean": Korean,
}

// faceClass is the class of an East Asian face of a database.
type faceClass struct {
	family string
	kind   Kind
	lang   Lang
	// rank is the index of the font in eastAsianFonts, or the length of
	// eastAsianFonts for another font.
	rank int
}

// classify returns the class of face, and false when face does not look
// like an East Asian font. It reads the names of the face only, and not its
// data. It looks at, in this order:
//
//  1. The names of the fonts of eastAsianFonts.
//  2. A name in an East Asian script. The language of the name record
//     gives the language of the face, or else the script does.
//  3. A name with a word of cjkWords, such as Noto Sans CJK JP. Its last
//     word gives the language.
func classify(face *svg.FaceInfo) (faceClass, bool) {
	if len(face.Families) == 0 {
		return faceClass{}, false
	}
	c := faceClass{family: face.Families[0].Name, rank: len(eastAsianFonts)}
	for _, f := range face.Families {
		if i, ok := lookupEastAsian(f.Name); ok {
			e := eastAsianFonts[i]
			c.kind, c.lang, c.rank = e.Kind, e.Lang, i
			return c, true
		}
	}
	found := false
	for _, f := range face.Families {
		l, ok := cjkName(f.Name)
		if !ok {
			continue
		}
		found = true
		if ll := LangOfLCID(f.Language); ll != NoLang {
			l = ll
		}
		if c.lang == NoLang {
			c.lang = l
		}
	}
	for _, f := range face.Families {
		lower := strings.ToLower(f.Name)
		if !found && slices.ContainsFunc(cjkWords, func(w string) bool { return strings.Contains(lower, w) }) {
			found = true
		}
		if c.lang == NoLang {
			if words := strings.Fields(lower); len(words) > 1 {
				c.lang = regionWords[words[len(words)-1]]
			}
		}
		if c.kind == NoKind {
			c.kind = NameKind(f.Name)
		}
	}
	return c, found
}

// EastAsian holds the East Asian faces of a database, with their kind and
// their language, for the substitute of a font that the database does not
// hold and for the fallback of an East Asian character.
//
// The fonts of the module github.com/xo/ivy/fonts and the Go fonts hold no
// East Asian face for it, so EastAsian finds faces only among the fonts of
// the system and the other font files of a program.
type EastAsian struct {
	faces []faceClass
}

// NewEastAsian returns the East Asian faces of db, of the faces from lo to
// hi. It reads the names of the faces only.
func NewEastAsian(db *svg.FontDatabase, lo, hi int) *EastAsian {
	e := &EastAsian{}
	seen := map[string]bool{}
	for id := lo; id < hi; id++ {
		face, ok := db.Face(svg.FontID(id))
		if !ok {
			continue
		}
		c, ok := classify(face)
		if !ok || seen[c.family] {
			continue
		}
		seen[c.family] = true
		e.faces = append(e.faces, c)
	}
	return e
}

// Len returns the number of the East Asian families.
func (e *EastAsian) Len() int {
	return len(e.faces)
}

// Families returns the East Asian families, the best for kind and lang
// first. A family of lang comes first, then for Chinese a family of the
// other Chinese, then a family of kind. In each of these, a family of both
// lang and kind comes first. Then come the fonts of Office in the order of
// eastAsianFonts, then the other families in the order of the database. A
// text with no language prefers Japanese, then Simplified Chinese, then
// Traditional Chinese, then Korean.
func (e *EastAsian) Families(kind Kind, lang Lang) []string {
	faces := slices.Clone(e.faces)
	score := func(c faceClass) int {
		s := 0
		switch {
		case lang != NoLang && c.lang == lang:
			s += 4
		case chinese(lang) && chinese(c.lang):
			s += 3
		}
		if kind != NoKind && c.kind == kind {
			s += 2
		}
		return s
	}
	langRank := func(c faceClass) int {
		if c.lang == NoLang {
			return int(Korean) + 1
		}
		return int(c.lang)
	}
	slices.SortStableFunc(faces, func(a, b faceClass) int {
		if d := cmp.Compare(score(b), score(a)); d != 0 {
			return d
		}
		if lang == NoLang {
			if d := cmp.Compare(langRank(a), langRank(b)); d != 0 {
				return d
			}
		}
		return cmp.Compare(a.rank, b.rank)
	})
	out := make([]string, len(faces))
	for i, c := range faces {
		out[i] = c.family
	}
	return out
}

// chinese reports whether l is Simplified or Traditional Chinese.
func chinese(l Lang) bool {
	return l == SimplifiedChinese || l == TraditionalChinese
}

// Probe returns a common character of lang, which a face of the language
// must draw: a hiragana for Japanese, a hangul syllable for Korean, and a
// Han character for the other languages. A font that draws only the rare
// Han characters, such as MingLiU-ExtB, does not draw it.
func Probe(lang Lang) rune {
	switch lang {
	case Japanese:
		return 'あ'
	case Korean:
		return '가'
	}
	return '中'
}

// foldWidth returns s with each full-width Latin letter, digit and sign as
// its ASCII form, and each ideographic space as a space.
func foldWidth(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 0xff01 && r <= 0xff5e:
			return r - 0xff01 + '!'
		case r == 0x3000:
			return ' '
		}
		return r
	}, s)
}
