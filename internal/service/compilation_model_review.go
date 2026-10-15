package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// Assertion review is the human half of the evidence contract: the compiler can
// prove a quote exists (structure/evidence.go) and the projection can show it,
// but "does this assertion hold" is a judgement about the text, so it is recorded
// as a decision on the row rather than guessed at compile time (ontology2.md
// §4.2, 人可审核).
//
// The decision lives in columns, not in the row payload: a document-level row's
// id is a hash over its content, so a payload write would both desynchronise the
// identity and be overwritten by the next compile. It is also deliberately not a
// delete: a rejected assertion has to survive the next compile, otherwise every
// recompile would resurrect the knowledge someone just refused.
const (
	// assertionReviewDefaultLimit / Max bound the page the queue returns.
	assertionReviewDefaultLimit = 30
	assertionReviewMaxLimit     = 200
	// assertionReviewScanCap bounds how many rows one page may scan. The two item
	// queries are windowed by offset+limit, so a deep page degrades into a capped
	// window rather than an unbounded read.
	assertionReviewScanCap = 500
	// assertionReviewMaxBatch bounds one decision request.
	assertionReviewMaxBatch = 200
)

// AssertionReviewStatus is the review state of one compiled assertion.
type AssertionReviewStatus string

const (
	// AssertionReviewUnreviewed covers both an absent status column (rows compiled
	// before review existed) and an explicit "unreviewed" (a decision that was
	// reset), so the two cannot drift apart into different buckets.
	AssertionReviewUnreviewed AssertionReviewStatus = "unreviewed"
	AssertionReviewAccepted   AssertionReviewStatus = "accepted"
	AssertionReviewRejected   AssertionReviewStatus = "rejected"
	AssertionReviewAll        AssertionReviewStatus = "all"
)

// NormalizeAssertionReviewStatus maps the request vocabulary onto the stored one,
// defaulting to the queue's purpose: unreviewed rows.
func NormalizeAssertionReviewStatus(raw string) AssertionReviewStatus {
	switch AssertionReviewStatus(strings.ToLower(strings.TrimSpace(raw))) {
	case AssertionReviewAccepted:
		return AssertionReviewAccepted
	case AssertionReviewRejected:
		return AssertionReviewRejected
	case AssertionReviewAll, "any":
		return AssertionReviewAll
	default:
		return AssertionReviewUnreviewed
	}
}

// ModelAssertionReviewInput is one review-queue query.
type ModelAssertionReviewInput struct {
	TenantID   string
	TemplateID string
	Status     AssertionReviewStatus
	Class      string
	Property   string
	SourceType string
	TargetType string
	DatasetIDs []string
	Limit      int
	Offset     int
}

// ModelAssertionReviewCounts is the review progress of one model: the queue's
// headline numbers, computed with one count query per bucket so the UI never has
// to derive progress from the page it happens to be looking at.
type ModelAssertionReviewCounts struct {
	Unreviewed int `json:"unreviewed"`
	Accepted   int `json:"accepted"`
	Rejected   int `json:"rejected"`
}

// ModelAssertionReviewItem is one compiled assertion as the queue reports it.
type ModelAssertionReviewItem struct {
	ID          string `json:"id"`
	DatasetID   string `json:"dataset_id"`
	DatasetName string `json:"dataset_name,omitempty"`
	Kind        string `json:"kind"`
	// Scope is "doc" for a document-level claim and "dataset" for the folded one.
	// The two are reviewed independently: approving a corpus-level fold is not the
	// same statement as approving what one document said.
	Scope           string                   `json:"scope,omitempty"`
	Name            string                   `json:"name,omitempty"`
	Type            string                   `json:"type,omitempty"`
	Description     string                   `json:"description,omitempty"`
	MentionCount    int                      `json:"mention_count,omitempty"`
	From            string                   `json:"from,omitempty"`
	To              string                   `json:"to,omitempty"`
	Property        string                   `json:"property,omitempty"`
	SourceType      string                   `json:"source_type,omitempty"`
	TargetType      string                   `json:"target_type,omitempty"`
	Attributes      map[string]interface{}   `json:"attributes,omitempty"`
	Version         int64                    `json:"version,omitempty"`
	SourceChunkIDs  []string                 `json:"source_chunk_ids,omitempty"`
	DocumentIDs     []string                 `json:"document_ids,omitempty"`
	Evidence        []map[string]interface{} `json:"evidence,omitempty"`
	ReviewStatus    string                   `json:"review_status,omitempty"`
	ReviewBy        string                   `json:"review_by,omitempty"`
	ReviewAt        int64                    `json:"review_at,omitempty"`
	ReviewNote      string                   `json:"review_note,omitempty"`
	Conflict        bool                     `json:"conflict,omitempty"`
	ConflictTargets []string                 `json:"conflict_targets,omitempty"`
	ConflictTypes   []string                 `json:"conflict_types,omitempty"`
}

