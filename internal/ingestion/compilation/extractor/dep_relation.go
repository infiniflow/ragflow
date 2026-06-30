// Go implementation of dependency-based relation extraction.
// Direct port of Python DepRelationExtractor — semantica-aligned.
// Operates on a dependency tree (heads + labels) independent of parser.

package extractor

import "strings"

// Verb lemmatization — multi-language
var verbLemma = map[string]string{
	// English
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
	"led": "lead", "leading": "lead",
	"managed": "manage", "managing": "manage",
	"headed": "head", "heading": "head",
	"ran": "run", "running": "run",
	"owned": "own", "owning": "own",
	"developed": "develop", "developing": "develop",
	"wrote": "write", "written": "write", "writing": "write",
	"published": "publish", "publishing": "publish",
	"invested": "invest", "investing": "invest",
	"partnered": "partner", "partnering": "partner",
	"collaborated": "collaborate", "collaborating": "collaborate",
	"sets": "set",
	// German
	"gegründet": "gründen", "gründete": "gründen",
	"arbeitet": "arbeiten", "arbeitete": "arbeiten",
	"befindet": "befinden",
	"liegt": "liegen", "lag": "liegen",
	"geboren": "gebären",
	"erworben": "erwerben", "erwarb": "erwerben",
	"gekauft": "kaufen", "kaufte": "kaufen",
	"übernommen": "übernehmen", "übernahm": "übernehmen",
	// French
	"fondé": "fonder", "fondée": "fonder",
	"créé": "créer", "créée": "créer",
	"travaille": "travailler",
	"employé": "employer", "employée": "employer",
	"situé": "situer", "située": "situer",
	"né": "naître", "née": "naître",
	"acquis": "acquérir",
	// Spanish + Portuguese (shared forms)
	"fundado": "fundar", "fundada": "fundar",
	"creado": "crear", "creada": "crear",
	"criado": "criar", "criada": "criar",
	"trabaja": "trabajar", "trabalha": "trabalhar",
	"ubicado": "ubicar", "ubicada": "ubicar",
	"situado": "situar", "situada": "situar",
	"localizado": "localizar", "localizada": "localizar",
	"sediado": "sediar", "sediada": "sediar",
	"nacido": "nacer", "nacida": "nacer",
	"nascido": "nascer", "nascida": "nascer",
}

func lemma(w string) string {
	if l, ok := verbLemma[w]; ok {
		return l
	}
	return w
}

// Verb+prep → relation type (multi-language)
var depVerbRelations = map[string]string{
	// English
	"found+by": "founded_by", "co-found+by": "founded_by",
	"establish+by": "founded_by", "create+by": "founded_by",
	"set+up": "founded_by", "start+by": "founded_by",
	"work+for": "works_for", "employ+by": "works_for",
	"hire+by": "works_for", "join": "works_for",
	"lead+by": "works_for", "manage+by": "works_for",
	"head+by": "works_for", "run+by": "works_for",
	"own+by": "owns", "develop+by": "develops",
	"write+by": "wrote", "publish+by": "published",
	"invest+in": "invests_in", "partner+with": "partners_with",
	"collaborate+with": "collaborates_with",
	"merge+with": "merged_with", "base+in": "located_in",
	"locate+in": "located_in", "situate+in": "located_in",
	"headquarter+in": "located_in", "bear+in": "born_in",
	"bear+on": "born_in", "acquire+by": "acquired", "buy+by": "acquired",
	// German
	"gründen+von": "founded_by",
	"arbeiten+für": "works_for", "beschäftigen+durch": "works_for",
	"sich+befinden": "located_in", "liegen+in": "located_in",
	"sitzen+in": "located_in", "gebären+in": "born_in",
	"erwerben+durch": "acquired", "übernehmen+durch": "acquired",
	// French
	"fonder+par": "founded_by", "créer+par": "founded_by",
	"travailler+pour": "works_for", "situer+à": "located_in",
	"baser+à": "located_in", "naître+à": "born_in",
	"acquérir+par": "acquired",
	// Spanish + Portuguese (shared lemmas)
	"fundar+por": "founded_by", "crear+por": "founded_by",
	"criar+por": "founded_by", "trabajar+para": "works_for",
	"trabalhar+para": "works_for", "ubicar+en": "located_in",
	"situar+en": "located_in", "localizar+em": "located_in",
	"situar+em": "located_in", "sediar+em": "located_in",
	"nacer+en": "born_in", "nascer+em": "born_in",
	"adquirir+por": "acquired",
}

