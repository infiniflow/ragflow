package structure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	appcommon "ragflow/internal/common"
	"ragflow/internal/ingestion/component/knowledge_compiler/common"

	"go.uber.org/zap"
)

// CompileConfig carries the per-run identity and extraction settings.
type CompileConfig struct {
	LLMID        string
	Type         Type // inferred compile kind (list / set / hypergraph)
	TenantID     string
	DocID        string
	Variant      common.Variant
	Lang         string
	ParserConfig map[string]any
	// TemplateID is mixed into stable row ids so two templates sharing a
	// compile kind don't collide on identical payloads (mirrors Python's
	// row_seed_extras in _struct_to_doc_storage_doc).
	TemplateID string
}

// extractionTemperature mirrors Python's gen_conf for structure extraction
// (_struct_extract_hypergraph uses temperature 0.1).
var extractionTemperature = 0.1

// disableThinking mirrors knowledge_compile_gen_conf's intent: extraction and
// judging calls must not spend the budget on chain-of-thought. Python disables
// it per model family (deepseek-v4/minimax via extra_body.thinking, qwen3 via
// enable_thinking, everything else via reasoning_effort="none"); the Go chat
// drivers expose a single normalized switch, so every compile call turns it
// off. The one Python exception — qwen3 -preview endpoints that REQUIRE
// thinking enabled — would need a model-name branch in the Go driver.
func chatRequest(llmID, systemPrompt, userPrompt string) common.ChatRequest {
	return common.ChatRequest{
		LLMID:           llmID,
		SystemPrompt:    systemPrompt,
		UserPrompt:      userPrompt,
		Temperature:     &extractionTemperature,
		DisableThinking: true,
	}
}

// extractHypergraph mirrors _struct_extract_hypergraph: stage 1 extracts node
// (entity) items, stage 2 extracts edge (relation) items constrained to the
// stage-1 entities via the {known_nodes} placeholder. The two stages within
// one batch are strictly sequential; batches parallelise at the caller.
func extractHypergraph(ctx context.Context, deps common.Deps, cfg CompileConfig, nodePrompt, edgePromptTmpl, packedText string) (nodes, edges []map[string]any, err error) {
	user := UserPrompt(packedText)
	nodeRaw, err := common.GenJSON(ctx, deps.Chat, chatRequest(cfg.LLMID, nodePrompt, user))
	if err != nil {
		return nil, nil, err
	}
	nodes = unwrapItems(nodeRaw)

	// Known entities: unique values of the config's entity id field, in
	// first-seen order (mirrors _struct_extract_hypergraph's known_keys).
	idField := EntityIDField(cfg.ParserConfig)
	var known []string
	for _, n := range nodes {
		v := strings.TrimSpace(stringOf(n[idField]))
		if v == "" || containsString(known, v) {
			continue
		}
		known = append(known, v)
	}

	// Relations can only reference entities from this batch. If entity extraction
	// returned nothing, skip the second LLM call and avoid creating orphan edges.
	if len(known) == 0 {
		return nodes, nil, nil
	}

	if strings.TrimSpace(edgePromptTmpl) == "" {
		return nodes, nil, nil
	}
	edgeRaw, err := common.GenJSON(ctx, deps.Chat, chatRequest(cfg.LLMID, fillKnownNodes(edgePromptTmpl, known), EdgeUserPrompt(packedText)))
	if err != nil {
		return nil, nil, err
	}
	return nodes, unwrapItems(edgeRaw), nil
}

