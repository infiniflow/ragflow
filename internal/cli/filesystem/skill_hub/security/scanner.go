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
	"bytes"
	"path/filepath"
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

// compiledThreatPattern pairs a threat pattern with its compiled regex so a
// scan never recompiles the rules and a malformed pattern fails loudly at
// package initialization instead of being silently skipped.
type compiledThreatPattern struct {
	ThreatPattern
	re *regexp.Regexp
}

// compiledThreatPatterns is built once at package initialization. A rule with
// an invalid regex panics here (regexp.MustCompile) rather than being dropped
// by a silent `continue` at scan time — a bad pattern must break the build,
// not ship as a dead rule.
var compiledThreatPatterns = mustCompileThreatPatterns(ThreatPatterns)

// mustCompileThreatPatterns compiles every threat pattern once, using the same
// case-insensitive (?i:...) wrapping the scanner applies at match time, so a
// pattern that does not compile panics here at package init rather than being
// silently dropped at scan time — the failure mode that left the original
// rm_recursive_dangerous rule dead.
func mustCompileThreatPatterns(patterns []ThreatPattern) []compiledThreatPattern {
	compiled := make([]compiledThreatPattern, 0, len(patterns))
	for _, p := range patterns {
		compiled = append(compiled, compiledThreatPattern{
			ThreatPattern: p,
			re:            regexp.MustCompile("(?i:" + p.Pattern + ")"),
		})
	}
	return compiled
}

// Scanner performs security scans on skill content
type Scanner struct {
	patterns []compiledThreatPattern
}

