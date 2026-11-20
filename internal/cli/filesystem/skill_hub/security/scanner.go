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

package security

import (
	"regexp"
	"strings"
)

// Finding represents a security issue found during scanning
type Finding struct {
	PatternID   string // Rule ID
	Severity    string // critical | high | medium | low
	Category    string // exfiltration | injection | destructive | persistence | network | obfuscation
	File        string // File path where found
	Line        int    // Line number
	Match       string // The matched text
	Description string // Human-readable description
}

// ScanResult represents the result of a security scan
type ScanResult struct {
	SkillName  string
	Source     string
	TrustLevel string // builtin | trusted | community
	Verdict    string // safe | caution | dangerous
	Findings   []Finding
}

// Scanner performs security scans on skill content
type Scanner struct {
	patterns []ThreatPattern
}

// NewScanner creates a new security scanner
func NewScanner() *Scanner {
	return &Scanner{
		patterns: ThreatPatterns,
	}
}

// ScanSkill scans skill files for security threats
func (s *Scanner) ScanSkill(skillName, source, trustLevel string, files map[string][]byte) *ScanResult {
	var allFindings []Finding

	for filename, content := range files {
		findings := s.scanFile(filename, string(content))
		allFindings = append(allFindings, findings...)
	}

	verdict := s.determineVerdict(allFindings)

	return &ScanResult{
		SkillName:  skillName,
		Source:     source,
		TrustLevel: trustLevel,
		Verdict:    verdict,
		Findings:   allFindings,
	}
}

// scanFile scans a single file for threats
func (s *Scanner) scanFile(filename, content string) []Finding {
	var findings []Finding
	lines := logicalLines(content)

	for _, pattern := range s.patterns {
		re, err := regexp.Compile("(?i:" + pattern.Pattern + ")")
		if err != nil {
			continue
		}

		for _, line := range lines {
			if matches := re.FindString(line.text); matches != "" {
				findings = append(findings, Finding{
					PatternID:   pattern.PatternID,
					Severity:    pattern.Severity,
					Category:    pattern.Category,
					File:        filename,
					Line:        line.number,
					Match:       strings.TrimSpace(matches),
					Description: pattern.Description,
				})
			}
		}
	}

	for _, line := range lines {
		if operand, ok := scanQuotedRmExpansion(line.text); ok {
			findings = append(findings, Finding{
				PatternID:   "rm_quoted_expansion",
				Severity:    "high",
				Category:    "destructive",
				File:        filename,
				Line:        line.number,
				Match:       strings.TrimSpace(operand),
				Description: "recursive rm of a quoted operand with a separator before an expansion",
			})
		}
	}

	return findings
}

// logicalLine is one shell command line after backslash continuations are
// folded: number is the physical line the logical line starts on, text no
// longer carries the removed `\<newline>` pairs.
type logicalLine struct {
	number int
	text   string
}

// logicalLines folds `\`-continued physical lines into logical commands so a
// rule sees one command even when its operand is written on the next line. A
// backslash inside single quotes or a comment is literal and does not continue
// the line, matching the shell. A comment ends at its physical newline, so the
// newline after `# ...\` must survive: the next line still runs.
func logicalLines(content string) []logicalLine {
	var lines []logicalLine
	var buf []byte
	number, start := 1, 1
	// Double-quote state matters: an apostrophe is literal text inside a
	// double-quoted span, so it must not open single-quote mode and swallow the
	// backslash-newline that follows.
	inSingle, inDouble, escaped, comment, wordStart := false, false, false, false, true
	// A folded `\<newline>` is removed by the shell, so the word-boundary
	// state from before the backslash still applies: `echo \<newline># c`
	// leaves `#` after a blank, where it opens a comment.
	wordStartBeforeEscape := true

	for i := 0; i < len(content); i++ {
		c := content[i]
		if c == '\n' {
			if escaped && !inSingle && !comment {
				buf = buf[:len(buf)-1]
				escaped = false
				if !inDouble {
					wordStart = wordStartBeforeEscape
				}
				number++
				continue
			}
			lines = append(lines, logicalLine{number: start, text: string(buf)})
			buf = buf[:0]
			number++
			start = number
			escaped, inSingle, inDouble, comment, wordStart = false, false, false, false, true
			continue
		}
		buf = append(buf, c)
		switch {
		case escaped:
			escaped = false
			wordStart = false
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
		case inDouble:
			// A backslash still escapes a following newline, so double quotes
			// keep the continuation; they only end on an unescaped quote.
			switch c {
			case '\\':
				escaped = true
			case '"':
				inDouble = false
			}
		case comment:
			// Comment text runs to the physical newline; nothing in it is code.
		case c == '\\':
			escaped = true
			wordStartBeforeEscape = wordStart
			wordStart = false
		case c == '\'':
			inSingle = true
			wordStart = false
		case c == '"':
			inDouble = true
			wordStart = false
		case c == '#' && wordStart:
			comment = true
		case c == ' ', c == '\t', c == ';', c == '&', c == '|':
			wordStart = true
		default:
			wordStart = false
		}
	}
	lines = append(lines, logicalLine{number: start, text: string(buf)})
	return lines
}