// ModelAssertionReviewQueue is the review worklist of one model.
type ModelAssertionReviewQueue struct {
	TemplateID        string                     `json:"template_id"`
	ModelVersion      int                        `json:"model_version"`
	Status            string                     `json:"status"`
	Datasets          []ModelInspectDataset      `json:"datasets"`
	SkippedDatasetIDs []string                   `json:"skipped_dataset_ids"`
	Counts            ModelAssertionReviewCounts `json:"counts"`
	Total             int64                      `json:"total"`
	Truncated         bool                       `json:"truncated"`
	ConflictCount     int                        `json:"conflicts"`
	Limit             int                        `json:"limit"`
	Offset            int                        `json:"offset"`
	Items             []ModelAssertionReviewItem `json:"items"`
}

// ModelAssertionReviewDecision is one accept/reject/reset of one row.
type ModelAssertionReviewDecision struct {
	ID        string `json:"id"`
	DatasetID string `json:"dataset_id"`
	Action    string `json:"action"`
	Note      string `json:"note"`
}

// ModelAssertionReviewActionInput is one batch of decisions.
type ModelAssertionReviewActionInput struct {
	TenantID   string
	TemplateID string
	ActorID    string
	Items      []ModelAssertionReviewDecision
}

// ModelAssertionReviewFailure reports one decision that was not applied. The
// others in the batch still are: each assertion is an independent judgement, so
// there is no reason to make a reviewer re-submit the whole batch because one row
// moved on.
type ModelAssertionReviewFailure struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// ModelAssertionReviewResult is the outcome of one batch.
type ModelAssertionReviewResult struct {
	Updated int                           `json:"updated"`
	Failed  []ModelAssertionReviewFailure `json:"failed"`
	Counts  ModelAssertionReviewCounts    `json:"counts"`
}

