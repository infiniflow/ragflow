package parser

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

func parseDoclingJSONItems(document map[string]any) ([]map[string]any, bool) {
	if schema, _ := document["schema_name"].(string); schema != "DoclingDocument" {
		return nil, false
	}
	body, ok := document["body"].(map[string]any)
	if !ok {
		return nil, false
	}

	heights := doclingPageHeights(document)
	children, _ := body["children"].([]any)
	// Walk the body tree in reading order. The arrays store referenced nodes.
	pending := slices.Clone(children)
	slices.Reverse(pending)
	visited := make(map[string]bool)
	items := make([]map[string]any, 0)
	for len(pending) > 0 {
		last := len(pending) - 1
		child, _ := pending[last].(map[string]any)
		pending = pending[:last]
		ref, _ := child["$ref"].(string)
		if visited[ref] {
			continue
		}
		node, collection := doclingJSONNode(document, ref)
		if node == nil {
			continue
		}
		visited[ref] = true
		nested, _ := node["children"].([]any)
		for _, child := range slices.Backward(nested) {
			pending = append(pending, child)
		}
		if collection == "texts" {
			if item := doclingJSONTextItem(node, heights); item != nil {
				items = append(items, item)
			}
		}
	}
	return items, true
}

func doclingJSONNode(document map[string]any, ref string) (map[string]any, string) {
	path, local := strings.CutPrefix(ref, "#/")
	if !local {
		return nil, ""
	}
	collection, index, _ := strings.Cut(path, "/")
	switch collection {
	case "texts", "groups", "pictures", "tables", "key_value_items", "form_items":
	default:
		return nil, ""
	}
	entries, _ := document[collection].([]any)
	i, err := strconv.Atoi(index)
	if err != nil || i < 0 || i >= len(entries) || strconv.Itoa(i) != index {
		return nil, ""
	}
	node, _ := entries[i].(map[string]any)
	return node, collection
}

func doclingJSONTextItem(node map[string]any, heights map[int]float64) map[string]any {
	text, _ := node["text"].(string)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if !strings.HasSuffix(text, "\n") {
		// General chunking concatenates adjacent text items.
		text += "\n"
	}
	item := map[string]any{"text": text, DocTypeKey: DocTypeText}
	switch label, _ := node["label"].(string); label {
	case "title", "section_header":
		level := 1
		if value, ok := numericAny(node["level"]); ok && value >= 1 && value <= 6 {
			level = int(value)
		}
		item["layout"] = "heading"
		item["heading_level"] = level
	}

	var positions [][]any
	for _, prov := range doclingProvs(node) {
		page, ok := numericAny(prov["page_no"])
		if !ok || page < 1 || page > doclingMaxSourcePageCount || page != math.Trunc(page) {
			continue
		}
		if _, found := item["page_number"]; !found {
			item["page_number"] = int(page)
		}
		if position := doclingProvPosition(prov, heights); position != nil {
			positions = append(positions, position)
		}
	}
	if positions := validPDFPositions(positions); len(positions) > 0 {
		item["positions"] = positions
		item["_pdf_positions"] = positions
	}
	return item
}
