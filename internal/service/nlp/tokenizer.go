package nlp

import (
	"bufio"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/siongui/gojianfan"
)

// TokenInfo represents token information (frequency, tag)
type TokenInfo struct {
	Freq int
	Tag  string
}

// RagTokenizer is a tokenizer for RAG (Retrieval-Augmented Generation) tasks.
// It provides methods for tokenizing text with support for Chinese character conversion,
// stemming, lemmatization, and dictionary-based segmentation.
type RagTokenizer struct {
	// Debug enables debug logging
	Debug bool
	// Denominator used in frequency calculations
	Denominator int
	// Dir dictionary file path
	Dir string
	// trie data structure for dictionary lookup (simplified as map for Go implementation)
	trie map[string]TokenInfo
	// rTrie for reverse key lookup
	rTrie map[string]bool
	// splitChar regex pattern for splitting by language
	splitChar *regexp.Regexp
}

// NewRagTokenizer creates a new RagTokenizer instance.
// If userDict is provided and exists, it will be used; otherwise, the default dictionary is loaded.
// debug flag enables debug logging.
func NewRagTokenizer(debug bool, userDict string) (*RagTokenizer, error) {
	rt := &RagTokenizer{
		Debug:       debug,
		Denominator: 1000000,
		trie:        make(map[string]TokenInfo),
		rTrie:       make(map[string]bool),
	}

	// Compile split char regex
	var err error
	rt.splitChar, err = regexp.Compile(`([ ,\.<>/?;:'\[\]\\` + "`" + `!@#$%^&*\(\)\{\}\|_+=《》，。？、；'':'\'\'【】~！￥%……（）——-]+|[a-zA-Z0-9,\.-]+)`)
	if err != nil {
		return nil, fmt.Errorf("failed to compile split regex: %w", err)
	}

	// Determine dictionary path
	dictPath := rt.findDictionary(userDict)
	if dictPath == "" {
		return nil, fmt.Errorf("dictionary not found")
	}
	rt.Dir = dictPath

	if rt.Debug {
		log.Printf("[RagTokenizer] Using dictionary: %s", rt.Dir)
	}

	// Try to load cached trie or build from dictionary
	trieCachePath := rt.Dir + ".trie.cache"
	if _, err := os.Stat(trieCachePath); err == nil {
		// TODO: implement cache loading
		if rt.Debug {
			log.Printf("[RagTokenizer] Loading from cache: %s", trieCachePath)
		}
	}

	// Load dictionary
	if err := rt.loadDict(rt.Dir); err != nil {
		return nil, fmt.Errorf("failed to load dictionary: %w", err)
	}

	return rt, nil
}

// findDictionary finds the dictionary file path
func (rt *RagTokenizer) findDictionary(userDict string) string {
	if userDict != "" {
		if _, err := os.Stat(userDict); err == nil {
			log.Printf("Using user dictionary: %s", userDict)
			return userDict
		}
		log.Printf("User dictionary not found: %s, using default", userDict)
	}

	// Get current file directory
	_, filename, _, _ := runtime.Caller(0)
	currentDir := filepath.Dir(filename)
	resourceDir := "/usr/share/infinity/resource/rag"

	currentDirHuqie := filepath.Join(currentDir, "huqie.txt")
	resourceDirHuqie := filepath.Join(resourceDir, "huqie.txt")

	if _, err := os.Stat(currentDirHuqie); err == nil {
		log.Printf("Using default dictionary: %s", currentDirHuqie)
		return currentDirHuqie
	}
	if _, err := os.Stat(resourceDirHuqie); err == nil {
		log.Printf("Using default dictionary: %s", resourceDirHuqie)
		return resourceDirHuqie
	}

	log.Printf("Dictionary huqie.txt not found in %s and %s", currentDir, resourceDir)
	return ""
}

// key converts a string to a key for trie lookup (lowercase UTF-8 bytes representation)
// This mimics Python's key_: str(line.lower().encode("utf-8"))[2:-1]
func (rt *RagTokenizer) key(line string) string {
	lower := strings.ToLower(line)
	bytes := []byte(lower)
	var builder strings.Builder
	for _, b := range bytes {
		if b >= 32 && b <= 126 && b != '\\' && b != '\'' {
			builder.WriteByte(b)
		} else {
			builder.WriteString(fmt.Sprintf("\\x%02x", b))
		}
	}
	return builder.String()
}

