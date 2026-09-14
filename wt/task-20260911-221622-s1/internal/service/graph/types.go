package graph

// KGEntity represents a knowledge graph entity.
type KGEntity struct {
	Name        string       // entity_kwd
	Type        string       // entity_type_kwd
	PageRank    float64      // rank_flt
	Similarity  float64      // _score
	Description string       // content_with_weight
	NhopEnts    []NhopEntity // n_hop_with_weight (parsed JSON)
}

// NhopEntity represents an N-hop neighbor path.
type NhopEntity struct {
	Path    []string  // entity names along the path
	Weights []float64 // pagerank weights per hop
}

// KGRelation represents a relation between two entities.
type KGRelation struct {
	From        string  // from_entity_kwd
	To          string  // to_entity_kwd
	Description string  // content_with_weight
	Sim         float64 // score accumulated during pipeline scoring
	PageRank    float64 // rank_flt or weight_int as float64
}

// Edge represents a directed (from_entity, to_entity) pair.
type Edge struct {
	From, To string
}

// EdgeScore represents the accumulated score for an edge from N-hop analysis.
type EdgeScore struct {
	Sim      float64
	PageRank float64
}

// ScoredEntity is a scored entity ready for output.
type ScoredEntity struct {
	Entity      string
	Score       float64
	Description string
}

// ScoredRelation is a scored relation ready for output.
type ScoredRelation struct {
	From        string
	To          string
	Score       float64
	Description string
}

// KGCommunityReport represents a community report.
type KGCommunityReport struct {
	Title    string  // docnm_kwd
	Content  string  // content_with_weight
	Weight   float64 // weight_flt
	Entities string  // entities_kwd
}
