package wiki

import (
	"bytes"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"golang.org/x/net/html"
)

// summarizeMarkdown extracts the first non-heading body block as plain text,
// falling back to the page title when there is no visible body text.
// Parse before truncating so long link destinations cannot leak into summaries.
func summarizeMarkdown(content, title string) string {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		// Raw HTML stays in an intermediate buffer only; tokenization removes tags.
		goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
	)
	var summary string
	for i, input := range []string{content, title} {
		source := []byte(input)
		doc := md.Parser().Parse(text.NewReader(source))
		_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			if i == 0 && node.Kind() == ast.KindHeading {
				return ast.WalkSkipChildren, nil
			}
			switch node.Kind() {
			case ast.KindHeading, ast.KindParagraph, ast.KindTextBlock, ast.KindCodeBlock,
				ast.KindFencedCodeBlock, ast.KindHTMLBlock, extast.KindTable:
				var rendered bytes.Buffer
				if err := md.Renderer().Render(&rendered, source, node); err != nil {
					return ast.WalkStop, err
				}
				summary = strings.Join(strings.Fields(extractHTMLText(rendered.Bytes())), " ")
				if summary != "" {
					return ast.WalkStop, nil
				}
				return ast.WalkSkipChildren, nil
			}
			return ast.WalkContinue, nil
		})
		if summary != "" {
			break
		}
	}
	if summary == "" {
		// A title consisting only of punctuation is still a literal page name.
		literalTitle := strings.Join(strings.Fields(title), " ")
		for _, r := range literalTitle {
			if !unicode.IsPunct(r) && !unicode.IsSymbol(r) && !unicode.IsSpace(r) {
				literalTitle = ""
				break
			}
		}
		summary = literalTitle
	}
	const maxSummaryBytes = 300
	if len(summary) > maxSummaryBytes {
		end := maxSummaryBytes
		for !utf8.RuneStart(summary[end]) {
			end--
		}
		summary = strings.TrimSpace(summary[:end])
	}
	return summary
}

// extractHTMLText reads rendered text, not link targets or formatting tags.
// Tokenization decodes entities once and preserves escaped HTML in code spans.
func extractHTMLText(source []byte) string {
	tokens := html.NewTokenizer(bytes.NewReader(source))
	var out strings.Builder
	var hiddenTag string
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			return out.String()
		case html.TextToken:
			if hiddenTag == "" {
				out.Write(tokens.Text())
			}
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			token := tokens.Token()
			if token.Data == "script" || token.Data == "style" {
				if token.Type == html.EndTagToken {
					hiddenTag = ""
				} else {
					hiddenTag = token.Data
				}
			}
			if hiddenTag != "" {
				continue
			}
			switch token.Data {
			case "img":
				for _, attr := range token.Attr {
					if attr.Key == "alt" {
						out.WriteString(attr.Val)
						break
					}
				}
			case "br", "hr", "p", "div", "li", "tr", "th", "td", "pre", "blockquote",
				"section", "article", "header", "footer", "main", "aside", "nav",
				"h1", "h2", "h3", "h4", "h5", "h6", "dl", "dt", "dd", "figure", "figcaption":
				out.WriteByte(' ')
			}
		}
	}
}