// scanQuotedRmExpansion is a quote-aware pass that stays out of
// ThreatPatterns: a regular expression over the raw line cannot track quote
// and escape state across an operand. It flags a recursive rm operand that
// carries a separator inside a double-quoted span with a later expansion in
// the same operand. The operand can continue after the closing quote with
// more quoted or unquoted fragments, and `\"` does not end the quoted span.
// Return the operand that carries the hidden expansion.
//
// The line is walked once. `;`, `|`, `&` and `(` delimit simple commands, and a
// substitution (`$(`, `<(` or `>(`) is a nested command context whose `)`
// restores the outer command, where the substitution counts as one word: that
// keeps `echo $(printf ok) rm ...` inert while `env IGNORE=<(printf ok) rm ...`
// still reaches the rm. A `)` that matches no substitution closes a subshell,
// a function-body header (`f() { rm ...; }`) or a case pattern
// (`case x) rm ...;;`), and a case arm starts a command of its own even inside
// a substitution.
func scanQuotedRmExpansion(line string) (string, bool) {
	code := shellCodeMask(line)
	var frames []parenFrame
	var chain prefixChain
	inCase := false
	skipWord := false
	escaped := false
	start := 0
	for i := 0; i <= len(line); i++ {
		if i < len(line) {
			if !code[i] {
				escaped = false
				continue
			}
			if escaped {
				escaped = false
				continue
			}
			if line[i] == '\\' {
				escaped = true
				continue
			}
			switch line[i] {
			case ';', '|', '&', '(', ')':
			default:
				continue
			}
		}
		wasSkipping := skipWord
		operand, found, lastWordEnd, nextChain, nextInCase := scanCommandSegment(line, start, i, chain, inCase, skipWord)
		chain, inCase, skipWord = nextChain, nextInCase, false
		if found {
			return operand, true
		}
		if i == len(line) {
			break
		}
		switch line[i] {
		case ';', '|', '&':
			// A case statement keeps its arms across `;;`, so the case state
			// survives a command boundary.
			chain = prefixChain{}
		case '(':
			frames = append(frames, parenFrame{
				substitution: isSubstitutionOpener(line, i),
				chain:        chain,
				inCase:       inCase,
				attached:     (lastWordEnd == i && i > start) || wasSkipping,
			})
			chain = prefixChain{}
			inCase = false
		case ')':
			if inCase {
				// A case arm starts a command of its own and does not close
				// the surrounding substitution.
				chain = prefixChain{}
				break
			}
			if len(frames) == 0 {
				// A function-body header or a case pattern at this level.
				chain = prefixChain{}
				break
			}
			frame := frames[len(frames)-1]
			frames = frames[:len(frames)-1]
			chain = prefixChain{}
			if frame.substitution {
				// The substitution is one word of the outer command: an
				// assignment keeps its name (`IGNORE=<(...)`), and elsewhere
				// it ends the prefix chain (`sudo $(...) rm`).
				chain = frame.chain
				inCase = frame.inCase
				if !frame.attached {
					chain.feed(substitutionWord)
				} else if i+1 < len(line) && !isWordBreak(line[i+1]) {
					skipWord = true
				}
			} else {
				inCase = false
			}
		}
		start = i + 1
	}
	return "", false
}

// substitutionWord stands for a command or process substitution when the outer
// command is evaluated: it is a plain word, never a variable assignment.
const substitutionWord = "x"

// parenFrame is one open parenthesis. A substitution frame restores the outer
// command at its `)`, and every frame keeps the case state of its enclosing
// context.
type parenFrame struct {
	substitution bool
	chain        prefixChain
	inCase       bool
	attached     bool
}

// isWordBreak reports whether c ends the word it follows inside a command.
func isWordBreak(c byte) bool {
	switch c {
	case ' ', '\t', '\r', ';', '|', '&', '(', ')':
		return true
	}
	return false
}