// unwrapItems mirrors _struct_unwrap_items: the contract is
// {"items": [{...}, ...]}; a top-level array is tolerated defensively.
// GenJSON already guarantees a JSON object, so the list form only appears
// under "items". Non-object entries are dropped.
func unwrapItems(raw map[string]any) []map[string]any {
	if raw == nil {
		return nil
	}
	arr, ok := raw["items"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// payloadChunkIDs mirrors _struct_payload_chunk_ids: keep only model-selected
// chunk IDs that belong to the current batch; fall back to all batch ids when
// the model returned none that qualify.
func payloadChunkIDs(payload map[string]any, batchIDs []string) []string {
	var rawIDs []string
	switch v := payload["source_chunk_ids"].(type) {
	case string:
		rawIDs = []string{v}
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok {
				rawIDs = append(rawIDs, s)
			}
		}
	case []string:
		rawIDs = v
	}
	allowed := make(map[string]bool, len(batchIDs))
	for _, id := range batchIDs {
		allowed[id] = true
	}
	var selected []string
	seen := map[string]bool{}
	for _, id := range rawIDs {
		id = strings.TrimSpace(id)
		if allowed[id] && !seen[id] {
			selected = append(selected, id)
			seen[id] = true
		}
	}
	if len(selected) == 0 {
		return append([]string{}, batchIDs...)
	}
	return selected
}

// payloadDescription mirrors _struct_payload_description: concat the string
// values of every field (lists flattened) with single spaces. It delegates to
// common.PayloadDescription, the shared implementation the tree variant also
// uses, so the two variants cannot drift apart.
func payloadDescription(payload map[string]any) string {
	return common.PayloadDescription(payload, nil)
}

// IndexText returns the flattened payload description for a compiled row's
// content JSON — the exact text Python tokenizes into content_ltks /
// content_sm_ltks (“_tokenize_for_search(_struct_payload_description(payload))“).
// Non-JSON content yields "", so non-structure variants keep tokenizing their
// raw content.
func IndexText(content string) string {
	payload := parsePayload(content)
	if payload == nil {
		return ""
	}
	return payloadDescription(payload)
}

// mentionCountOf mirrors _struct_to_doc_storage_doc's mention_count parsing:
// the payload's own count when it carries a usable one, else 1.
func mentionCountOf(payload map[string]any) int {
	switch v := payload["mention_count"].(type) {
	case float64:
		if v > 0 {
			return int(v)
		}
	case int:
		if v > 0 {
			return v
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return n
		}
	}
	return 1
}

// payloadJSON serialises a payload the way Python's json.dumps(ensure_ascii=
// False) does: Go's json.Marshal escapes <, > and & to < etc., which
// would corrupt entity names containing those characters, so HTML escaping is
// disabled. Keys are alphabetically sorted (map marshal), giving a canonical,
// hash-stable form.
func payloadJSON(payload map[string]any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return "{}"
	}
	return strings.TrimSpace(b.String())
}

// parsePayload is the inverse of payloadJSON (nil on malformed content).
func parsePayload(content string) map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		return nil
	}
	return m
}

