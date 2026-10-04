package common

import "strings"

// FilterOrphanRelations removes relation products whose endpoints are not
// present in the same compilation result. Entity and relation rows are scoped
// by variant, template, and compile kind so unrelated templates cannot satisfy
// each other's endpoints.
func FilterOrphanRelations(products []Product) []Product {
	entities := make(map[string]struct{}, len(products))
	for _, product := range products {
		if product.Meta == nil || strings.TrimSpace(metaString(product.Meta, "kind")) != "entity" {
			continue
		}
		name := normalizeRelationEndpoint(metaString(product.Meta, "name"))
		if name != "" {
			entities[relationScope(product)+"\x00"+name] = struct{}{}
		}
	}

	filtered := make([]Product, 0, len(products))
	for _, product := range products {
		from := normalizeRelationEndpoint(metaString(product.Meta, "from"))
		to := normalizeRelationEndpoint(metaString(product.Meta, "to"))
		isRelation := strings.TrimSpace(metaString(product.Meta, "kind")) == "relation" || (from != "" || to != "")
		if isRelation && (from == "" || to == "" || !hasRelationEntity(entities, product, from) || !hasRelationEntity(entities, product, to)) {
			continue
		}
		filtered = append(filtered, product)
	}
	return filtered
}

func hasRelationEntity(entities map[string]struct{}, product Product, name string) bool {
	_, ok := entities[relationScope(product)+"\x00"+name]
	return ok
}

func relationScope(product Product) string {
	compileKind := metaString(product.Meta, "compile_kwd")
	if compileKind == "" {
		compileKind = string(product.Variant)
	}
	template := strings.TrimSpace(product.TemplateID)
	if template == "" {
		template = strings.TrimSpace(product.Kind)
	}
	return string(product.Variant) + "\x00" + template + "\x00" + compileKind
}

func normalizeRelationEndpoint(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func metaString(meta map[string]any, key string) string {
	value, _ := meta[key].(string)
	return value
}
