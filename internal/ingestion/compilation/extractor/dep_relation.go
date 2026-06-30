// Go implementation of dependency-based relation extraction.
// Direct port of Python DepRelationExtractor._extract_from_root logic.
// Operates on a dependency tree (heads + labels) independent of how it was parsed.

package extractor

import (
	"strings"
)

// Verb lemmatization (spaCy lemma → base form, since Go doesn't have spaCy)
var verbLemma = map[string]string{
	"founded": "found", "founding": "found",
	"works": "work", "working": "work",
	"based": "base", "basing": "base",
	"located": "locate", "locating": "locate",
	"situated": "situate", "situating": "situate",
	"acquired": "acquire", "acquiring": "acquire",
	"employed": "employ", "employing": "employ",
	"hired": "hire", "hiring": "hire",
	"born": "bear",
	"joined": "join", "joining": "join",
	"merged": "merge", "merging": "merge",
	"bought": "buy", "buying": "buy",
	"created": "create", "creating": "create",
	"established": "establish", "establishing": "establish",
	"started": "start", "starting": "start",
	"headedquartered": "headquarter",
	"sets": "set",
}

func lemma(w string) string {
	if l, ok := verbLemma[w]; ok {
		return l
	}
	return w
}

// Verb+prep → relation type mapping (same as Python _VERB_RELATIONS)
var depVerbRelations = map[string]string{
	"found+by":        "founded_by",
	"co-found+by":     "founded_by",
	"establish+by":    "founded_by",
	"create+by":       "founded_by",
	"set+up":          "founded_by",
	"start+by":        "founded_by",
	"work+for":        "works_for",
	"employ+by":       "works_for",
	"hire+by":         "works_for",
	"join":            "works_for",
	"base+in":         "located_in",
	"locate+in":       "located_in",
	"situate+in":      "located_in",
	"bear+in":         "born_in",
	"bear+on":         "born_in",
	"acquire+by":      "acquired",
	"merge+with":      "acquired",
	"buy+by":          "acquired",
}

// Copula title patterns (same as Python _COPULA_TITLE_MAP)
var depCopulaTitles = map[string][]string{
	"ceo":       {"ceo_of", "works_for"},
	"cto":       {"works_for"},
	"cfo":       {"works_for"},
	"coo":       {"works_for"},
	"vp":        {"works_for"},
	"director":  {"works_for"},
	"manager":   {"works_for"},
	"engineer":  {"works_for"},
	"employee":  {"works_for"},
	"founder":   {"founded_by"},
	"co-founder": {"founded_by"},
}

// DepToken holds a single token's dependency info
type DepToken struct {
	Text     string `json:"text"`
	Head     int    `json:"head"` // index of head token (-1 = root)
	Dep      string `json:"dep"`  // dependency relation label
	Index    int    `json:"index"`
	POS      string `json:"pos,omitempty"`
}

// DepExtractRelations extracts typed relations from a dependency parse tree.
// tokens: list of tokens with head and dep fields (from spaCy/C++ parser).
// entities: NER entities with character offsets.
// Returns typed relations (excludes "related_to").
func DepExtractRelations(text string, tokens []DepToken, entities []Entity, lang string) []Relation {
	entityMap := buildEntityMap(entities)
	var relations []Relation

	// Find root tokens and extract (spaCy: root head = self index)
	for _, tok := range tokens {
		if tok.Head == tok.Index { // ROOT (self-loop in spaCy convention)
			rels := extractFromRoot(text, tok.Index, tokens, entityMap)
			relations = append(relations, rels...)

			// Copula ("X is [title] of Y") — only for "be" verbs
			if isCopulaVerb(tok) || lemma(strings.ToLower(tok.Text)) == "be" {
				rels := extractCopula(text, tok.Index, tokens, entityMap)
				relations = append(relations, rels...)
			}
		}
	}

	// Deduplicate
	relations = dedupRelations(relations)
	return relations
}

func isCopulaVerb(tok DepToken) bool {
	lower := strings.ToLower(tok.Text)
	return lower == "is" || lower == "are" || lower == "was" || lower == "were" || lower == "be"
}