// buildRows converts extracted node/edge payloads into entity/relation
// products, mirroring _struct_process_batch: embedding input is
// payloadDescription(payload); content is the payload JSON (Python's
// content_with_weight); source_chunk_ids are the model's picks filtered to
// the batch; the stable row id hashes (content, doc_id, template_id).
func buildRows(ctx context.Context, deps common.Deps, cfg CompileConfig, nodes, edges []map[string]any, batchIDs []string) ([]common.Product, error) {
	srcField, tgtField := RelationMemberFields(cfg.ParserConfig)

	type spec struct {
		kind    string // "entity" | "relation"
		payload map[string]any
	}
	specs := make([]spec, 0, len(nodes)+len(edges))
	for _, p := range nodes {
		specs = append(specs, spec{kind: "entity", payload: p})
	}
	for _, p := range edges {
		if isSelfLoopRelation(p, srcField, tgtField) {
			continue
		}
		specs = append(specs, spec{kind: "relation", payload: p})
	}
	if len(specs) == 0 {
		return nil, nil
	}

	texts := make([]string, len(specs))
	for i, s := range specs {
		texts[i] = payloadDescription(s.payload)
	}
	vectors, err := deps.Embed.Encode(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(specs) {
		return nil, fmt.Errorf("knowledge_compiler: embedding count mismatch (%d vs %d)", len(vectors), len(specs))
	}

	// Alias resolution is an ontology idea: the other kinds' `type` values are not a
	// declared class vocabulary, so mapping them would be inventing a rule. An
	// empty index leaves every observed name exactly as it came.
	classAlias, propAlias := map[string]string{}, map[string]string{}
	if common.IsOntologyKind(stringOf(cfg.ParserConfig["kind"])) {
		classAlias, propAlias = ontologyTermIndex(cfg.ParserConfig)
	}
	droppedValues := 0
	rows := make([]common.Product, 0, len(specs))
	for i, s := range specs {
		content := payloadJSON(s.payload)
		idParts := []string{content, cfg.DocID}
		if cfg.TemplateID != "" {
			idParts = append(idParts, cfg.TemplateID)
		}
		meta := map[string]any{
			"kind":             s.kind,
			"source_chunk_ids": payloadChunkIDs(s.payload, batchIDs),
			// mention_count_int mirrors Python: the payload's own count when it
			// carries one, else 1 (never a synthesized constant).
			"mention_count": mentionCountOf(s.payload),
		}
		if s.kind == "entity" {
			if name := entityName(s.payload); name != "" {
				meta["name"] = name
			}
			// entity_type_kwd is only stamped when the payload has a type —
			// Python omits the column entirely for an untyped entity rather
			// than writing a synthesized "other".
			// A declared alias answers for the class it names, so the row is
			// stamped with the canonical class rather than the spelling the model
			// happened to pick. An undeclared name passes through: drift stays
			// visible (see ontologyTermIndex).
			typ := resolveDeclaredType(classAlias, stringOf(s.payload["type"]))
			if typ != "" {
				meta["entity_type"] = typ
			}
			// The declared datatype attributes travel to the row's `attr` json
			// column, which is what makes them filterable by property name,
			// value and range (ontology.md §4.6). They deliberately stay out of
			// the row's vector: PayloadDescription does not recurse into maps,
			// so the entity vector is not diluted by its attribute values.
			attrs, rejected := ontologyAttributes(s.payload, typ, cfg.ParserConfig)
			droppedValues += rejected
			if len(attrs) > 0 {
				meta["attr"] = attrs
			}
			if desc := strings.TrimSpace(stringOf(s.payload["description"])); desc != "" {
				meta["description"] = desc
			}
		} else {
			from := relationEndpoint(s.payload, srcField, "source", "src", "from")
			to := relationEndpoint(s.payload, tgtField, "target", "tgt", "to")
			if from != "" {
				meta["from"] = from
			}
			if to != "" {
				meta["to"] = to
			}
			if typ := resolveDeclaredType(propAlias, stringOf(s.payload["type"])); typ != "" {
				meta["relation_type"] = typ
			}
		}
		rows = append(rows, common.Product{
			ID:       common.StableRowID(idParts...),
			DocID:    cfg.DocID,
			TenantID: cfg.TenantID,
			Variant:  cfg.Variant,
			Content:  content,
			Vector:   vectors[i],
			Meta:     meta,
		})
	}
	if droppedValues > 0 {
		// A datatype or value-set violation is invisible downstream once the
		// value is gone, so it is reported here: the fix is in the template's
		// declaration, never in the document.
		appcommon.Warn("knowledge_compiler: dropped attribute values that did not fit the ontology declaration",
			zap.String("doc_id", cfg.DocID),
			zap.Int("dropped_values", droppedValues))
	}
	return rows, nil
}

func isSelfLoopRelation(payload map[string]any, sourceField, targetField string) bool {
	from := relationEndpoint(payload, sourceField, "source", "src", "from")
	to := relationEndpoint(payload, targetField, "target", "tgt", "to")
	return isSelfLoop(from, to)
}

// entityTypeKey normalizes an entity name for the name → class map. Only case
// and surrounding whitespace are folded: the edge stage is told to reuse the
// entity list verbatim, so any other difference means a different entity
// rather than a variant spelling of the same one.
func entityTypeKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// stampRelationEndpointTypes writes each relation's endpoint classes into its
// from_type / to_type meta, resolved from the entity rows of the same compile.
//
// These two keys are what the ontology model graph is drawn and counted from,
// so this must run after local dedup: by then aliases have been merged and
// relation endpoints rewritten, and the names being resolved are the final
// ones. An endpoint with no matching entity row is left unstamped rather than
// guessed.
func stampRelationEndpointTypes(prods []common.Product) {
	types := make(map[string]string, len(prods))
	for _, p := range prods {
		if kind, _ := p.Meta["kind"].(string); kind != "entity" {
			continue
		}
		typ, _ := p.Meta["entity_type"].(string)
		if typ = strings.TrimSpace(typ); typ == "" {
			continue
		}
		name := entityNameValue(p)
		if name == "" {
			continue
		}
		if _, seen := types[entityTypeKey(name)]; !seen {
			types[entityTypeKey(name)] = typ
		}
	}
	if len(types) == 0 {
		return
	}
	for i := range prods {
		if kind, _ := prods[i].Meta["kind"].(string); kind != "relation" {
			continue
		}
		from, _ := prods[i].Meta["from"].(string)
		to, _ := prods[i].Meta["to"].(string)
		if t, ok := types[entityTypeKey(from)]; ok {
			prods[i].Meta["from_type"] = t
		}
		if t, ok := types[entityTypeKey(to)]; ok {
			prods[i].Meta["to_type"] = t
		}
	}
}

// ---- The declared ontology (standard model) ----
//
// An ontology template declares one entry per property in its relation section:
// `type` is the property name, `kind` says whether it joins two individuals
// ("object") or gives one a literal ("datatype"), and `domain` / `range` name
// the classes it runs between. `domain` and `range` take a "|"-separated list
// when a property is legitimately polymorphic.

// ontologyProperty is one declared property, keyed by its `type`.
//
// `enum` and `required` are the acceptance half of the declaration: they say what
// a stored value has to look like, not what the model should write. Only `enum`
// also reaches the prompt, because a closed value set is a generative fact (it
// stops the model inventing categories); `required` never does, because telling a
// model an attribute is mandatory is how you get invented values.
type ontologyProperty struct {
	kind     string // "object" | "datatype"
	domains  []string
	ranges   []string
	datatype string
	enum     []string
	required bool
}

// splitValueList reads a declared value set. The template editor writes a YAML
// list, a hand-edited config may carry a "|"-separated string; both mean the same
// closed set.
func splitValueList(v any) []string {
	switch x := v.(type) {
	case string:
		return splitPipeList(x)
	case []string:
		return dedupeValues(x)
	case []any:
		raw := make([]string, 0, len(x))
		for _, item := range x {
			raw = append(raw, stringOf(item))
		}
		return dedupeValues(raw)
	}
	return nil
}

// dedupeValues trims and deduplicates a declared value set, preserving order.
func dedupeValues(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// splitPipeList splits a "|"-separated class list, trimming and deduplicating.
func splitPipeList(raw string) []string {
	parts := strings.Split(raw, "|")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		if p = strings.TrimSpace(p); p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// declaredOntology reads the classes and properties a template declares. A
// property's kind is inferred from its range when the template omits `kind` — a
// range that names a declared class means an object property — mirroring
// validateOntologyTemplate in the template service.
func declaredOntology(parserConfig map[string]any) (map[string]bool, map[string]ontologyProperty) {
	classes := map[string]bool{}
	for _, raw := range configFields(common.GetMap(parserConfig, "entity")) {
		f, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if name := strings.TrimSpace(stringOf(f["type"])); name != "" {
			classes[name] = true
		}
	}
	props := map[string]ontologyProperty{}
	for _, raw := range configFields(common.GetMap(parserConfig, "relation")) {
		f, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(stringOf(f["type"]))
		if name == "" {
			continue
		}
		p := ontologyProperty{
			kind:     strings.ToLower(strings.TrimSpace(stringOf(f["kind"]))),
			domains:  splitPipeList(stringOf(f["domain"])),
			ranges:   splitPipeList(stringOf(f["range"])),
			datatype: strings.ToLower(strings.TrimSpace(stringOf(f["datatype"]))),
			enum:     splitValueList(f["enum"]),
			required: boolValue(f["required"]),
		}
		if p.kind == "" {
			p.kind = "datatype"
			for _, r := range p.ranges {
				if classes[r] {
					p.kind = "object"
					break
				}
			}
		}
		props[name] = p
	}
	return classes, props
}

// boolValue reads a declared boolean. The template editor writes a YAML bool, a
// hand-edited config may carry the string form.
func boolValue(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "yes", "y", "1":
			return true
		}
	}
	return false
}

// ontologyTermIndex maps every declared term AND its declared aliases to the
// canonical name, keyed lowercase.
//
// The model is asked to use the declared names, but it also sees the aliases in
// the prompt, so it sometimes answers with one. Resolving here keeps two things
// true that the rest of the pipeline depends on: the row's `entity_type` /
// `relation_type` is always a DECLARED term (so the model graph, the type-scoped
// retrieval and the quality ledger all agree on one vocabulary), and an alias is
// not miscounted as vocabulary drift.
//
// Only aliases the template declares are mapped: an undeclared name passes
// through untouched, because hiding drift is worse than reporting it.
func ontologyTermIndex(parserConfig map[string]any) (classes, properties map[string]string) {
	classes = map[string]string{}
	properties = map[string]string{}
	add := func(into map[string]string, canonical string, raw any) {
		into[strings.ToLower(canonical)] = canonical
		for _, alias := range splitValueList(raw) {
			if _, taken := into[strings.ToLower(alias)]; !taken {
				into[strings.ToLower(alias)] = canonical
			}
		}
	}
	for _, raw := range configFields(common.GetMap(parserConfig, "entity")) {
		f, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if name := strings.TrimSpace(stringOf(f["type"])); name != "" {
			add(classes, name, f["aliases"])
		}
	}
	for _, raw := range configFields(common.GetMap(parserConfig, "relation")) {
		f, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if name := strings.TrimSpace(stringOf(f["type"])); name != "" {
			add(properties, name, f["aliases"])
		}
	}
	return classes, properties
}

// resolveDeclaredType maps an observed type name to its canonical declared name,
// leaving an undeclared name exactly as observed.
func resolveDeclaredType(index map[string]string, observed string) string {
	observed = strings.TrimSpace(observed)
	if observed == "" {
		return ""
	}
	if canonical, ok := index[strings.ToLower(observed)]; ok {
		return canonical
	}
	return observed
}

// filterOutOfOntologyRelations drops object-property assertions whose endpoint
// classes contradict the template's declaration.
//
// This is the ONLY place the declared domain / range is enforced. The prompt
// states it in prose, and the relation stage never even receives the entities'
// classes (the "## Known Entities" list is names only), so the model can and
// does violate it — see ontology.md §8. Enforcing it here costs no extra model
// call.
//
// Two deliberate NON-drops:
//   - an endpoint whose class is unknown (left unstamped because that entity was
//     not compiled in this batch) is kept: the declaration is not contradicted,
//     and dropping would lose data on partial batches — the ontology model graph
//     reports it as an unattributed relation instead;
//   - an assertion whose property the template does not declare at all is kept:
//     the model graph surfaces such vocabulary drift as "observed, not declared"
//     rather than hiding it.
//
// filterOutOfOntologyRelations splits object-property assertions into the ones
// the template's declaration admits and the ones it rejects, returning both.
//
// This is the ONLY place the declared domain / range is enforced. The prompt
// states it in prose, and the relation stage never even receives the entities'
// classes (the "## Known Entities" list is names only), so the model can and
// does violate it — see ontology.md §8. Enforcing it here costs no extra model
// call.
//
// Three deliberate NON-drops:
//   - an endpoint whose class is unknown (left unstamped because that entity was
//     not compiled in this batch) is kept: the declaration is not contradicted,
//     and dropping would lose data on partial batches — the ontology model graph
//     reports it as an unattributed relation instead;
//   - an assertion whose property the template does not declare at all is kept:
//     vocabulary drift must be visible, not hidden;
//   - an endpoint class that is a DECLARED DESCENDANT of the declared one is
//     kept: rdfs:subClassOf is transitive, so a person satisfies a property
//     declared on agent (ontology.md §8 (20) — the model graph expands
//     inheritance with ancestorChain, and a writer that did not would silently
//     disagree with the view it feeds).
//
// Rejected assertions are returned rather than discarded, so the caller can
// write them down: ontology.md §2.7's feedback loop cannot start from evidence
// that was never recorded (§8 (18)).
func filterOutOfOntologyRelations(prods []common.Product, parserConfig map[string]any) (kept, rejected []common.Product) {
	classes, props := declaredOntology(parserConfig)
	if len(classes) == 0 || len(props) == 0 {
		return prods, nil
	}
	ancestors := ontologyClassAncestors(parserConfig)
	inList := func(list []string, want string) bool {
		for _, v := range list {
			if v == want {
				return true
			}
		}
		return false
	}
	// satisfies reports whether an observed endpoint class is admissible: either
	// the declaration names it, or it is a declared descendant of a named class.
	satisfies := func(observed string, declared []string) bool {
		if observed == "" || len(declared) == 0 {
			return true
		}
		if inList(declared, observed) {
			return true
		}
		for _, ancestor := range ancestors[observed] {
			if inList(declared, ancestor) {
				return true
			}
		}
		return false
	}
	kept = make([]common.Product, 0, len(prods))
	for _, p := range prods {
		if kind, _ := p.Meta["kind"].(string); kind != "relation" {
			kept = append(kept, p)
			continue
		}
		prop, _ := p.Meta["relation_type"].(string)
		spec, isDeclared := props[strings.TrimSpace(prop)]
		if !isDeclared || spec.kind != "object" {
			kept = append(kept, p)
			continue
		}
		fromType, _ := p.Meta["from_type"].(string)
		toType, _ := p.Meta["to_type"].(string)
		fromType, toType = strings.TrimSpace(fromType), strings.TrimSpace(toType)
		if !satisfies(fromType, spec.domains) {
			rejected = append(rejected, droppedRelation(p, "domain", prop, fromType, spec.domains))
			continue
		}
		if !satisfies(toType, spec.ranges) {
			rejected = append(rejected, droppedRelation(p, "range", prop, toType, spec.ranges))
			continue
		}
		kept = append(kept, p)
	}
	return kept, rejected
}

// droppedRelation turns a rejected assertion into a row of its own
// (kind "dropped_relation") instead of letting it vanish. It keeps the same
// endpoint meta a relation carries, so the row lands with prop_kwd /
// from_entity_kwd / to_entity_kwd / from_type_kwd / to_type_kwd and the quality
// panel can list what was rejected, count it per property and point the reader
// at the document it came from. The reason rides in the content, so no column is
// added for it.
func droppedRelation(p common.Product, side, prop, observed string, declared []string) common.Product {
	meta := make(map[string]any, len(p.Meta)+2)
	for k, v := range p.Meta {
		meta[k] = v
	}
	meta["kind"] = "dropped_relation"
	meta["drop_side"] = side
	out := p
	out.Meta = meta
	out.Content = fmt.Sprintf("dropped %s: the template declares %s %s, the assertion used %q",
		prop, side, strings.Join(declared, "|"), observed)
	return out
}

// ontologyClassAncestors maps each declared class to its declared ancestors,
// nearest first (breadth-first over `parent`, `|`-separated), with cycles cut.
// It is the writer-side twin of the model graph's ancestorChain: without it the
// declared domain / range would reject subclass instances that standard
// semantics admit (ontology.md §8 (20)).
func ontologyClassAncestors(parserConfig map[string]any) map[string][]string {
	parents := map[string][]string{}
	for _, raw := range configFields(common.GetMap(parserConfig, "entity")) {
		f, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(stringOf(f["type"]))
		if name == "" {
			continue
		}
		parents[name] = splitPipeList(stringOf(f["parent"]))
	}
	out := map[string][]string{}
	for name := range parents {
		seen := map[string]bool{name: true}
		queue := append([]string{}, parents[name]...)
		var chain []string
		for len(queue) > 0 {
			next := queue[0]
			queue = queue[1:]
			if next == "" || seen[next] {
				continue
			}
			seen[next] = true
			chain = append(chain, next)
			queue = append(queue, parents[next]...)
		}
		if len(chain) > 0 {
			out[name] = chain
		}
	}
	return out
}

// ontologyAttributeSpec is one declared datatype attribute, as the entity prompt
// states it: the property name, the literal type the value is expected in, and
// the closed value set when the template declares one.
type ontologyAttributeSpec struct {
	name     string
	datatype string
	enum     []string
}

// ontologyAttributeSpecs maps each declared class to the datatype attributes it
// carries — its own plus the ones it inherits.
//
// Inheritance matters: rdfs:subClassOf says a subclass has every attribute its
// parent has, so an attribute declared on `agent` belongs to `person` and
// `organization` too. A class may declare several parents (`|`-separated), so
// the walk is breadth-first over the whole parent set, nearest first — the same
// order ontologyClassAncestors returns. Own attributes come first, then
// inherited ones, so a subclass overrides its parent for the same name, and a
// class closer to the subclass overrides a farther ancestor. Order follows
// declaration order in the template, keeping the prompt text and stored keys
// deterministic.
func ontologyAttributeSpecs(parserConfig map[string]any) map[string][]ontologyAttributeSpec {
	_, props := declaredOntology(parserConfig)

	parentOf := map[string][]string{}
	order := []string{}
	for _, raw := range configFields(common.GetMap(parserConfig, "entity")) {
		f, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(stringOf(f["type"]))
		if name == "" {
			continue
		}
		order = append(order, name)
		if parents := splitPipeList(stringOf(f["parent"])); len(parents) > 0 {
			parentOf[name] = parents
		}
	}

	own := map[string][]ontologyAttributeSpec{}
	for _, raw := range configFields(common.GetMap(parserConfig, "relation")) {
		f, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(stringOf(f["type"]))
		if name == "" {
			continue
		}
		spec, known := props[name]
		if !known || spec.kind != "datatype" {
			continue
		}
		for _, d := range spec.domains {
			own[d] = append(own[d], ontologyAttributeSpec{name: name, datatype: spec.datatype, enum: spec.enum})
		}
	}

	out := make(map[string][]ontologyAttributeSpec, len(order))
	for _, class := range order {
		seenName := map[string]bool{}
		seenClass := map[string]bool{class: true}
		attrs := []ontologyAttributeSpec{}
		queue := []string{class}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, a := range own[cur] {
				if !seenName[a.name] {
					seenName[a.name] = true
					attrs = append(attrs, a)
				}
			}
			for _, parent := range parentOf[cur] {
				if seenClass[parent] {
					// A cycle cannot end the walk; the template validator
					// rejects one, this only keeps the reader finite.
					continue
				}
				seenClass[parent] = true
				queue = append(queue, parent)
			}
		}
		if len(attrs) > 0 {
			out[class] = attrs
		}
	}
	return out
}