// scanCommandSegment runs the quoted-operand pass over the words of the simple
// command between two boundaries and reports where its last word ended. The
// words are fed to the prefix chain one at a time, so a long line stays linear:
// once a word is not a prefix, no later rm of the same command is reachable
// and `echo rm -rf ...` stays inert, while `sudo -u root rm ...` is inspected.
// skipWord drops the leading fragment of a word a substitution continued.
func scanCommandSegment(line string, start, end int, chain prefixChain, inCase, skipWord bool) (string, bool, int, prefixChain, bool) {
	first := true
	sawCase := false
	lastWordEnd := 0
	for _, word := range shellWordSpans(line, start, end) {
		lastWordEnd = word.end
		if skipWord {
			skipWord = false
			continue
		}
		if !chain.broken {
			if after, ok := rmExecutable(line, word); ok {
				if operand, found := quotedRmOperand(line, after, end); found {
					return operand, true, lastWordEnd, chain, inCase
				}
			}
		}
		switch {
		case first && word.text == "case":
			sawCase = true
		case sawCase && word.text == "in":
			inCase = true
			sawCase = false
		case word.text == "esac":
			inCase = false
		}
		first = false
		chain.feed(word.text)
	}
	return "", false, lastWordEnd, chain, inCase
}

// rmExecutable reports the offset just past the executable rm when word is
// exactly "rm" or a path whose basename is "rm" (`/bin/rm`, `bin/rm`, `./rm`)
// and the command word ends at a blank. A longer basename (`transform`,
// `/usr/bin/tr.rm`) names a different program.
func rmExecutable(line string, word shellWordSpan) (int, bool) {
	switch {
	case word.text == "rm":
	case strings.HasSuffix(word.text, "/rm"):
	default:
		return 0, false
	}
	if word.end < len(line) && line[word.end] != ' ' && line[word.end] != '\t' {
		return 0, false
	}
	return word.end, true
}

// quotedRmOperand returns the first recursive operand after the executable rm
// whose operand hides an expansion behind a separator inside a quoted span.
func quotedRmOperand(line string, after, end int) (string, bool) {
	recursive, endOfOptions := false, false
	var operands []string
	for _, word := range shellWords(line[after:end]) {
		if !endOfOptions && word == "--" {
			endOfOptions = true
			continue
		}
		if !endOfOptions && len(word) > 1 && word[0] == '-' {
			if isRecursiveFlag(word) {
				recursive = true
			}
			continue
		}
		operands = append(operands, word)
	}
	if !recursive {
		return "", false
	}
	for _, operand := range operands {
		if operandHidesExpansion(operand) {
			return operand, true
		}
	}
	return "", false
}

// prefixChain tracks, word by word, whether the words read so far could stand
// ahead of an executable: only variable assignments, shell keywords that start
// a command (`then`, `do`), and command wrappers with the options and values
// they take may (`sudo -u root rm`, `if true; then rm`). The first other word
// makes every later word of the same command an argument, and a quoted word is
// never a prefix, so `"echo" rm` stays a command of echo.
type prefixChain struct {
	wrapper string // wrapper whose option list is still being read
	pending bool   // the last option of wrapper waits for its value
	broken  bool
}

func (c *prefixChain) feed(word string) {
	if c.broken {
		return
	}
	if c.wrapper != "" && !c.takeWrapperWord(word) {
		c.wrapper = ""
	}
	if c.wrapper != "" {
		return
	}
	switch {
	case isAssignment(word), isCommandKeyword(word):
	case isCommandWrapper(word):
		c.wrapper = word
		c.pending = false
	default:
		c.broken = true
	}
}

// takeWrapperWord reports whether the wrapper's option list consumes word.
func (c *prefixChain) takeWrapperWord(word string) bool {
	if c.pending && !strings.HasPrefix(word, "-") {
		c.pending = false
		return true
	}
	if strings.HasPrefix(word, "-") {
		c.pending = wrapperOptionTakesValue(c.wrapper, word)
		return true
	}
	// env takes assignments after its options (`env FOO=1 rm`).
	return c.wrapper == "env" && isAssignment(word)
}

// shellWordSpan is one word of a command segment with its byte offsets.
type shellWordSpan struct {
	text string
	end  int
}