// NewScanner creates a new security scanner
func NewScanner() *Scanner {
	return &Scanner{
		patterns: compiledThreatPatterns,
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
	// A commented-out example such as "# Example: rm -rf $TARGET" is not a
	// command, but a destructive-rm finding makes the skill uninstallable for
	// community sources, so match against the code-only view of each logical
	// line.
	prose := isProseFile(filename)

	for _, pattern := range s.patterns {
		for _, line := range lines {
			target := line.text
			if !prose {
				target = maskShellComments(target)
			}
			if matches := pattern.re.FindString(target); matches != "" {
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
		if line.data {
			// A here-document body is literal input to a program, not a
			// command the shell runs.
			continue
		}
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

// maskShellComments blanks the text a POSIX shell treats as a comment without
// changing the line length, so match offsets stay valid for the original line.
// A '#' opens a comment only where a word may start and only outside quotes;
// '#' inside a word (a#b), in an expansion ($#) or in a quoted string is data.
func maskShellComments(line string) string {
	var masked []byte
	inComment := false
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		if inComment {
			if masked == nil {
				masked = []byte(line)
			}
			masked[i] = ' '
			continue
		}
		if quote != 0 {
			if c == '\\' && quote == '"' && i+1 < len(line) {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\\':
			if i+1 < len(line) {
				i++
			}
		case '\'', '"':
			quote = c
		case '#':
			if i == 0 || isShellWordBreak(line[i-1]) {
				inComment = true
				if masked == nil {
					masked = []byte(line)
				}
				masked[i] = ' '
			}
		}
	}
	if masked == nil {
		return line
	}
	return string(masked)
}

func isShellWordBreak(c byte) bool {
	switch c {
	case ' ', '\t', ';', '&', '|', '(', ')', '<', '>':
		return true
	}
	return false
}

// Markdown is prose the assistant reads as instructions, so a line such as
// "# ignore previous instructions" is content, not a comment. Only files where
// '#' introduces something the shell would skip are masked.
func isProseFile(filename string) bool {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".md", ".markdown", ".mdx", ".txt", ".rst", ".adoc":
		return true
	}
	return false
}

// logicalLine is one shell command line after backslash continuations are
// folded: number is the physical line the logical line starts on, text no
// longer carries the removed `\<newline>` pairs. data marks a line that sits
// inside a here-document body, which the shell hands to a program as literal
// input instead of running as a command.
type logicalLine struct {
	number int
	text   string
	data   bool
}

// heredocTerm is one pending here-document: the delimiter that ends its body,
// whether `<<-` strips leading tabs, and whether any part of the delimiter word
// was quoted. A quoted delimiter makes the body literal; an unquoted one still
// runs command substitutions.
type heredocTerm struct {
	delimiter string
	stripTabs bool
	quoted    bool
}

// logicalLines folds `\`-continued physical lines into logical commands so a
// rule sees one command even when its operand is written on the next line. A
// backslash inside single quotes or a comment is literal and does not continue
// the line, matching the shell. A comment ends at its physical newline, so the
// newline after `# ...\` must survive: the next line still runs. A quoted
// string that spans physical newlines stays in the same command. A
// here-document body is marked as data and the command line ends at its
// delimiter.
func logicalLines(content string) []logicalLine {
	var lines []logicalLine
	number := 1
	// parenDepth keeps a command that spans physical lines inside `$(...)`, a
	// subshell or a `(pattern)` one logical line. It survives a comment line so
	// the command after `# ...` still knows its parentheses are open.
	parenDepth := 0
	for pos := 0; pos < len(content); {
		start := number
		var buf []byte
		var heredocs []heredocTerm
		// Double-quote state matters: an apostrophe is literal text inside a
		// double-quoted span, so it must not open single-quote mode and swallow
		// the backslash-newline that follows.
		inSingle, inDouble, escaped, comment, wordStart := false, false, false, false, true
		// A folded `\<newline>` is removed by the shell, so the word-boundary
		// state from before the backslash still applies: `echo \<newline># c`
		// leaves `#` after a blank, where it opens a comment.
		wordStartBeforeEscape := true

		for pos < len(content) {
			c := content[pos]
			if c == '\n' {
				if escaped && !inSingle && !comment {
					buf = buf[:len(buf)-1]
					escaped = false
					if !inDouble {
						wordStart = wordStartBeforeEscape
					}
					number++
					pos++
					continue
				}
				if inSingle || inDouble {
					// The shell keeps the newline inside the quoted word and
					// keeps running the same command, so the scan must too.
					buf = append(buf, c)
					number++
					pos++
					continue
				}
				if comment {
					// A comment ends at the physical newline. End the logical
					// line here so the comment does not mask the next line;
					// the open parentheses still carry over.
					break
				}
				if parenDepth > 0 || continuesShellCommand(buf) {
					// The shell reads the next physical line as part of this
					// command; the newline is plain whitespace there.
					buf = append(buf, ' ')
					wordStart = true
					number++
					pos++
					continue
				}
				break
			}
			if !inSingle && !inDouble && !comment && !escaped && c == '<' && pos+1 < len(content) && content[pos+1] == '<' {
				if term, ok := parseHeredocDelimiter(content, pos); ok {
					heredocs = append(heredocs, term)
				}
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
			case c == '(':
				parenDepth++
				wordStart = true
			case c == ')':
				if parenDepth > 0 {
					parenDepth--
				}
				wordStart = true
			case c == ' ', c == '\t', c == ';', c == '&', c == '|':
				wordStart = true
			default:
				wordStart = false
			}
			pos++
		}
		lines = append(lines, logicalLine{number: start, text: string(buf)})
		if pos >= len(content) {
			break
		}
		pos++ // the newline that ended the command
		number++
		for _, term := range heredocs {
			pos, number = appendHeredocBody(content, pos, number, term, &lines)
		}
	}
	return lines
}

// continuesShellCommand reports whether the line ends with an operator that
// makes the shell read the next physical line as the same command.
func continuesShellCommand(buf []byte) bool {
	trimmed := bytes.TrimRight(buf, " \t\r")
	if len(trimmed) == 0 {
		return false
	}
	if trimmed[len(trimmed)-1] == '|' {
		return true
	}
	return len(trimmed) >= 2 && trimmed[len(trimmed)-2] == '&' && trimmed[len(trimmed)-1] == '&'
}

// appendHeredocBody consumes the physical lines of one here-document body. A
// quoted delimiter hands the body to the program literally, so the lines are
// only marked as data. An unquoted delimiter still runs command substitutions
// while the body expands, so the text inside `$(...)` and backticks is also
// returned as code.
func appendHeredocBody(content string, pos, number int, term heredocTerm, lines *[]logicalLine) (int, int) {
	for pos < len(content) {
		text, next, hasEOL := readPhysicalLine(content, pos)
		match, nextPos, matchEOL, used := text, next, hasEOL, 1
		// An unquoted delimiter has its `\`-newline pairs removed before the
		// shell compares it, so the terminator may be split over two lines.
		for !term.quoted && matchEOL && strings.HasSuffix(match, "\\") {
			match = match[:len(match)-1]
			continued, after, afterEOL := readPhysicalLine(content, nextPos)
			match += continued
			nextPos, matchEOL = after, afterEOL
			used++
		}
		if term.stripTabs {
			match = strings.TrimLeft(match, "\t")
		}
		if match == term.delimiter {
			// The delimiter line closes the body and is not a command.
			pos = nextPos
			number += used
			if !matchEOL {
				number--
			}
			return pos, number
		}
		if !term.quoted {
			for _, sub := range heredocSubstitutions(match) {
				*lines = append(*lines, logicalLine{number: number, text: sub})
			}
		}
		*lines = append(*lines, logicalLine{number: number, text: text, data: true})
		pos = nextPos
		number += used
		if !matchEOL {
			number--
			return pos, number
		}
	}
	return pos, number
}

// readPhysicalLine returns one physical line, the offset just past its newline,
// and whether a newline ended it.
func readPhysicalLine(content string, pos int) (string, int, bool) {
	eol := strings.IndexByte(content[pos:], '\n')
	if eol < 0 {
		return content[pos:], len(content), false
	}
	return content[pos : pos+eol], pos + eol + 1, true
}

// heredocSubstitutions returns the text of every command substitution an
// unquoted here-document body would run: `$(...)`, backticks and nested
// substitutions. An escaped `$(` or backtick is literal text.
func heredocSubstitutions(s string) []string {
	var subs []string
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '`':
			j := i + 1
			for j < len(s) && s[j] != '`' {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			end := min(j, len(s))
			subs = append(subs, s[i+1:end])
			i = end
		case '$':
			if i+1 >= len(s) || s[i+1] != '(' {
				continue
			}
			depth, j := 1, i+2
			for j < len(s) && depth > 0 {
				switch s[j] {
				case '\\':
					j++
				case '(':
					depth++
				case ')':
					depth--
				}
				j++
			}
			end := j
			if depth == 0 {
				end = j - 1
			}
			if end > len(s) {
				end = len(s)
			}
			subs = append(subs, s[i+2:end])
			i = j - 1
		}
	}
	return subs
}

// parseHeredocDelimiter reads the `<<` or `<<-` introducer at pos and returns
// its delimiter word with the shell quoting removed. A here-string (`<<<`) or
// an introducer without a word is not a here-document. Any quoting in the word
// makes the body literal.
func parseHeredocDelimiter(content string, pos int) (heredocTerm, bool) {
	if pos > 0 && content[pos-1] == '<' {
		// The second or third `<` of a here-string introducer.
		return heredocTerm{}, false
	}
	j := pos + 2
	if j < len(content) && content[j] == '<' {
		return heredocTerm{}, false
	}
	term := heredocTerm{}
	if j < len(content) && content[j] == '-' {
		term.stripTabs = true
		j++
	}
	for j < len(content) && (content[j] == ' ' || content[j] == '\t') {
		j++
	}
	var word []byte
	quote := byte(0)
	consumed := false
scan:
	for j < len(content) {
		c := content[j]
		if quote != 0 {
			switch {
			case c == quote:
				quote = 0
			case c == '\\' && quote == '"' && j+1 < len(content):
				j++
				word = append(word, content[j])
			default:
				word = append(word, c)
			}
			j++
			continue
		}
		switch c {
		case '\'', '"':
			// A quoted word is still a delimiter; quote removal may leave it
			// empty (`<<''` ends the body at the first blank line).
			consumed = true
			term.quoted = true
			quote = c
			j++
		case '\\':
			consumed = true
			term.quoted = true
			if j+1 < len(content) {
				j++
				word = append(word, content[j])
			}
			j++
		case ' ', '\t', '\n', ';', '&', '|', '<', '>', '(', ')':
			break scan
		default:
			consumed = true
			word = append(word, c)
			j++
		}
	}
	if !consumed {
		return heredocTerm{}, false
	}
	term.delimiter = string(word)
	return term, true
}

// maxSplitDepth bounds env -S recursion: every level strips at least its own
// `-S`, so a hostile line would otherwise grow the stack with its length.
const maxSplitDepth = 8

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
	return scanQuotedRmExpansionDepth(line, 0)
}

func scanQuotedRmExpansionDepth(line string, depth int) (string, bool) {
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
			// A substitution that is a redirection target or a process
			// substitution does not end the simple command: scan its body as a
			// command of its own, then keep the outer command whole so operands
			// written after it still belong to the rm.
			if line[i] == '(' && isSubstitutionOpener(line, i) && substitutionFollowsRedirection(line, start, i) {
				if close, ok := matchingParen(line, i); ok {
					if operand, found := scanQuotedRmExpansionDepth(line[i+1:close], depth); found {
						return operand, true
					}
					i = close
					continue
				}
			}
			switch line[i] {
			case ';', '|', '&', '(', ')':
			default:
				continue
			}
		}
		wasSkipping := skipWord
		operand, found, lastWordEnd, nextChain, nextInCase := scanCommandSegment(line, start, i, chain, inCase, skipWord, depth)
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
				// the surrounding substitution. A `(` of the optional
				// `(pattern)` form is dropped with its delimiter.
				if n := len(frames); n > 0 && !frames[n-1].substitution {
					frames = frames[:n-1]
				}
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
				// A subshell or a `(pattern)` closes back into the case state
				// of the enclosing context.
				inCase = frame.inCase
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
// depth bounds the env -S rescan of the command string it splices in.
func scanCommandSegment(line string, start, end int, chain prefixChain, inCase, skipWord bool, depth int) (string, bool, int, prefixChain, bool) {
	first := true
	sawCase := false
	lastWordEnd := 0
	for _, word := range shellWordSpans(line, start, end) {
		lastWordEnd = word.end
		if skipWord {
			skipWord = false
			continue
		}
		if value, ok := chain.takeSplitString(word.text); ok {
			// env -S splits its value into argv words: quotes group and a
			// backslash escapes, but a separator or `$` stays literal. Inspect
			// the argv env would execute, then the shell words after it.
			split, valid := envSplitWords(value)
			chain.broken = true
			chain.wrapper = ""
			if !valid {
				// env rejects the split string before running anything.
				continue
			}
			words := make([]cmdWord, 0, len(split)+4)
			for _, v := range split {
				words = append(words, cmdWord{text: v, literal: true})
			}
			for _, span := range shellWordSpans(line, word.end, end) {
				words = append(words, cmdWord{text: span.text})
			}
			if depth < maxSplitDepth {
				if operand, found := scanArgvForQuotedRm(words, depth+1); found {
					return operand, true, lastWordEnd, chain, inCase
				}
			}
			continue
		}
		if !chain.broken && !chain.wrapperConsumes(word.text) {
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
		case first && word.text == "esac":
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

// isRedirection reports whether word is a redirection operator (`>`, `>>`,
// `2>`, `<&`, `&>`): an optional file descriptor followed by `<` and `>` only.
// The word after the operator is its target, not an operand of the command.
func isRedirection(word string) bool {
	i := 0
	for i < len(word) && word[i] >= '0' && word[i] <= '9' {
		i++
	}
	if i == len(word) {
		return false
	}
	for ; i < len(word); i++ {
		if word[i] != '<' && word[i] != '>' && word[i] != '&' {
			return false
		}
	}
	return true
}

// quotedRmOperand returns the first recursive operand after the executable rm
// whose operand hides an expansion behind a separator inside a quoted span.
func quotedRmOperand(line string, after, end int) (string, bool) {
	recursive, endOfOptions, target := false, false, false
	var operands []string
	for _, word := range shellWords(line[after:end]) {
		if target {
			target = false
			continue
		}
		if isRedirection(word) {
			target = true
			continue
		}
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
	split   bool   // the pending value is env's split command string
	lookup  bool   // command -v/-V lists the name instead of running it
	broken  bool
}

func (c *prefixChain) feed(word string) {
	if c.broken {
		return
	}
	if c.wrapper != "" && !c.takeWrapperWord(word) {
		c.wrapper = ""
		c.split = false
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

// wrapperConsumes reports whether the wrapper's option list would consume
// word instead of letting it stand as the executable (`env -u rm`).
func (c *prefixChain) wrapperConsumes(word string) bool {
	if c.wrapper == "" {
		return false
	}
	if c.lookup {
		// `command -v rm ...` describes the name; nothing after it runs.
		return true
	}
	if c.pending {
		// A required value is taken as written even when it looks like an
		// option: `xargs -I -n rm` uses -n as the replacement string.
		return true
	}
	if strings.HasPrefix(word, "-") {
		return true
	}
	// env takes assignments after its options (`env FOO=1 rm`).
	return c.wrapper == "env" && isAssignment(word)
}

// takeWrapperWord reports whether the wrapper's option list consumes word.
func (c *prefixChain) takeWrapperWord(word string) bool {
	if !c.wrapperConsumes(word) {
		return false
	}
	if c.pending {
		c.pending = false
		return true
	}
	if strings.HasPrefix(word, "-") {
		c.pending = wrapperOptionTakesValue(c.wrapper, word)
		c.split = c.pending && isSplitStringOption(c.wrapper, word)
		if c.wrapper == "command" && !strings.HasPrefix(word, "--") && strings.ContainsAny(word[1:], "vV") {
			// `command -v` and `command -V` (also combined as -pv) look the
			// name up instead of executing it; `command -p` still runs it.
			c.lookup = true
		}
		return true
	}
	return true
}

// takeSplitString reports the command line env would run for word. env -S and
// --split-string splice their value into the command, so the value starts the
// command instead of hiding behind a metadata option.
func (c *prefixChain) takeSplitString(word string) (string, bool) {
	if c.broken {
		return "", false
	}
	if c.split {
		c.pending, c.split = false, false
		return unquoteShellWord(word), true
	}
	if c.wrapper != "env" || c.pending {
		return "", false
	}
	if strings.HasPrefix(word, "--split-string=") {
		return unquoteShellWord(word[len("--split-string="):]), true
	}
	if strings.HasPrefix(word, "-S") && len(word) > 2 {
		return unquoteShellWord(word[2:]), true
	}
	return "", false
}

// isSplitStringOption reports whether option makes env splice its value into
// the command it runs.
func isSplitStringOption(wrapper, option string) bool {
	return wrapper == "env" && (option == "-S" || option == "--split-string")
}

// cmdWord is one argument of the command env -S builds. literal marks a word
// that comes from the split string: env passes it through untouched, so a
// separator or `$` inside it is text, not a shell expansion.
type cmdWord struct {
	text    string
	literal bool
}

// scanArgvForQuotedRm inspects the argv env would execute. The words arrive as
// arguments with no shell separators, so only a word that still belongs to the
// shell line can carry an expansion; depth bounds nested split strings.
func scanArgvForQuotedRm(words []cmdWord, depth int) (string, bool) {
	var chain prefixChain
	for i, word := range words {
		if chain.broken || chain.lookup {
			return "", false
		}
		if value, ok := chain.takeSplitString(word.text); ok {
			split, valid := envSplitWords(value)
			if !valid {
				return "", false
			}
			next := make([]cmdWord, 0, len(split)+len(words)-i)
			for _, v := range split {
				next = append(next, cmdWord{text: v, literal: true})
			}
			next = append(next, words[i+1:]...)
			if depth < maxSplitDepth {
				return scanArgvForQuotedRm(next, depth+1)
			}
			return "", false
		}
		if !chain.wrapperConsumes(word.text) && isRmArgvWord(word.text) {
			return quotedRmArgvOperands(words[i+1:])
		}
		chain.feed(word.text)
	}
	return "", false
}

// isRmArgvWord reports whether an argv word names the rm executable. An argv
// word is whole, so any basename rm counts.
func isRmArgvWord(word string) bool {
	return word == "rm" || strings.HasSuffix(word, "/rm")
}

// quotedRmArgvOperands returns the first recursive rm operand that hides an
// expansion. Only shell words count: a split-string operand carries literal
// text, so a `$` in it never expands.
func quotedRmArgvOperands(words []cmdWord) (string, bool) {
	recursive, endOfOptions := false, false
	var operands []cmdWord
	for i := 0; i < len(words); i++ {
		word := words[i]
		if !endOfOptions && word.text == "--" {
			endOfOptions = true
			continue
		}
		if !endOfOptions && len(word.text) > 1 && word.text[0] == '-' {
			if isRecursiveFlag(word.text) {
				recursive = true
			}
			continue
		}
		if !word.literal && isRedirection(word.text) && i+1 < len(words) {
			i++ // the redirection target is not an operand
			continue
		}
		operands = append(operands, word)
	}
	if !recursive {
		return "", false
	}
	for _, operand := range operands {
		if !operand.literal && operandHidesExpansion(operand.text) {
			return operand.text, true
		}
	}
	return "", false
}

// envSplitWords splits the value of env -S the way coreutils does: whitespace
// separates arguments, single and double quotes group, backslash sequences are
// processed, and a `#` as the first character of an argument ignores the rest.
// It reports false when env itself rejects the string: a `$` must begin a
// `${NAME}` expansion, and env exits before running any command otherwise.
// Every returned word is literal: env performs no shell separator or
// substitution handling on the value.
func envSplitWords(s string) ([]string, bool) {
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
		if !inWord && s[i] == '#' {
			break
		}
		switch c := s[i]; c {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			flush()
		case '\'':
			inWord = true
			for i++; i < len(s) && s[i] != '\''; i++ {
				if s[i] == '\\' && i+1 < len(s) && (s[i+1] == '\'' || s[i+1] == '\\') {
					i++
				}
				word.WriteByte(s[i])
			}
		case '"':
			inWord = true
			for i++; i < len(s); i++ {
				if s[i] == '\\' && i+1 < len(s) {
					switch s[i+1] {
					case '"', '\\', '$', '#', '`':
						i++
						word.WriteByte(s[i])
						continue
					case '_':
						i++
						word.WriteByte(' ')
						continue
					}
				}
				if s[i] == '"' {
					break
				}
				word.WriteByte(s[i])
			}
		case '\\':
			inWord = true
			if i+1 >= len(s) {
				continue
			}
			i++
			switch s[i] {
			case 'c':
				// \c ignores the rest of the string.
				flush()
				return words, true
			case 'f':
				word.WriteByte('\f')
			case 'n':
				word.WriteByte('\n')
			case 'r':
				word.WriteByte('\r')
			case 't':
				word.WriteByte('\t')
			case 'v':
				word.WriteByte('\v')
			case '_':
				// Outside quotes \_ separates arguments.
				flush()
			default:
				word.WriteByte(s[i])
			}
		case '$':
			if i+1 >= len(s) || s[i+1] != '{' {
				return nil, false
			}
			end := strings.IndexByte(s[i+2:], '}')
			if end < 0 {
				return nil, false
			}
			end += i + 2
			// env expands ${NAME}; the runtime value is unknown, so the pattern
			// stays as literal text.
			word.WriteString(s[i : end+1])
			inWord = true
			i = end
		default:
			inWord = true
			word.WriteByte(c)
		}
	}
	flush()
	return words, true
}

// unquoteShellWord removes the quoting the shell applies to one word, leaving
// the text the wrapped program receives.
func unquoteShellWord(word string) string {
	var b strings.Builder
	for i := 0; i < len(word); i++ {
		switch word[i] {
		case '\'':
			for i++; i < len(word) && word[i] != '\''; i++ {
				b.WriteByte(word[i])
			}
		case '"':
			for i++; i < len(word); i++ {
				if word[i] == '\\' && i+1 < len(word) {
					switch word[i+1] {
					case '"', '\\', '$', '`':
						i++
						b.WriteByte(word[i])
						continue
					}
				}
				if word[i] == '"' {
					break
				}
				b.WriteByte(word[i])
			}
		case '\\':
			if i+1 < len(word) {
				i++
				b.WriteByte(word[i])
			}
		default:
			b.WriteByte(word[i])
		}
	}
	return b.String()
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

// substitutionFollowsRedirection reports whether the substitution opening at
// pos is the target of a redirection (`>$(cmd)`, `2>$(cmd)`) or a process
// substitution (`<(cmd)`). Neither ends the simple command, so an rm operand
// written after it still belongs to the rm. A command-position substitution
// (`$(cmd) rm ...`) or one inside an assignment does not qualify.
func substitutionFollowsRedirection(line string, start, pos int) bool {
	spans := shellWordSpans(line, start, pos)
	if len(spans) == 0 {
		return false
	}
	last := strings.TrimSuffix(spans[len(spans)-1].text, "$")
	return isRedirection(last)
}

// matchingParen returns the offset of the `)` that closes the `(` at pos,
// skipping quoted and escaped characters.
func matchingParen(line string, pos int) (int, bool) {
	depth, escaped := 0, false
	inSingle, inDouble := false, false
	for i := pos; i < len(line); i++ {
		c := line[i]
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
				escaped = true
			case '"':
				inDouble = false
			}
		default:
			switch c {
			case '\\':
				escaped = true
			case '\'':
				inSingle = true
			case '"':
				inDouble = true
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					return i, true
				}
			}
		}
	}
	return 0, false
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
				if separatorInDouble && expansionStartsAt(operand, i) {
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
				if separatorInDouble && expansionStartsAt(operand, i) {
					return true
				}
			}
		}
	}
	return false
}

// expansionStartsAt reports whether the `$` at i opens a shell expansion. A
// bare dollar sign at the end of a word or before a quote is literal text.
func expansionStartsAt(s string, i int) bool {
	if i+1 >= len(s) {
		return false
	}
	switch c := s[i+1]; c {
	case '{', '(', '$', '#', '?', '!', '@', '*', '-':
		return true
	default:
		return isShellWordByte(c)
	}
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