// ReviewAssertionQueue lists the assertions of one model across the datasets that
// compile it, filtered by review state.
//
// It is model-scoped (not dataset-scoped) because that is the question a reviewer
// has: "what has this model produced that nobody has checked?". One engine query
// per kind covers every dataset at once — the rows of a tenant live in one index,
// partitioned by kb_id — so the fan-out that Inspect needs per dataset is not
// needed here.
func (s *CompilationTemplateGovernanceService) ReviewAssertionQueue(ctx context.Context, db *gorm.DB, in ModelAssertionReviewInput) (*ModelAssertionReviewQueue, error) {
	template, err := s.templateDAO.GetTemplate(ctx, db, in.TenantID, in.TemplateID)
	if err != nil {
		return nil, err
	}
	datasets, skipped, datasetIDs, err := reviewScopeDatasets(ctx, db, in.TenantID, template, in.DatasetIDs)
	if err != nil {
		return nil, err
	}

	status := NormalizeAssertionReviewStatus(string(in.Status))
	limit := in.Limit
	if limit <= 0 {
		limit = assertionReviewDefaultLimit
	}
	if limit > assertionReviewMaxLimit {
		limit = assertionReviewMaxLimit
	}
	offset := in.Offset
	if offset < 0 {
		offset = 0
	}

	queue := &ModelAssertionReviewQueue{
		TemplateID:        template.ID,
		ModelVersion:      template.Version,
		Status:            string(status),
		Datasets:          datasets,
		SkippedDatasetIDs: skipped,
		Truncated:         len(skipped) > 0,
		Limit:             limit,
		Offset:            offset,
		Items:             []ModelAssertionReviewItem{},
	}
	if len(datasetIDs) == 0 {
		return queue, nil
	}

	counts, err := reviewCounts(ctx, in.TenantID, datasetIDs)
	if err != nil {
		return nil, err
	}
	queue.Counts = counts

	// One window covers both kinds: the page is (offset, limit) over the merged,
	// sorted item list, so a caller that pages never sees a row twice nor misses
	// one because the two kinds advanced at different speeds.
	window := offset + limit
	if window > assertionReviewScanCap {
		window = assertionReviewScanCap
		queue.Truncated = true
	}

	baseFilter := map[string]interface{}{}
	applyReviewStatusFilter(baseFilter, status)
	if class := strings.TrimSpace(in.Class); class != "" {
		baseFilter["entity_type_kwd"] = []string{class}
	}
	if prop := strings.TrimSpace(in.Property); prop != "" {
		baseFilter["prop_kwd"] = []string{prop}
	}
	if src := strings.TrimSpace(in.SourceType); src != "" {
		baseFilter["from_type_kwd"] = []string{src}
	}
	if tgt := strings.TrimSpace(in.TargetType); tgt != "" {
		baseFilter["to_type_kwd"] = []string{tgt}
	}

	entityFilter := reviewKindFilter(baseFilter, "entity")
	entityRows, entityTotal, err := reviewSearch(ctx, in.TenantID, datasetIDs, graphEntityFields, entityFilter,
		(&types.OrderByExpr{}).Desc("mention_count_int"), 0, window)
	if err != nil {
		return nil, err
	}
	relationFilter := reviewKindFilter(baseFilter, "relation")
	relationRows, relationTotal, err := reviewSearch(ctx, in.TenantID, datasetIDs, graphRelationFields, relationFilter, nil, 0, window)
	if err != nil {
		return nil, err
	}
	queue.Total = entityTotal + relationTotal

	names := map[string]string{}
	for _, ds := range datasets {
		names[ds.ID] = ds.Name
	}

	entities := make([]StructureGraphNode, 0, len(entityRows))
	for _, row := range entityRows {
		if node := projectEntity(row); node != nil {
			if evidence := ontologyEvidenceOf(row); len(evidence) > 0 {
				node["evidence"] = evidence
			}
			annotateReviewRow(node, row, names)
			entities = append(entities, node)
		}
	}
	relations := make([]StructureGraphRelation, 0, len(relationRows))
	for _, row := range relationRows {
		if edge := projectRelation(row); edge != nil {
			if evidence := ontologyEvidenceOf(row); len(evidence) > 0 {
				edge["evidence"] = evidence
			}
			annotateReviewRow(edge, row, names)
			relations = append(relations, edge)
		}
	}
	// Conflict flags describe the window, not the corpus: detecting them globally
	// would mean a scan per page, and a reviewer reads conflicts on the page in
	// front of them (see markRelationConflicts).
	markEntityClassConflicts(entities)
	markRelationConflicts(relations)

	items := make([]ModelAssertionReviewItem, 0, len(entities)+len(relations))
	for _, node := range entities {
		items = append(items, reviewItemFromEntity(node))
	}
	for _, edge := range relations {
		items = append(items, reviewItemFromRelation(edge))
	}
	// Deterministic order: unresolved disagreements first (that is what a reviewer
	// is for), then by substance so the page does not reshuffle between requests.
	sortReviewItems(items)
	if len(items) > offset {
		end := offset + limit
		if end > len(items) {
			end = len(items)
		}
		queue.Items = items[offset:end]
	}
	if len(items) >= window {
		queue.Truncated = true
	}
	for _, item := range items {
		if item.Conflict {
			queue.ConflictCount++
		}
	}
	return queue, nil
}