func extractFromRoot(text string, rootIdx int, tokens []DepToken, entityMap map[string]Entity) []Relation {
	var relations []Relation
	root := tokens[rootIdx]
	verbLemma := lemma(strings.ToLower(root.Text))

	// Collect arguments
	nsubj := getChildEntity(rootIdx, tokens, "nsubj", entityMap)
	nsubjpass := getChildEntity(rootIdx, tokens, "nsubjpass", entityMap)
	dobj := getChildEntity(rootIdx, tokens, "dobj", entityMap)
	agentObj := getAgentPobj(rootIdx, tokens, entityMap)
	prepList := getPrepObjs(rootIdx, tokens, entityMap)
	haveAgent := hasChildDep(rootIdx, tokens, "agent")

	// Passive: "X was founded/acquired by Y"
	if nsubjpass != nil && agentObj != nil && haveAgent {
		if relType := lookupVerb(verbLemma, "by"); relType != "" {
			subj, obj := *nsubjpass, *agentObj
			if relType == "founded_by" {
				subj, obj = *nsubjpass, *agentObj  // ORG→PERSON
			} else if relType == "acquired" {
				subj, obj = *agentObj, *nsubjpass  // buyer→target
			} else {
				subj, obj = *agentObj, *nsubjpass
			}
			relations = append(relations, Relation{
				Subject: Entity{Text: subj.Text, Label: subj.Label, StartChar: subj.StartChar, EndChar: subj.EndChar},
				Predicate: relType,
				Object:   Entity{Text: obj.Text, Label: obj.Label, StartChar: obj.StartChar, EndChar: obj.EndChar},
				Confidence: 0.85,
			})
		}
	}

	// Active: "X VERB Y" or "X VERB prep Y"
	if nsubj != nil {
		if dobj != nil {
			if relType := lookupVerb(verbLemma, ""); relType != "" {
				relations = append(relations, makeRelation(*nsubj, relType, *dobj, 0.85))
			}
		}
		for _, pe := range prepList {
			if relType := lookupVerb(verbLemma, pe.prep); relType != "" {
				relations = append(relations, makeRelation(*nsubj, relType, pe.entity, 0.85))
			}
		}
	}

	// Passive with prep: "X is based/located in Y"
	if nsubjpass != nil {
		for _, pe := range prepList {
			relType := lookupVerb(verbLemma, pe.prep)
			if relType == "" {
				relType = lookupVerb("be+"+verbLemma, pe.prep)
			}
			if relType != "" {
				relations = append(relations, makeRelation(*nsubjpass, relType, pe.entity, 0.85))
			}
		}
	}

	return relations
}