// shellWordSpans splits line[start:end] into words the way shellWords does:
// quoted and escaped bytes stay part of the word, and a `#` at a word start
// opens a comment.
func shellWordSpans(line string, start, end int) []shellWordSpan {
	var spans []shellWordSpan
	for i := start; i < end; {
		switch line[i] {
		case ' ', '\t', '\r':
			i++
			continue
		case '#':
			return spans
		}
		wordStart := i
		for i < end {
			c := line[i]
			if c == ' ' || c == '\t' || c == '\r' {
				break
			}
			switch c {
			case '\'':
				i++
				for i < end && line[i] != '\'' {
					i++
				}
				if i < end {
					i++
				}
			case '"':
				i++
				for i < end {
					if line[i] == '\\' && i+1 < end {
						i += 2
						continue
					}
					if line[i] == '"' {
						i++
						break
					}
					i++
				}
			case '\\':
				i++
				if i < end {
					i++
				}
			default:
				i++
			}
		}
		spans = append(spans, shellWordSpan{text: line[wordStart:i], end: i})
	}
	return spans
}

// isCommandKeyword reports whether word is a shell keyword that ends a
// compound-command header and is directly followed by a command of its own
// (`if ...; then rm`), or the brace that opens a brace group
// (`{ rm ...; }`). Keywords are only accepted ahead of a wrapper chain, so
// `echo then rm` still fails on its first word.
func isCommandKeyword(word string) bool {
	switch word {
	case "if", "then", "elif", "else", "while", "until", "do", "time", "!", "{":
		return true
	}
	return false
}

// isAssignment reports whether word is a leading NAME=VALUE variable
// assignment.
func isAssignment(word string) bool {
	name, _, ok := strings.Cut(word, "=")
	if !ok || name == "" {
		return false
	}
	// A variable name cannot start with a digit.
	if name[0] >= '0' && name[0] <= '9' {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !isShellWordByte(name[i]) {
			return false
		}
	}
	return true
}

// isCommandWrapper reports whether word is a command wrapper that preserves
// the wrapped command's identity.
func isCommandWrapper(word string) bool {
	switch word {
	case "sudo", "exec", "env", "nohup", "nice", "command", "xargs":
		return true
	}
	return false
}

// wrapperOptionTakesValue reports whether option consumes the next word when
// it follows the given wrapper. An attached value (`-uroot`, `--user=root`)
// consumes nothing further.
func wrapperOptionTakesValue(wrapper, option string) bool {
	if strings.HasPrefix(option, "--") {
		if strings.ContainsRune(option, '=') {
			return false
		}
		return takesValue(wrapper, option[2:])
	}
	// An attached short value (`-uroot`) leaves nothing to consume.
	if len(option) > 2 {
		return false
	}
	return takesValue(wrapper, option[1:])
}

func takesValue(wrapper, name string) bool {
	switch wrapper {
	case "sudo":
		switch name {
		case "u", "g", "h", "p", "C", "T", "r", "t", "U", "D", "R",
			"user", "group", "host", "prompt", "close-from", "command-timeout",
			"role", "type", "other-user", "chdir", "chroot":
			return true
		}
	case "env":
		switch name {
		case "u", "C", "S", "unset", "chdir", "split-string":
			return true
		}
	case "nice":
		switch name {
		case "n", "adjustment":
			return true
		}
	case "xargs":
		switch name {
		case "a", "d", "E", "I", "L", "n", "P", "s",
			"arg-file", "delimiter", "eof", "replace", "max-lines",
			"max-args", "max-procs", "max-chars":
			return true
		}
	case "exec":
		return name == "a"
	}
	return false
}

// isSubstitutionOpener reports whether the `(` at pos opens a command
// substitution (`$(`) or a process substitution (`<(` or `>(`).
func isSubstitutionOpener(line string, pos int) bool {
	if pos == 0 {
		return false
	}
	switch line[pos-1] {
	case '$', '<', '>':
		return true
	}
	return false
}

func isShellWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isRecursiveFlag(word string) bool {
	if strings.HasPrefix(word, "--") {
		name := word[2:]
		return name != "" && !strings.ContainsRune(name, '=') && strings.HasPrefix("recursive", name)
	}
	for i := 1; i < len(word); i++ {
		if word[i] == 'r' || word[i] == 'R' {
			return true
		}
	}
	return false
}