// DecideAssertions records accept/reject/reset decisions on compiled assertions.
//
// Only rows that are this model's assertions in a dataset that compiles the model
// may be decided: the engine's update only enforces kb_id, so a caller-supplied id
// from another dataset or template would otherwise be writable through this path.
func (s *CompilationTemplateGovernanceService) DecideAssertions(ctx context.Context, db *gorm.DB, in ModelAssertionReviewActionInput) (*ModelAssertionReviewResult, error) {
	template, err := s.templateDAO.GetTemplate(ctx, db, in.TenantID, in.TemplateID)
	if err != nil {
		return nil, err
	}
	_, _, datasetIDs, err := reviewScopeDatasets(ctx, db, in.TenantID, template, nil)
	if err != nil {
		return nil, err
	}
	bound := map[string]bool{}
	for _, id := range datasetIDs {
		bound[id] = true
	}
	result := &ModelAssertionReviewResult{Failed: []ModelAssertionReviewFailure{}}
	if len(in.Items) > assertionReviewMaxBatch {
		return nil, fmt.Errorf("too many decisions in one request: %d (max %d)", len(in.Items), assertionReviewMaxBatch)
	}

	docEngine := engine.Get()
	if docEngine == nil {
		return nil, fmt.Errorf("document engine is not initialized")
	}
	actor := strings.TrimSpace(in.ActorID)
	now := float64(time.Now().UnixMilli())

	// Group by (dataset, action) so one engine update covers every id that shares
	// an outcome, and a decision that applies to several rows costs one call.
	type updateKey struct {
		datasetID string
		action    AssertionReviewStatus
		note      string
	}
	updates := map[updateKey][]string{}
	seen := map[string]bool{}
	for _, item := range in.Items {
		id := strings.TrimSpace(item.ID)
		datasetID := strings.TrimSpace(item.DatasetID)
		action := NormalizeAssertionReviewDecision(item.Action)
		switch {
		case id == "":
			result.Failed = append(result.Failed, ModelAssertionReviewFailure{ID: id, Reason: "missing row id"})
			continue
		case action == "":
			result.Failed = append(result.Failed, ModelAssertionReviewFailure{ID: id, Reason: "action must be accept, reject or reset"})
			continue
		case !bound[datasetID]:
			result.Failed = append(result.Failed, ModelAssertionReviewFailure{ID: id, Reason: "dataset does not compile this model"})
			continue
		case seen[datasetID+"\x00"+id]:
			continue
		}
		seen[datasetID+"\x00"+id] = true
		key := updateKey{datasetID: datasetID, action: action, note: strings.TrimSpace(item.Note)}
		updates[key] = append(updates[key], id)
	}

	// Sorted so the failure list and the update order do not depend on map
	// iteration.
	keys := make([]updateKey, 0, len(updates))
	for key := range updates {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].datasetID != keys[j].datasetID {
			return keys[i].datasetID < keys[j].datasetID
		}
		if keys[i].action != keys[j].action {
			return keys[i].action < keys[j].action
		}
		return keys[i].note < keys[j].note
	})

	for _, key := range keys {
		ids := updates[key]
		templateIDs := []string{template.ID}
		found, _, err := reviewSearch(ctx, in.TenantID, []string{key.datasetID}, []string{"id"},
			map[string]interface{}{
				"id":                       ids,
				"compilation_template_ids": templateIDs,
				"knowledge_graph_kwd":      []string{"entity", "relation"},
			}, nil, 0, len(ids))
		if err != nil {
			return nil, err
		}
		verified := map[string]bool{}
		for _, row := range found {
			verified[strings.TrimSpace(firstStringValue(row["id"]))] = true
		}
		applicable := make([]string, 0, len(ids))
		for _, id := range ids {
			if !verified[id] {
				result.Failed = append(result.Failed, ModelAssertionReviewFailure{
					ID: id, Reason: "row is not a compiled assertion of this model",
				})
				continue
			}
			applicable = append(applicable, id)
		}
		if len(applicable) == 0 {
			continue
		}
		update := map[string]interface{}{
			"review_status_kwd": string(key.action),
			"review_by_kwd":     actor,
			"review_note":       key.note,
		}
		if key.action == AssertionReviewUnreviewed {
			// A reset clears the decision instead of pretending someone judged it
			// just now.
			update["review_by_kwd"] = ""
			update["review_at_flt"] = 0.0
		} else {
			update["review_at_flt"] = now
		}
		if err := docEngine.UpdateChunks(ctx,
			map[string]interface{}{"id": applicable, "kb_id": key.datasetID},
			update, wikiIndexName(in.TenantID), key.datasetID); err != nil {
			for _, id := range applicable {
				result.Failed = append(result.Failed, ModelAssertionReviewFailure{ID: id, Reason: err.Error()})
			}
			continue
		}
		result.Updated += len(applicable)
	}

	counts, err := reviewCounts(ctx, in.TenantID, datasetIDs)
	if err != nil {
		return nil, err
	}
	result.Counts = counts
	return result, nil
}

// NormalizeAssertionReviewDecision maps an action onto the status it writes.
func NormalizeAssertionReviewDecision(action string) AssertionReviewStatus {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "accept", "accepted", "approve":
		return AssertionReviewAccepted
	case "reject", "rejected":
		return AssertionReviewRejected
	case "reset", "unreviewed", "clear":
		return AssertionReviewUnreviewed
	default:
		return ""
	}
}