func extractCopula(text string, rootIdx int, tokens []DepToken, entityMap map[string]Entity) []Relation {
	var relations []Relation
	subj := getChildEntity(rootIdx, tokens, "nsubj", entityMap)
	if subj == nil {
		return nil
	}

	// Find attr child that has "of Y"
	var titleLemma string
	var prepObj *Entity
	for _, c := range childrenOf(rootIdx, tokens) {
		if c.Dep == "attr" {
			for _, cc := range childrenOf(c.Index, tokens) {
				if cc.Dep == "prep" && strings.ToLower(cc.Text) == "of" {
					for _, gc := range childrenOf(cc.Index, tokens) {
						if gc.Dep == "pobj" {
							if ent := findEntityInSubtree(gc.Index, tokens, entityMap); ent != nil {
								prepObj = ent
								titleLemma = strings.ToLower(c.Text)
							}
						}
					}
				}
			}
		}
	}

	if titleLemma == "" || prepObj == nil {
		return nil
	}

	for keyword, relTypes := range depCopulaTitles {
		if strings.Contains(titleLemma, keyword) {
			for _, rt := range relTypes {
				relations = append(relations, makeRelation(*subj, rt, *prepObj, 0.85))
			}
			break
		}
	}
	return relations
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type prepEntry struct {
	prep   string
	entity Entity
}

func lookupVerb(verb, prep string) string {
	if prep != "" {
		if v, ok := depVerbRelations[verb+"+"+prep]; ok {
			return v
		}
		return ""
	}
	return depVerbRelations[verb]
}

func getChildEntity(idx int, tokens []DepToken, dep string, emap map[string]Entity) *Entity {
	for _, c := range childrenOf(idx, tokens) {
		if c.Dep == dep {
			return findEntityInSubtree(c.Index, tokens, emap)
		}
	}
	return nil
}

func getAgentPobj(idx int, tokens []DepToken, emap map[string]Entity) *Entity {
	for _, c := range childrenOf(idx, tokens) {
		if c.Dep == "agent" {
			for _, gc := range childrenOf(c.Index, tokens) {
				if gc.Dep == "pobj" {
					return findEntityInSubtree(gc.Index, tokens, emap)
				}
			}
		}
	}
	return nil
}

func getPrepObjs(idx int, tokens []DepToken, emap map[string]Entity) []prepEntry {
	var result []prepEntry
	for _, c := range childrenOf(idx, tokens) {
		if c.Dep == "prep" {
			prepLemma := strings.ToLower(c.Text)
			for _, gc := range childrenOf(c.Index, tokens) {
				if gc.Dep == "pobj" {
					if ent := findEntityInSubtree(gc.Index, tokens, emap); ent != nil {
						result = append(result, prepEntry{prepLemma, *ent})
					}
				}
			}
		}
	}
	return result
}

func hasChildDep(idx int, tokens []DepToken, dep string) bool {
	for _, c := range childrenOf(idx, tokens) {
		if c.Dep == dep {
			return true
		}
	}
	return false
}

func childrenOf(idx int, tokens []DepToken) []DepToken {
	var kids []DepToken
	for _, t := range tokens {
		if t.Head == idx && t.Index != idx { // exclude self-loop (root)
			kids = append(kids, t)
		}
	}
	return kids
}

func findEntityInSubtree(idx int, tokens []DepToken, emap map[string]Entity) *Entity {
	// Collect subtree words excluding structure tokens
	words := collectWords(idx, tokens, map[int]bool{})
	if len(words) == 0 {
		return nil
	}
	text := strings.Join(words, " ")
	key := strings.ToLower(strings.TrimSpace(text))
	if ent, ok := emap[key]; ok {
		return &ent
	}
	// Try splitting on conjunctions
	for _, sep := range []string{" and ", " or ", ", "} {
		if strings.Contains(key, sep) {
			candidate := strings.TrimSpace(strings.SplitN(key, sep, 2)[0])
			if ent, ok := emap[candidate]; ok {
				return &ent
			}
		}
	}
	return nil
}

func collectWords(idx int, tokens []DepToken, visited map[int]bool) []string {
	if visited[idx] {
		return nil
	}
	visited[idx] = true
	tok := tokens[idx]
	// Skip structure words
	if tok.Dep == "prep" || tok.Dep == "punct" || tok.Dep == "det" ||
		tok.Dep == "aux" || tok.Dep == "auxpass" || tok.Dep == "cc" ||
		tok.Dep == "conj" {
		return nil
	}
	// Collect children first, sorted by token index to preserve word order
	var childWords []string
	kids := childrenOf(idx, tokens)
	sortByIndex(kids)
	for _, c := range kids {
		childWords = append(childWords, collectWords(c.Index, tokens, visited)...)
	}
	// Determine position: children before parent if they have lower index
	hasChildBefore := false
	for _, k := range kids {
		if k.Index < idx {
			hasChildBefore = true
			break
		}
	}
	if hasChildBefore {
		return append(childWords, tok.Text)
	}
	return append([]string{tok.Text}, childWords...)
}

func sortByIndex(ts []DepToken) {
	for i := 0; i < len(ts); i++ {
		for j := i + 1; j < len(ts); j++ {
			if ts[j].Index < ts[i].Index {
				ts[i], ts[j] = ts[j], ts[i]
			}
		}
	}
}

func makeRelation(subj Entity, pred string, obj Entity, conf float64) Relation {
	return Relation{
		Subject:    subj,
		Predicate:  pred,
		Object:     obj,
		Confidence: conf,
	}
}

func dedupRelations(rels []Relation) []Relation {
	seen := make(map[string]bool)
	var result []Relation
	for _, r := range rels {
		key := strings.ToLower(r.Subject.Text + "|" + r.Predicate + "|" + r.Object.Text)
		rev := strings.ToLower(r.Object.Text + "|" + r.Predicate + "|" + r.Subject.Text)
		if seen[key] || seen[rev] {
			continue
		}
		seen[key] = true
		result = append(result, r)
	}
	return result
}

func buildEntityMap(entities []Entity) map[string]Entity {
	m := make(map[string]Entity, len(entities)*2)
	for _, e := range entities {
		m[strings.ToLower(e.Text)] = e
		cleaned := strings.TrimRight(e.Text, ".,;:!?")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned != e.Text {
			m[strings.ToLower(cleaned)] = e
		}
	}
	return m
}
