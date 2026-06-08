package common

const (
	// PAGERANK_FLD is the field name for pagerank score
	PAGERANK_FLD = "pagerank_fea"
	// TAG_FLD is the field name for tag features
	TAG_FLD = "tag_feas"
	// MAX_RESULT_WINDOW is the maximum result window for ES
	MAX_RESULT_WINDOW = 10000
	// SearchAfterBatchSize caps how many hits one Elasticsearch
	// request can return per search_after iteration.
	SearchAfterBatchSize = 1000
)