// reviewScopeDatasets resolves the datasets a model is compiled by, capped and
// reported exactly like Inspect's fan-out (one cap, one meaning of truncated).
func reviewScopeDatasets(ctx context.Context, db *gorm.DB, tenantID string, template *entity.CompilationTemplate, wanted []string) ([]ModelInspectDataset, []string, []string, error) {
	refs := modelRefsFor(template)
	bound, err := dao.NewDatasetModelRefDAO().DatasetsByModelRefs(ctx, db, tenantID, refs)
	if err != nil {
		return nil, nil, nil, err
	}
	datasets, skipped := inspectSelectDatasets(bound, ModelInspectScopeCorpus, wanted)
	ids := make([]string, 0, len(datasets))
	for _, ds := range datasets {
		ids = append(ids, ds.ID)
	}
	return datasets, skipped, ids, nil
}

// reviewCounts counts the three buckets with one query each. Counting is done
// per bucket instead of deriving "unreviewed = total - decided" so a row whose
// status was written as an unknown value cannot silently land in the queue.
func reviewCounts(ctx context.Context, tenantID string, datasetIDs []string) (ModelAssertionReviewCounts, error) {
	counts := ModelAssertionReviewCounts{}
	for _, bucket := range []struct {
		status AssertionReviewStatus
		into   *int
	}{
		{AssertionReviewUnreviewed, &counts.Unreviewed},
		{AssertionReviewAccepted, &counts.Accepted},
		{AssertionReviewRejected, &counts.Rejected},
	} {
		filter := map[string]interface{}{"knowledge_graph_kwd": []string{"entity", "relation"}}
		applyReviewStatusFilter(filter, bucket.status)
		_, total, err := reviewSearch(ctx, tenantID, datasetIDs, []string{"id"}, filter, nil, 0, 1)
		if err != nil {
			return counts, err
		}
		*bucket.into = int(total)
	}
	return counts, nil
}

// applyReviewStatusFilter narrows a filter to one review bucket.
//
// "Unreviewed" is expressed as "not accepted and not rejected" rather than as a
// missing column: rows compiled before review existed have no such column (ES),
// rows written for a newer table have an empty one (Infinity), and a reset writes
// the explicit value — the negative form is the one filter that treats all three
// as the same state.
func applyReviewStatusFilter(filter map[string]interface{}, status AssertionReviewStatus) {
	switch status {
	case AssertionReviewAccepted:
		filter["review_status_kwd"] = []string{string(AssertionReviewAccepted)}
	case AssertionReviewRejected:
		filter["review_status_kwd"] = []string{string(AssertionReviewRejected)}
	case AssertionReviewAll:
	default:
		filter["must_not"] = map[string]interface{}{
			"review_status_kwd": []string{string(AssertionReviewAccepted), string(AssertionReviewRejected)},
		}
	}
}

// reviewKindFilter pins one row kind on top of the caller's filters.
func reviewKindFilter(base map[string]interface{}, kind string) map[string]interface{} {
	out := make(map[string]interface{}, len(base)+1)
	for k, v := range base {
		out[k] = v
	}
	out["knowledge_graph_kwd"] = []string{kind}
	return out
}

// reviewSearch runs one raw-row search across several datasets in a single engine
// query: a tenant's rows share one index and are partitioned by kb_id, so the
// per-dataset fan-out used elsewhere is unnecessary here.
func reviewSearch(ctx context.Context, tenantID string, datasetIDs []string, selectFields []string, filter map[string]interface{}, orderBy *types.OrderByExpr, offset, limit int) ([]map[string]interface{}, int64, error) {
	docEngine := engine.Get()
	if docEngine == nil {
		return nil, 0, fmt.Errorf("document engine is not initialized")
	}
	if limit < 1 {
		limit = 1
	}
	res, err := docEngine.Search(ctx, &types.SearchRequest{
		IndexNames:   []string{wikiIndexName(tenantID)},
		KbIDs:        datasetIDs,
		Offset:       offset,
		Limit:        limit,
		SelectFields: selectFields,
		Filter:       filter,
		OrderBy:      orderBy,
	})
	if err != nil {
		return nil, 0, err
	}
	if res == nil {
		return nil, 0, nil
	}
	return res.Chunks, res.Total, nil
}