// rkey converts a reversed string to a key for backward trie lookup
// This mimics Python's rkey_: str(("DD" + (line[::-1].lower())).encode("utf-8"))[2:-1]
func (rt *RagTokenizer) rkey(line string) string {
	// Reverse the string
	runes := []rune(line)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	reversed := string(runes)
	lower := strings.ToLower(reversed)
	prefixed := "DD" + lower
	bytes := []byte(prefixed)
	var builder strings.Builder
	for _, b := range bytes {
		if b >= 32 && b <= 126 && b != '\\' && b != '\'' {
			builder.WriteByte(b)
		} else {
			builder.WriteString(fmt.Sprintf("\\x%02x", b))
		}
	}
	return builder.String()
}

// loadDict loads dictionary from file and builds trie
func (rt *RagTokenizer) loadDict(fnm string) error {
	log.Printf("[HUQIE]: Build trie from %s", fnm)

	file, err := os.Open(fnm)
	if err != nil {
		return fmt.Errorf("failed to open dictionary file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		line = regexp.MustCompile(`[\r\n]+`).ReplaceAllString(line, "")
		parts := regexp.MustCompile(`[ \t]`).Split(line, -1)
		if len(parts) < 3 {
			continue
		}

		word := parts[0]
		freqVal := parts[1]
		tag := parts[2]

		k := rt.key(word)
		freqFloat, err := parseFloat(freqVal)
		if err != nil {
			continue
		}
		F := int(math.Log(freqFloat/float64(rt.Denominator)) + 0.5)

		// Update if not exists or has lower frequency
		if existing, exists := rt.trie[k]; !exists || existing.Freq < F {
			rt.trie[k] = TokenInfo{Freq: F, Tag: tag}
		}
		rt.rTrie[rt.rkey(word)] = true
		lineCount++
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading dictionary: %w", err)
	}

	log.Printf("[HUQIE]: Loaded %d entries from dictionary", lineCount)
	return nil
}

// parseFloat safely parses a float from string
func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

// LoadUserDict loads a user dictionary from file (replaces current trie)
func (rt *RagTokenizer) LoadUserDict(fnm string) error {
	// Try to load cache first
	cachePath := fnm + ".trie"
	if _, err := os.Stat(cachePath); err == nil {
		// TODO: implement cache loading
		if rt.Debug {
			log.Printf("Loading user dict from cache: %s", cachePath)
		}
	}

	// Reset and load from file
	rt.trie = make(map[string]TokenInfo)
	rt.rTrie = make(map[string]bool)
	return rt.loadDict(fnm)
}

// AddUserDict adds a user dictionary to the current trie
func (rt *RagTokenizer) AddUserDict(fnm string) error {
	return rt.loadDict(fnm)
}

// StrQ2B converts full-width characters to half-width characters
func (rt *RagTokenizer) StrQ2B(ustring string) string {
	var result strings.Builder
	for _, char := range ustring {
		insideCode := int(char)
		if insideCode == 0x3000 {
			insideCode = 0x0020
		} else {
			insideCode -= 0xFEE0
		}
		if insideCode < 0x0020 || insideCode > 0x7E {
			result.WriteRune(char)
		} else {
			result.WriteRune(rune(insideCode))
		}
	}
	return result.String()
}

// Traditional2Simplified converts traditional Chinese characters to simplified Chinese characters.
// Uses gojianfan library similar to Python's HanziConv.
func (rt *RagTokenizer) Traditional2Simplified(line string) string {
	return gojianfan.T2S(line)
}

// hasKeysWithPrefix checks if any key in trie has the given prefix
func (rt *RagTokenizer) hasKeysWithPrefix(prefix string) bool {
	for k := range rt.trie {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

// TokenResult represents a token with its frequency and tag
type TokenResult struct {
	Token string
	Freq  int
	Tag   string
}

// dfs performs depth-first search for token segmentation (recursive)
func (rt *RagTokenizer) dfs(chars []rune, s int, preTks []TokenResult, tkslist *[][]TokenResult, depth int, memo map[string]int) int {
	const maxDepth = 10
	if depth > maxDepth {
		if s < len(chars) {
			remaining := string(chars[s:])
			copyPretks := append([]TokenResult{}, preTks...)
			copyPretks = append(copyPretks, TokenResult{Token: remaining, Freq: -12, Tag: ""})
			*tkslist = append(*tkslist, copyPretks)
		}
		return s
	}

	stateKey := fmt.Sprintf("%d_%v", s, preTks)
	if val, exists := memo[stateKey]; exists {
		return val
	}

	res := s
	if s >= len(chars) {
		*tkslist = append(*tkslist, preTks)
		memo[stateKey] = s
		return s
	}

	// Check for repetitive characters
	if s < len(chars)-4 {
		isRepetitive := true
		charToCheck := chars[s]
		for i := 1; i < 5; i++ {
			if s+i >= len(chars) || chars[s+i] != charToCheck {
				isRepetitive = false
				break
			}
		}
		if isRepetitive {
			end := s
			for end < len(chars) && chars[end] == charToCheck {
				end++
			}
			mid := s + min(10, end-s)
			t := string(chars[s:mid])
			k := rt.key(t)
			copyPretks := append([]TokenResult{}, preTks...)
			if info, exists := rt.trie[k]; exists {
				copyPretks = append(copyPretks, TokenResult{Token: t, Freq: info.Freq, Tag: info.Tag})
			} else {
				copyPretks = append(copyPretks, TokenResult{Token: t, Freq: -12, Tag: ""})
			}
			nextRes := rt.dfs(chars, mid, copyPretks, tkslist, depth+1, memo)
			if nextRes > res {
				res = nextRes
			}
			memo[stateKey] = res
			return res
		}
	}

	S := s + 1
	if s+2 <= len(chars) {
		t1 := string(chars[s : s+1])
		t2 := string(chars[s : s+2])
		if rt.hasKeysWithPrefix(rt.key(t1)) && !rt.hasKeysWithPrefix(rt.key(t2)) {
			S = s + 2
		}
	}

	if len(preTks) > 2 && len(preTks[len(preTks)-1].Token) == 1 &&
		len(preTks[len(preTks)-2].Token) == 1 && len(preTks[len(preTks)-3].Token) == 1 {
		t1 := preTks[len(preTks)-1].Token + string(chars[s:s+1])
		if rt.hasKeysWithPrefix(rt.key(t1)) {
			S = s + 2
		}
	}

	for e := S; e <= len(chars); e++ {
		t := string(chars[s:e])
		k := rt.key(t)
		if e > s+1 && !rt.hasKeysWithPrefix(k) {
			break
		}
		if info, exists := rt.trie[k]; exists {
			pretks := append([]TokenResult{}, preTks...)
			pretks = append(pretks, TokenResult{Token: t, Freq: info.Freq, Tag: info.Tag})
			nextRes := rt.dfs(chars, e, pretks, tkslist, depth+1, memo)
			if nextRes > res {
				res = nextRes
			}
		}
	}

	if res > s {
		memo[stateKey] = res
		return res
	}

	t := string(chars[s : s+1])
	k := rt.key(t)
	copyPretks := append([]TokenResult{}, preTks...)
	if info, exists := rt.trie[k]; exists {
		copyPretks = append(copyPretks, TokenResult{Token: t, Freq: info.Freq, Tag: info.Tag})
	} else {
		copyPretks = append(copyPretks, TokenResult{Token: t, Freq: -12, Tag: ""})
	}
	result := rt.dfs(chars, s+1, copyPretks, tkslist, depth+1, memo)
	memo[stateKey] = result
	return result
}

// Freq returns the frequency of a token from the dictionary
func (rt *RagTokenizer) Freq(tk string) int {
	k := rt.key(tk)
	if info, exists := rt.trie[k]; exists {
		return int(math.Exp(float64(info.Freq))*float64(rt.Denominator) + 0.5)
	}
	return 0
}

// Tag returns the part-of-speech tag of a token from the dictionary
func (rt *RagTokenizer) Tag(tk string) string {
	k := rt.key(tk)
	if info, exists := rt.trie[k]; exists {
		return info.Tag
	}
	return ""
}

// score calculates a score for a list of token-frequency-tag tuples
func (rt *RagTokenizer) score(tfts []TokenResult) ([]string, float64) {
	const B = 30.0
	F, L := 0.0, 0.0
	tks := make([]string, 0, len(tfts))
	for _, tf := range tfts {
		F += float64(tf.Freq)
		if len(tf.Token) >= 2 {
			L += 1
		}
		tks = append(tks, tf.Token)
	}
	L = L / float64(len(tks))
	if rt.Debug {
		log.Printf("[SC] %v %d %f %f %f", tks, len(tks), L, F, B/float64(len(tks))+L+F)
	}
	return tks, B/float64(len(tks)) + L + F
}

// tokenScore holds tokens and their score
type tokenScore struct {
	tokens []string
	score  float64
}

// sortTokens sorts token lists by score
func (rt *RagTokenizer) sortTokens(tkslist [][]TokenResult) []tokenScore {
	res := make([]tokenScore, 0, len(tkslist))
	for _, tfts := range tkslist {
		tks, s := rt.score(tfts)
		res = append(res, tokenScore{tokens: tks, score: s})
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].score > res[j].score
	})
	return res
}

// merge merges tokens considering split characters
func (rt *RagTokenizer) merge(tks string) string {
	re := regexp.MustCompile(`[ ]+`)
	tksList := re.ReplaceAllString(tks, " ")
	tokens := strings.Split(tksList, " ")
	var res []string
	s := 0
	for s < len(tokens) {
		E := s + 1
		for e := s + 2; e < min(len(tokens)+2, s+6); e++ {
			if e > len(tokens) {
				break
			}
			tk := strings.Join(tokens[s:e], "")
			if rt.splitChar.MatchString(tk) && rt.Freq(tk) > 0 {
				E = e
			}
		}
		res = append(res, strings.Join(tokens[s:E], ""))
		s = E
	}
	return strings.Join(res, " ")
}

// maxForward performs maximum forward matching segmentation
func (rt *RagTokenizer) maxForward(line string) ([]string, float64) {
	var res []TokenResult
	runes := []rune(line)
	s := 0
	for s < len(runes) {
		e := s + 1
		t := string(runes[s:e])
		for e < len(runes) && rt.hasKeysWithPrefix(rt.key(t)) {
			e++
			t = string(runes[s:e])
		}

		for e-1 > s && rt.key(t) != "" {
			if _, exists := rt.trie[rt.key(t)]; exists {
				break
			}
			e--
			t = string(runes[s:e])
		}

		if info, exists := rt.trie[rt.key(t)]; exists {
			res = append(res, TokenResult{Token: t, Freq: info.Freq, Tag: info.Tag})
		} else {
			res = append(res, TokenResult{Token: t, Freq: 0, Tag: ""})
		}

		s = e
	}

	return rt.score(res)
}

// maxBackward performs maximum backward matching segmentation
func (rt *RagTokenizer) maxBackward(line string) ([]string, float64) {
	var res []TokenResult
	runes := []rune(line)
	s := len(runes) - 1
	for s >= 0 {
		e := s + 1
		t := string(runes[s:e])
		for s > 0 && rt.hasKeysWithPrefix(rt.rkey(t)) {
			s--
			t = string(runes[s:e])
		}

		for s+1 < e && rt.key(t) != "" {
			if _, exists := rt.trie[rt.key(t)]; exists {
				break
			}
			s++
			t = string(runes[s:e])
		}

		if info, exists := rt.trie[rt.key(t)]; exists {
			res = append(res, TokenResult{Token: t, Freq: info.Freq, Tag: info.Tag})
		} else {
			res = append(res, TokenResult{Token: t, Freq: 0, Tag: ""})
		}

		s--
	}

	// Reverse result
	for i, j := 0, len(res)-1; i < j; i, j = i+1, j-1 {
		res[i], res[j] = res[j], res[i]
	}

	return rt.score(res)
}

// englishNormalize applies stemming and lemmatization to English tokens
// Simplified version without external stemmer/lemmatizer dependencies
func (rt *RagTokenizer) englishNormalize(tks []string) []string {
	re := regexp.MustCompile(`^[a-zA-Z_-]+$`)
	res := make([]string, len(tks))
	for i, t := range tks {
		if re.MatchString(t) {
			// Simple stemming: remove common suffixes
			res[i] = simpleStem(t)
		} else {
			res[i] = t
		}
	}
	return res
}

// simpleStem performs simple English stemming
func simpleStem(word string) string {
	// Very basic stemming rules
	suffixes := []string{"ing", "ed", "er", "est", "ly", "tion", "ness", "ment"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(word, suffix) && len(word) > len(suffix)+2 {
			return word[:len(word)-len(suffix)]
		}
	}
	return word
}

// LangSegment represents a text segment with language info
type LangSegment struct {
	Text      string
	IsChinese bool
}

// splitByLang splits text into segments by language (Chinese vs non-Chinese)
func (rt *RagTokenizer) splitByLang(line string) []LangSegment {
	var txtLangPairs []LangSegment
	arr := rt.splitChar.Split(line, -1)
	for _, a := range arr {
		if a == "" {
			continue
		}
		runes := []rune(a)
		s := 0
		e := s + 1
		zh := IsChinese(runes[s])
		for e < len(runes) {
			_zh := IsChinese(runes[e])
			if _zh == zh {
				e++
				continue
			}
			txtLangPairs = append(txtLangPairs, LangSegment{Text: string(runes[s:e]), IsChinese: zh})
			s = e
			e = s + 1
			zh = _zh
		}
		if s >= len(runes) {
			continue
		}
		txtLangPairs = append(txtLangPairs, LangSegment{Text: string(runes[s:e]), IsChinese: zh})
	}
	return txtLangPairs
}

// Tokenize tokenizes a line of text into a space-separated string of tokens.
// It performs full-width to half-width conversion, traditional to simplified Chinese conversion,
// language‑aware splitting, and dictionary‑based segmentation.
func (rt *RagTokenizer) Tokenize(line string) string {
	// Replace non-word characters with space
	re := regexp.MustCompile(`\W+`)
	line = re.ReplaceAllString(line, " ")
	line = rt.StrQ2B(line)
	line = strings.ToLower(line)
	line = rt.Traditional2Simplified(line)

	arr := rt.splitByLang(line)
	var res []string
	for _, seg := range arr {
		L := seg.Text
		if !seg.IsChinese {
			// Tokenize English
			tokens := strings.Fields(L)
			normalized := rt.englishNormalize(tokens)
			res = append(res, normalized...)
			continue
		}
		if len(L) < 2 || regexp.MustCompile(`^[a-z\.-]+$`).MatchString(L) || regexp.MustCompile(`^[0-9\.-]+$`).MatchString(L) {
			res = append(res, L)
			continue
		}

		// Use max forward for the first time
		tks, s := rt.maxForward(L)
		tks1, s1 := rt.maxBackward(L)
		if rt.Debug {
			log.Printf("[FW] %v %f", tks, s)
			log.Printf("[BW] %v %f", tks1, s1)
		}

		i, j, _i, _j := 0, 0, 0, 0
		same := 0
		for i+same < len(tks1) && j+same < len(tks) && tks1[i+same] == tks[j+same] {
			same++
		}
		if same > 0 {
			res = append(res, strings.Join(tks[j:j+same], " "))
		}
		_i = i + same
		_j = j + same
		j = _j + 1
		i = _i + 1

		for i < len(tks1) && j < len(tks) {
			tk1 := strings.Join(tks1[_i:i], "")
			tk := strings.Join(tks[_j:j], "")
			if tk1 != tk {
				if len(tk1) > len(tk) {
					j++
				} else {
					i++
				}
				continue
			}

			if tks1[i] != tks[j] {
				i++
				j++
				continue
			}
			// Backward tokens from _i to i are different from forward tokens from _j to j
			var tkslist [][]TokenResult
			rt.dfs([]rune(strings.Join(tks[_j:j], "")), 0, []TokenResult{}, &tkslist, 0, make(map[string]int))
			if len(tkslist) > 0 {
				sorted := rt.sortTokens(tkslist)
				res = append(res, strings.Join(sorted[0].tokens, " "))
			}

			same = 1
			for i+same < len(tks1) && j+same < len(tks) && tks1[i+same] == tks[j+same] {
				same++
			}
			res = append(res, strings.Join(tks[j:j+same], " "))
			_i = i + same
			_j = j + same
			j = _j + 1
			i = _i + 1
		}

		if _i < len(tks1) {
			var tkslist [][]TokenResult
			rt.dfs([]rune(strings.Join(tks[_j:], "")), 0, []TokenResult{}, &tkslist, 0, make(map[string]int))
			if len(tkslist) > 0 {
				sorted := rt.sortTokens(tkslist)
				res = append(res, strings.Join(sorted[0].tokens, " "))
			}
		}
	}

	result := strings.Join(res, " ")
	if rt.Debug {
		log.Printf("[TKS] %s", rt.merge(result))
	}
	return rt.merge(result)
}

// FineGrainedTokenize further splits tokens from Tokenize output for finer granularity.
func (rt *RagTokenizer) FineGrainedTokenize(tks string) string {
	tokens := strings.Fields(tks)
	zhNum := 0
	for _, c := range tokens {
		if c != "" && len([]rune(c)) > 0 {
			runes := []rune(c)
			if IsChinese(runes[0]) {
				zhNum++
			}
		}
	}
	if float64(zhNum) < float64(len(tokens))*0.2 {
		var res []string
		for _, tk := range tokens {
			parts := strings.Split(tk, "/")
			res = append(res, parts...)
		}
		return strings.Join(res, " ")
	}

	var res []string
	for _, tk := range tokens {
		if len(tk) < 3 || regexp.MustCompile(`^[0-9,\.-]+$`).MatchString(tk) {
			res = append(res, tk)
			continue
		}
		var tkslist [][]TokenResult
		if len(tk) > 10 {
			tkslist = append(tkslist, []TokenResult{{Token: tk, Freq: 0, Tag: ""}})
		} else {
			rt.dfs([]rune(tk), 0, []TokenResult{}, &tkslist, 0, make(map[string]int))
		}
		if len(tkslist) < 2 {
			res = append(res, tk)
			continue
		}
		sorted := rt.sortTokens(tkslist)
		stk := sorted[0].tokens
		if len(stk) == len([]rune(tk)) {
			stk = []string{tk}
		} else {
			if regexp.MustCompile(`^[a-z\.-]+$`).MatchString(tk) {
				for _, t := range stk {
					if len(t) < 3 {
						stk = []string{tk}
						break
					}
				}
			}
		}
		res = append(res, strings.Join(stk, " "))
	}

	return strings.Join(rt.englishNormalize(res), " ")
}

// IsChinese checks if a rune is a Chinese character
func IsChinese(ch rune) bool {
	return ch >= '\u4e00' && ch <= '\u9fa5'
}

// IsNumber checks if a rune is a digit (0-9)
func IsNumber(ch rune) bool {
	return ch >= '\u0030' && ch <= '\u0039'
}

// IsAlphabet checks if a rune is an English letter (A-Z, a-z)
func IsAlphabet(ch rune) bool {
	return (ch >= '\u0041' && ch <= '\u005a') || (ch >= '\u0061' && ch <= '\u007a')
}

// NaiveQie naive tokenization for testing
func NaiveQie(txt string) []string {
	var tks []string
	re := regexp.MustCompile(`.*[a-zA-Z]$`)
	for _, t := range strings.Fields(txt) {
		if len(tks) > 0 && re.MatchString(tks[len(tks)-1]) && re.MatchString(t) {
			tks = append(tks, " ")
		}
		tks = append(tks, t)
	}
	return tks
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