// datatypePropertiesByClass is ontologyAttributeSpecs reduced to the attribute
// names, which is all the projection needs.
func datatypePropertiesByClass(parserConfig map[string]any) map[string][]string {
	out := map[string][]string{}
	for class, specs := range ontologyAttributeSpecs(parserConfig) {
		names := make([]string, 0, len(specs))
		for _, s := range specs {
			names = append(names, s.name)
		}
		out[class] = names
	}
	return out
}

// ontologyAttributes collects one entity's declared datatype attributes out of
// its payload, keyed for the row's `attr` json column.
//
// Two shapes are accepted, so the template decides how the model is asked to
// return them:
//
//   - a top-level payload key named after the attribute (what listing them as
//     entity `output_fields[]` scalars would produce);
//   - a nested object under one of the entity section's declared
//     `output_fields[].name` values — the `shape` mechanism, which is the
//     practical choice when each class carries a different attribute set.
//
// Only DECLARED attributes survive, so a model that invents one cannot smuggle
// it into the filterable `attr` column.
//
// A surviving value is also coerced to its declared `datatype`, because `attr` is
// the only place a value is ever range-filtered: a date stored as "1st November"
// and an integer stored as "7" silently answer zero rows. A value that does not
// fit its declaration is DROPPED and counted rather than stored as if it were
// valid, so a wrong datatype shows up as a number instead of as missing data.
//
// An attribute whose datatype the template leaves empty is stored verbatim: the
// template never said what the value is, and inventing a type here would reject
// data the model was never asked to format.
func ontologyAttributes(payload map[string]any, entityType string, parserConfig map[string]any) (map[string]any, int) {
	if entityType == "" || payload == nil {
		return nil, 0
	}
	specs := ontologyAttributeSpecs(parserConfig)[entityType]
	if len(specs) == 0 {
		return nil, 0
	}
	out := map[string]any{}
	rejected := 0
	take := func(spec ontologyAttributeSpec, raw any) {
		if _, taken := out[spec.name]; taken || isEmptyScalar(raw) {
			return
		}
		value, ok := normalizeAttributeValue(raw, spec.datatype, spec.enum)
		if !ok {
			rejected++
			return
		}
		out[spec.name] = value
	}
	for _, spec := range specs {
		take(spec, payload[spec.name])
	}
	for _, raw := range configOutputFields(common.GetMap(parserConfig, "entity")) {
		f, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		nested, ok := payload[strings.TrimSpace(stringOf(f["name"]))].(map[string]any)
		if !ok {
			continue
		}
		for _, spec := range specs {
			take(spec, nested[spec.name])
		}
	}
	if len(out) == 0 {
		return nil, rejected
	}
	return out, rejected
}