// annotateReviewRow records which dataset a row came from. A model spans
// datasets, so every item must say which one it is a claim about; the name rides
// along so the UI does not have to resolve ids it already received.
func annotateReviewRow(node map[string]interface{}, row map[string]interface{}, names map[string]string) {
	datasetID := strings.TrimSpace(firstStringValue(row["kb_id"]))
	if datasetID == "" {
		return
	}
	node["dataset_id"] = datasetID
	if name := names[datasetID]; name != "" {
		node["dataset_name"] = name
	}
}

func reviewItemFromEntity(node StructureGraphNode) ModelAssertionReviewItem {
	item := ModelAssertionReviewItem{
		ID:             graphStr(node["id"]),
		DatasetID:      graphStr(node["dataset_id"]),
		DatasetName:    graphStr(node["dataset_name"]),
		Kind:           "entity",
		Scope:          graphStr(node["scope"]),
		Name:           graphStr(node["name"]),
		Type:           graphStr(node["type"]),
		Description:    graphStr(node["description"]),
		Version:        reviewInt64(node["version"]),
		ReviewStatus:   graphStr(node["review_status"]),
		ReviewBy:       graphStr(node["review_by"]),
		ReviewNote:     graphStr(node["review_note"]),
		SourceChunkIDs: stringSliceValue(node["source_chunk_ids"]),
		DocumentIDs:    stringSliceValue(node["document_ids"]),
	}
	item.MentionCount = int(reviewInt64(node["mention_count"]))
	item.ReviewAt = reviewInt64(node["review_at"])
	if attrs, ok := node["attributes"].(map[string]interface{}); ok {
		item.Attributes = attrs
	}
	if evidence, ok := node["evidence"].([]map[string]interface{}); ok {
		item.Evidence = evidence
	}
	item.Conflict, _ = node["conflict"].(bool)
	item.ConflictTypes = stringSliceValue(node["conflict_types"])
	return item
}

func reviewItemFromRelation(node StructureGraphRelation) ModelAssertionReviewItem {
	item := ModelAssertionReviewItem{
		ID:             graphStr(node["id"]),
		DatasetID:      graphStr(node["dataset_id"]),
		DatasetName:    graphStr(node["dataset_name"]),
		Kind:           "relation",
		Scope:          graphStr(node["scope"]),
		From:           graphStr(node["from"]),
		To:             graphStr(node["to"]),
		Property:       graphStr(node["property"]),
		Type:           graphStr(node["type"]),
		SourceType:     graphStr(node["source_type"]),
		TargetType:     graphStr(node["target_type"]),
		Version:        reviewInt64(node["version"]),
		ReviewStatus:   graphStr(node["review_status"]),
		ReviewBy:       graphStr(node["review_by"]),
		ReviewNote:     graphStr(node["review_note"]),
		SourceChunkIDs: stringSliceValue(node["source_chunk_ids"]),
		DocumentIDs:    stringSliceValue(node["document_ids"]),
	}
	item.ReviewAt = reviewInt64(node["review_at"])
	if evidence, ok := node["evidence"].([]map[string]interface{}); ok {
		item.Evidence = evidence
	}
	item.Conflict, _ = node["conflict"].(bool)
	item.ConflictTargets = stringSliceValue(node["conflict_targets"])
	return item
}

// sortReviewItems puts the rows that need a decision first: an unresolved
// disagreement, then the assertions with more evidence behind them (a claim
// mentioned twice is the one worth reading), then by identity so paging is stable.
func sortReviewItems(items []ModelAssertionReviewItem) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Conflict != b.Conflict {
			return a.Conflict
		}
		if a.MentionCount != b.MentionCount {
			return a.MentionCount > b.MentionCount
		}
		if len(a.Evidence) != len(b.Evidence) {
			return len(a.Evidence) > len(b.Evidence)
		}
		left, right := reviewItemSortKey(a), reviewItemSortKey(b)
		if left != right {
			return left < right
		}
		return a.ID < b.ID
	})
}

func reviewItemSortKey(item ModelAssertionReviewItem) string {
	if item.Kind == "relation" {
		return "relation\x00" + strings.ToLower(item.From) + "\x00" + strings.ToLower(item.Property) + "\x00" + strings.ToLower(item.To)
	}
	return "entity\x00" + strings.ToLower(item.Name) + "\x00" + strings.ToLower(item.Type)
}

func reviewInt64(v interface{}) int64 {
	if f, ok := graphFloat(v); ok {
		return int64(f)
	}
	return 0
}
