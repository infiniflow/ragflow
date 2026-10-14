package common

import "testing"

// TestRemoveRedundantSpaces mirrors common.string_utils.remove_redundant_spaces.
func TestRemoveRedundantSpaces(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// Both passes run sequentially — pass 1 strips space after `(`,
		// pass 2 strips space before `)`, so both go.
		{"pass1+pass2 on ( world )", "hello ( world )", "hello (world)"},
		// Pass 2 strips space before `!`.
		{"pass2: space before !", "world !", "world!"},
		// Comma is not a boundary in pass 2 (it's in the negated set
		// along with `<` and `(`), so no change.
		{"comma not a boundary", "a , b", "a , b"},
		{"no match", "foo bar", "foo bar"},
		{"empty", "", ""},
		{"digit not a boundary", "abc 123", "abc 123"},
		{"left paren kept (no following space)", "(abc)", "(abc)"},
		// Uppercase letters are word characters too: the Python port compiles
		// with re.IGNORECASE, so "Hello World" must not become "HelloWorld".
		{"uppercase is a word char", "Hello World", "Hello World"},
		{"uppercase initials", "A B", "A B"},
		// Regression: non-Latin letters must not fall into the negated
		// "punctuation" set, otherwise the space between two words is deleted
		// ("привет мир" -> "приветмир").
		{"russian", "привет мир", "привет мир"},
		{"greek", "Γειά σου κόσμε", "Γειά σου κόσμε"},
		{"korean", "안녕 세계", "안녕 세계"},
		{"arabic", "مرحبا بالعالم", "مرحبا بالعالم"},
		{"chinese", "中文 测试 句子", "中文 测试 句子"},
		// Punctuation cleanup still applies to non-Latin text.
		{"accented, space before !", "sécurité des données !", "sécurité des données!"},
		{"space before punctuation is still dropped", "привет !", "привет!"},
		{"full-width parens", "（ 中文 ）", "（中文）"},
		// The non-space class is a literal space, like Python's `[^ ]`: a
		// newline or tab is a boundary character, not whitespace to skip.
		{"newline is a boundary", "hello \nworld", "hello\nworld"},
		{"tab is a boundary", "foo\t bar", "foo\tbar"},
		{"CRLF run collapses", "x\r\n  y", "x\r\ny"},
		// `<` is a left boundary: the space after it goes, the one before stays.
		{"space after < is dropped, before kept", "a < b", "a <b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RemoveRedundantSpaces(tc.in); got != tc.want {
				t.Errorf("RemoveRedundantSpaces(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestCleanMarkdownBlock mirrors common.string_utils.clean_markdown_block.
func TestCleanMarkdownBlock(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "standard markdown block",
			input: "```markdown\nHello world\n```",
			want:  "Hello world",
		},
		{
			name:  "windows CRLF endings",
			input: "```markdown\r\nHello world\r\n```",
			want:  "Hello world",
		},
		{
			name:  "nested code blocks preserved",
			input: "```markdown\nText with ```nested``` blocks\n```",
			want:  "Text with ```nested``` blocks",
		},
		{
			name:  "mixed whitespace tabs",
			input: "\t```markdown\t\n\tContent with tabs\n\t```\t",
			want:  "Content with tabs",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only markdown tags",
			input: "```markdown```",
			want:  "",
		},
		{
			name:  "unwrapped text unchanged",
			input: "Just plain text without markdown block",
			want:  "Just plain text without markdown block",
		},
		{
			name:  "unwrapped python block",
			input: "```python\nprint('hello')\n```",
			want:  "```python\nprint('hello')\n```",
		},
		{
			name:  "unwrapped JSON block",
			input: "```json\n{\"value\": 1}\n```",
			want:  "```json\n{\"value\": 1}\n```",
		},
		{
			name:  "unlabeled code block",
			input: "```\ncode without a language\n```",
			want:  "```\ncode without a language\n```",
		},
		{
			name:  "prose ending with code block",
			input: "Example:\n```python\nprint('hello')\n```",
			want:  "Example:\n```python\nprint('hello')\n```",
		},
		{
			name:  "multiple unwrapped code blocks",
			input: "```python\nfirst()\n```\n\n```python\nsecond()\n```",
			want:  "```python\nfirst()\n```\n\n```python\nsecond()\n```",
		},
		{
			name:  "unwrapped CRLF block",
			input: "  ```python\r\nprint('hello')\r\n```  ",
			want:  "```python\r\nprint('hello')\r\n```",
		},
		{
			name:  "closing fence without markdown opener",
			input: "Unopened block\n```",
			want:  "Unopened block\n```",
		},
		{
			name:  "markdown wrapper around code example",
			input: "```markdown\nExample:\n```python\nprint('hello')\n```\n```",
			want:  "Example:\n```python\nprint('hello')\n```",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CleanMarkdownBlock(tc.input); got != tc.want {
				t.Errorf("CleanMarkdownBlock(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