// Copula title patterns
var depCopulaTitles = map[string][]string{
	"ceo": {"ceo_of", "works_for"}, "cto": {"works_for"},
	"cfo": {"works_for"}, "coo": {"works_for"},
	"vp": {"works_for"}, "director": {"works_for"},
	"manager": {"works_for"}, "engineer": {"works_for"},
	"employee": {"works_for"},
	"founder": {"founded_by"}, "co-founder": {"founded_by"},
}

// Multi-hop inference rules
var multiHopRules = map[string]map[string]string{
	"ceo_of":     {"is_subsidiary_of": "works_for", "located_in": "works_for"},
	"works_for":  {"is_subsidiary_of": "works_for"},
	"founded_by": {"is_subsidiary_of": "founded_by"},
}

// DepToken holds token dependency info
type DepToken struct {
	Text  string `json:"text"`
	Head  int    `json:"head"`
	Dep   string `json:"dep"`
	Index int    `json:"index"`
	POS   string `json:"pos,omitempty"`
}

// DepExtractRelations extracts typed relations from a dependency parse tree.
func DepExtractRelations(text string, tokens []DepToken, entities []Entity, lang string) []Relation {
	entityMap := buildEntityMapMulti(entities)
	var relations []Relation

	for _, tok := range tokens {
		if tok.Head != tok.Index {
			continue
		}
		// Check negation
		if hasNegation(tok.Index, tokens) {
			continue
		}
		rels := extractFromRoot(text, tok.Index, tokens, entityMap)
		relations = append(relations, rels...)
		if isCopulaVerb(tok) || lemma(strings.ToLower(tok.Text)) == "be" {
			rels := extractCopula(text, tok.Index, tokens, entityMap)
			relations = append(relations, rels...)
		}
	}

	// Multi-hop inference
	relations = inferMultiHop(relations)
	relations = dedupRelations(relations)
	return relations
}

func hasNegation(idx int, tokens []DepToken) bool {
	for _, t := range tokens {
		if t.Head == idx && t.Dep == "neg" {
			return true
		}
	}
	return false
}

func isCopulaVerb(tok DepToken) bool {
	lower := strings.ToLower(tok.Text)
	return lower == "is" || lower == "are" || lower == "was" || lower == "were" || lower == "be"
}

// Multi-hop inference
func inferMultiHop(rels []Relation) []Relation {
	bySubj := make(map[string][]Relation)
	for _, r := range rels {
		if r.Predicate == "related_to" {
			continue
		}
		key := strings.ToLower(r.Subject.Text)
		bySubj[key] = append(bySubj[key], r)
	}

	var inferred []Relation
	for _, r := range rels {
		if r.Predicate == "related_to" {
			continue
		}
		objKey := strings.ToLower(r.Object.Text)
		if chain, ok := bySubj[objKey]; ok {
			for _, r2 := range chain {
				if hopRules, ok := multiHopRules[r.Predicate]; ok {
					if inferredPred, ok := hopRules[r2.Predicate]; ok {
						conf := r.Confidence
						if r2.Confidence < conf {
							conf = r2.Confidence
						}
						conf *= 0.9
						inferred = append(inferred, Relation{
							Subject:    r.Subject,
							Predicate:  inferredPred,
							Object:     r2.Object,
							Confidence: conf,
						})
					}
				}
			}
		}
	}
	return append(rels, inferred...)
}

