package parser

import (
	"bytes"

	"golang.org/x/net/html"
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