// normalizeAttributeValue coerces one declared attribute value to its datatype,
// enforcing a declared value set when there is one. ok=false means the value does
// not fit the declaration and must not be stored.
func normalizeAttributeValue(v any, datatype string, enum []string) (any, bool) {
	datatype = strings.ToLower(strings.TrimSpace(datatype))
	if datatype == "list" || datatype == "array" {
		items := scalarList(v)
		if len(items) == 0 {
			return nil, false
		}
		if len(enum) == 0 {
			return items, true
		}
		kept := make([]string, 0, len(items))
		for _, item := range items {
			if inValueSet(enum, item) {
				kept = append(kept, item)
			}
		}
		if len(kept) == 0 {
			return nil, false
		}
		return kept, true
	}
	text := scalarText(v)
	if text == "" {
		return nil, false
	}
	if len(enum) > 0 && !inValueSet(enum, text) {
		return nil, false
	}
	switch datatype {
	case "":
		// The template declared no type: keep the payload value as it came. Only a
		// string is trimmed — a number or a bool must not become text, because
		// that is exactly the type loss this function exists to prevent.
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s), true
		}
		return v, true
	case "string":
		return text, true
	case "int", "integer":
		if n, err := strconv.ParseInt(strings.ReplaceAll(text, ",", ""), 10, 64); err == nil {
			return n, true
		}
		return nil, false
	case "float", "number":
		if f, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", ""), 64); err == nil {
			return f, true
		}
		return nil, false
	case "bool", "boolean":
		if b, ok := declaredBool(text); ok {
			return b, true
		}
		return nil, false
	case "date":
		if iso, ok := normalizeDate(text); ok {
			return iso, true
		}
		return nil, false
	}
	// An unknown datatype is a template error the validator already rejects; not
	// dropping on a guess keeps a hand-edited config from discarding data.
	return v, true
}