// shellCodeMask marks the characters that belong to unquoted code. A command
// name inside a quoted string or after a comment is not a command.
func shellCodeMask(line string) []bool {
	mask := make([]bool, len(line))
	inSingle, inDouble, escaped := false, false, false
	atWordStart := true

	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case escaped:
			escaped = false
			mask[i] = !inSingle && !inDouble
			atWordStart = false
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
		case inDouble:
			switch c {
			case '\\':
				escaped = true
			case '"':
				inDouble = false
			}
		default:
			switch c {
			case '\\':
				escaped = true
				mask[i] = true
				atWordStart = false
			case '\'':
				inSingle = true
				atWordStart = false
			case '"':
				inDouble = true
				atWordStart = false
			case '#':
				if atWordStart {
					return mask
				}
				mask[i] = true
				atWordStart = false
			case ' ', '\t':
				mask[i] = true
				atWordStart = true
			case ';', '&', '|':
				mask[i] = true
				atWordStart = true
			default:
				mask[i] = true
				atWordStart = false
			}
		}
	}
	return mask
}

// shellWords splits one simple command into words. Quoted spans are kept
// whole, a backslash hides the next character outside single quotes, an
// unquoted separator ends the command, and an unquoted `#` at a word start
// opens a comment.
func shellWords(s string) []string {
	var words []string
	var word strings.Builder
	inWord := false
	flush := func() {
		if inWord {
			words = append(words, word.String())
			word.Reset()
			inWord = false
		}
	}

	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case ' ', '\t', '\r':
			flush()
		case ';', '&', '|', '\n':
			flush()
			return words
		case '#':
			if !inWord {
				return words
			}
			inWord = true
			word.WriteByte(c)
		case '\'':
			inWord = true
			word.WriteByte(c)
			j := i + 1
			for j < len(s) && s[j] != '\'' {
				word.WriteByte(s[j])
				j++
			}
			if j < len(s) {
				word.WriteByte(s[j])
			}
			i = j
		case '"':
			inWord = true
			word.WriteByte(c)
			j := i + 1
			for j < len(s) {
				if s[j] == '\\' && j+1 < len(s) {
					word.WriteByte(s[j])
					word.WriteByte(s[j+1])
					j += 2
					continue
				}
				if s[j] == '"' {
					word.WriteByte(s[j])
					j++
					break
				}
				word.WriteByte(s[j])
				j++
			}
			i = j - 1
		case '\\':
			inWord = true
			word.WriteByte(c)
			if i+1 < len(s) {
				i++
				word.WriteByte(s[i])
			}
		default:
			inWord = true
			word.WriteByte(c)
		}
	}
	flush()
	return words
}

// isDoubleQuoteEscapable reports whether a backslash gives the next byte
// special meaning inside double quotes.
func isDoubleQuoteEscapable(c byte) bool {
	switch c {
	case '$', '`', '"', '\\', '\n':
		return true
	}
	return false
}

// operandHidesExpansion reports whether a word holds a separator inside a
// double-quoted span before an expansion anywhere later in the same word. A
// single-quoted or escaped `$` expands nothing.
func operandHidesExpansion(operand string) bool {
	inSingle, inDouble, escaped := false, false, false
	separatorInDouble := false

	for i := 0; i < len(operand); i++ {
		c := operand[i]
		switch {
		case escaped:
			escaped = false
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
		case inDouble:
			switch c {
			case '\\':
				// Inside double quotes a backslash only escapes $, `, ", \
				// and newline; before anything else it stays literal and the
				// next byte keeps its meaning, so `a\;$T` still holds a
				// separator ahead of the expansion.
				if i+1 < len(operand) && isDoubleQuoteEscapable(operand[i+1]) {
					escaped = true
				}
			case '"':
				inDouble = false
			case '$':
				if separatorInDouble {
					return true
				}
			case ';', '&', '|', '#':
				separatorInDouble = true
			}
		default:
			switch c {
			case '\\':
				escaped = true
			case '\'':
				inSingle = true
			case '"':
				inDouble = true
			case '$':
				if separatorInDouble {
					return true
				}
			}
		}
	}
	return false
}

// determineVerdict determines the overall verdict based on findings
func (s *Scanner) determineVerdict(findings []Finding) string {
	if len(findings) == 0 {
		return "safe"
	}

	hasCritical := false
	hasHigh := false

	for _, f := range findings {
		if f.Severity == "critical" {
			hasCritical = true
		} else if f.Severity == "high" {
			hasHigh = true
		}
	}

	if hasCritical {
		return "dangerous"
	}
	if hasHigh {
		return "caution"
	}
	return "caution"
}

// HasCriticalChecks if any finding is critical severity
func (r *ScanResult) HasCritical() bool {
	for _, f := range r.Findings {
		if f.Severity == "critical" {
			return true
		}
	}
	return false
}

// CountBySeverity counts findings by severity level
func (r *ScanResult) CountBySeverity(severity string) int {
	count := 0
	for _, f := range r.Findings {
		if f.Severity == severity {
			count++
		}
	}
	return count
}
