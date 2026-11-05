package parser

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"

	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

// stripHTMLHeaderFooter removes <header>/<footer> elements and nodes
// with role="banner"/role="contentinfo" from the HTML byte stream,
// then re-serializes. Mirrors Python rag/flow/parser/utils.py:70-74
// remove_header_footer_html_blob (BeautifulSoup decompose).
func stripHTMLHeaderFooter(data []byte) ([]byte, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	removeHeaderFooterNodes(doc)
	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// removeHeaderFooterNodes walks the parsed tree and detaches every
// <header>/<footer> element and every element whose role attribute
// is "banner" or "contentinfo".
func removeHeaderFooterNodes(n *html.Node) {
	var next *html.Node
	for child := n.FirstChild; child != nil; child = next {
		next = child.NextSibling
		if child.Type == html.ElementNode && isHeaderFooterNode(child) {
			n.RemoveChild(child)
			continue
		}
		removeHeaderFooterNodes(child)
	}
}

func isHeaderFooterNode(n *html.Node) bool {
	if n.Data == "header" || n.Data == "footer" {
		return true
	}
	for _, attr := range n.Attr {
		if attr.Key == "role" && (attr.Val == "banner" || attr.Val == "contentinfo") {
			return true
		}
	}
	return false
}

// flattenHTMLMediaToText normalizes media items (tables and images) to plain
// text items when flatten_media_to_text is enabled. Structured tables are
// rendered into HTML strings (preserving cells and captions), and image and
// table items have their doc_type_kwd rewritten to "text". Mirrors Python
// parser.py and DOCX/spreadsheet flatten_media_to_text.
func flattenHTMLMediaToText(items []map[string]any) []map[string]any {
	for _, item := range items {
		if table, ok := item["table"].(*entity.TableData); ok && table != nil {
			text, _ := item["text"].(string)
			rendered := tableutil.RenderTableHTML(table)
			if text != "" {
				item["text"] = strings.TrimSpace(rendered + "\n" + text)
			} else {
				item["text"] = rendered
			}
			delete(item, "table")
			item["doc_type_kwd"] = "text"
		} else if item["doc_type_kwd"] == "image" || item["doc_type_kwd"] == "table" {
			item["doc_type_kwd"] = "text"
		}
	}
	return items
}