// scalarText renders a payload scalar as text, returning "" for a value that is
// not a scalar (a map or a list has no single text form).
func scalarText(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case json.Number:
		return strings.TrimSpace(x.String())
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	return ""
}

// scalarList reads a list-valued attribute, accepting a single scalar as a
// one-item list (the model often answers a `list` attribute with one value).
func scalarList(v any) []string {
	switch x := v.(type) {
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if text := scalarText(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	case []string:
		return dedupeValues(x)
	}
	if text := scalarText(v); text != "" {
		return []string{text}
	}
	return nil
}

// inValueSet reports whether a value belongs to a declared closed set, matching
// case-insensitively so a capitalisation difference is not a rejection.
func inValueSet(set []string, value string) bool {
	for _, want := range set {
		if want == value || strings.EqualFold(want, value) {
			return true
		}
	}
	return false
}

// declaredBool reads a boolean-ish literal. The non-English forms are accepted
// because the sources are: a Chinese document answers "是", not "true".
func declaredBool(text string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "true", "yes", "y", "1", "是", "对":
		return true, true
	case "false", "no", "n", "0", "否", "不":
		return false, true
	}
	return false, false
}

// normalizeDate rewrites a declared `date` value as ISO 8601 so range filters
// work. A bare year is kept as a year: it is what the source says, and padding it
// to January 1st would invent precision the document does not have.
func normalizeDate(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	if len(text) == 4 && text >= "1000" && text <= "2999" {
		if _, err := strconv.Atoi(text); err == nil {
			return text, true
		}
	}
	for _, layout := range []string{
		"2006-01-02", "2006/01/02", "2006.01.02", "2006-1-2", "2006/1/2",
		"2 January 2006", "2 Jan 2006", "January 2, 2006", "Jan 2, 2006",
		time.RFC3339, time.RFC3339Nano,
	} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.Format("2006-01-02"), true
		}
	}
	if len(text) >= len("2006-01-02T15:04:05") {
		if parsed, err := time.Parse("2006-01-02T15:04:05", text[:len("2006-01-02T15:04:05")]); err == nil {
			return parsed.Format("2006-01-02"), true
		}
	}
	return "", false
}