// Entity map: multi-occurrence aware
func buildEntityMapMulti(entities []Entity) map[string][]Entity {
	m := make(map[string][]Entity)
	for _, e := range entities {
		key := strings.ToLower(e.Text)
		m[key] = append(m[key], e)
		cleaned := strings.TrimRight(e.Text, ".,;:!?")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned != e.Text {
			ckey := strings.ToLower(cleaned)
			m[ckey] = append(m[ckey], e)
		}
	}
	return m
}

func findBestEntity(key string, entityMap map[string][]Entity) *Entity {
	entries := entityMap[strings.ToLower(strings.TrimSpace(key))]
	if len(entries) == 0 {
		return nil
	}
	return &entries[0]
}

// extractFromRoot — passive/active/preposition patterns
func extractFromRoot(text string, rootIdx int, tokens []DepToken, entityMap map[string][]Entity) []Relation {
	var relations []Relation
	root := tokens[rootIdx]
	verbLemma := lemma(strings.ToLower(root.Text))

	nsubj := getChildEntity(rootIdx, tokens, "nsubj", entityMap)
	nsubjpass := getChildEntity(rootIdx, tokens, "nsubjpass", entityMap)
	dobj := getChildEntity(rootIdx, tokens, "dobj", entityMap)
	agentObj := getAgentPobj(rootIdx, tokens, entityMap)
	prepList := getPrepObjs(rootIdx, tokens, entityMap)
	haveAgent := hasChildDep(rootIdx, tokens, "agent")

	// Passive
	if nsubjpass != nil && agentObj != nil && haveAgent {
		if relType := lookupVerb(verbLemma, "by"); relType != "" {
			subj, obj := *nsubjpass, *agentObj
			if relType != "founded_by" && relType != "acquired" {
				subj, obj = *agentObj, *nsubjpass
			}
			relations = append(relations, makeRelation(subj, relType, obj, 0.90))
		}
	}

	// Active
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

	// Passive with prep
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

func extractCopula(text string, rootIdx int, tokens []DepToken, entityMap map[string][]Entity) []Relation {
	var relations []Relation
	subj := getChildEntity(rootIdx, tokens, "nsubj", entityMap)
	if subj == nil {
		return nil
	}
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
				relations = append(relations, makeRelation(*subj, rt, *prepObj, 0.88))
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

func getChildEntity(idx int, tokens []DepToken, dep string, emap map[string][]Entity) *Entity {
	for _, c := range childrenOf(idx, tokens) {
		if c.Dep == dep {
			return findEntityInSubtree(c.Index, tokens, emap)
		}
	}
	return nil
}

func getAgentPobj(idx int, tokens []DepToken, emap map[string][]Entity) *Entity {
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

func getPrepObjs(idx int, tokens []DepToken, emap map[string][]Entity) []prepEntry {
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
		if t.Head == idx && t.Index != idx {
			kids = append(kids, t)
		}
	}
	return kids
}

func findEntityInSubtree(idx int, tokens []DepToken, emap map[string][]Entity) *Entity {
	words := collectWords(idx, tokens, map[int]bool{})
	if len(words) == 0 {
		return nil
	}
	text := strings.Join(words, " ")
	key := strings.ToLower(strings.TrimSpace(text))
	if ent := findBestEntity(key, emap); ent != nil {
		return ent
	}
	for _, sep := range []string{" and ", " or ", ", "} {
		if strings.Contains(key, sep) {
			candidate := strings.TrimSpace(strings.SplitN(key, sep, 2)[0])
			if ent := findBestEntity(candidate, emap); ent != nil {
				return ent
			}
		}
	}
	// Fuzzy: try substring match
	for ek, ev := range emap {
		if strings.Contains(ek, key) || strings.Contains(key, ek) {
			e := ev[0]
			return &e
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
	if tok.Dep == "prep" || tok.Dep == "punct" || tok.Dep == "det" ||
		tok.Dep == "aux" || tok.Dep == "auxpass" || tok.Dep == "cc" ||
		tok.Dep == "conj" || tok.Dep == "neg" {
		return nil
	}
	var childWords []string
	kids := childrenOf(idx, tokens)
	sortByIndex(kids)
	for _, c := range kids {
		childWords = append(childWords, collectWords(c.Index, tokens, visited)...)
	}
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