// isEmptyScalar reports whether a payload value carries nothing worth storing.
// Maps count as empty: an attribute value is a scalar or a list of scalars, and
// a map here would be an un-declared nested object.
func isEmptyScalar(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case []any:
		return len(x) == 0
	case []string:
		return len(x) == 0
	case map[string]any:
		return true
	}
	return false
}

func isSelfLoop(from, to string) bool {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	return from != "" && from == to
}

func filterSelfLoopRelations(rows []common.Product) []common.Product {
	filtered := make([]common.Product, 0, len(rows))
	for _, row := range rows {
		if kind, _ := row.Meta["kind"].(string); kind == "relation" {
			from, _ := row.Meta["from"].(string)
			to, _ := row.Meta["to"].(string)
			if payload := parsePayload(row.Content); payload != nil {
				if payloadFrom := relationEndpoint(payload, "", "source", "src", "from"); payloadFrom != "" {
					from = payloadFrom
				}
				if payloadTo := relationEndpoint(payload, "", "target", "tgt", "to"); payloadTo != "" {
					to = payloadTo
				}
			}
			if isSelfLoop(from, to) {
				continue
			}
		}
		filtered = append(filtered, row)
	}
	return filtered
}

// entityName mirrors _struct_graph_entity's name resolution
// (name → text → term → title, "-1" sentinel rejected).
func entityName(payload map[string]any) string {
	for _, k := range []string{"name", "text", "term", "title"} {
		if s := strings.TrimSpace(stringOf(payload[k])); s != "" && s != "-1" {
			return s
		}
	}
	return ""
}

// relationEndpoint resolves a relation's endpoint: the config-declared member
// field first, then the conventional aliases (mirrors _struct_graph_relation
// combined with _struct_relation_member_fields).
func relationEndpoint(payload map[string]any, declared string, aliases ...string) string {
	if declared != "" {
		if s := strings.TrimSpace(stringOf(payload[declared])); s != "" && s != "-1" {
			return s
		}
	}
	for _, k := range aliases {
		if s := strings.TrimSpace(stringOf(payload[k])); s != "" && s != "-1" {
			return s
		}
	}
	return ""
}

// stringOf renders a scalar payload value; maps/slices render as "" (they are
// not scalar text).
func stringOf(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strings.TrimSuffix(fmt.Sprintf("%v", x), ".0")
	case bool:
		return fmt.Sprintf("%v", x)
	default:
		return ""
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
