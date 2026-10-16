package handler

import (
	"ragflow/internal/common"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

// OntologyABoxRetrievalHandler serves the standalone compiled-rows (ABox)
// retrieval API: the assertions that fit the structure a template declares.
//
// It is deliberately not a mode of /v1/retrieval. The main pipeline answers a
// question over the corpus; this endpoint answers which compiled assertions
// match a stated structure, and other retrievals — the agentic loop among them —
// call the service behind it instead of growing the main pipeline one more flag.
type OntologyABoxRetrievalHandler struct {
	service *service.OntologyABoxRetrievalService
}

func NewOntologyABoxRetrievalHandler(svc *service.OntologyABoxRetrievalService) *OntologyABoxRetrievalHandler {
	return &OntologyABoxRetrievalHandler{service: svc}
}

// ontologyABoxRetrievalBody is the JSON body. Every structure field is explicit
// input: nothing here is derived from a model's reading of the question's prose,
// so a request cannot narrow retrieval to whatever the reader happened to say.
type ontologyABoxRetrievalBody struct {
	DatasetIDs          []string `json:"dataset_ids" binding:"required"`
	DocumentIDs         []string `json:"document_ids,omitempty"`
	Question            string   `json:"question" binding:"required"`
	OntologyTemplateID  string   `json:"ontology_template_id,omitempty"`
	OntologyVersion     int      `json:"ontology_version,omitempty"`
	EntityTypes         []string `json:"entity_types,omitempty"`
	Properties          []string `json:"properties,omitempty"`
	FromTypes           []string `json:"from_types,omitempty"`
	ToTypes             []string `json:"to_types,omitempty"`
	TopK                int      `json:"top_k,omitempty"`
	SimilarityThreshold float64  `json:"similarity_threshold,omitempty"`
	// DocumentScope is the caller's readable document set, used to count which
	// assertions' quotations may be cited. Absent means unrestricted.
	DocumentScope []string `json:"document_scope,omitempty"`
	// SkipUnreviewed drops pending assertions. The default keeps them, tagged:
	// a model whose latest compile is unreviewed would otherwise be invisible.
	SkipUnreviewed bool `json:"skip_unreviewed,omitempty"`
}

// Retrieve answers with the compiled rows that fit the requested structure. The
// applied filter travels back with them, expansion included: a caller who asked
// for an abstract class needs to see the descendants it actually searched.
//
// The response also grades the evidence it returns. A model that cannot be read,
// or one that matches no assertion, still returns rows rather than an error —
// narrowed rows are graded structured and everything else schema_free, with
// structure_applied=false and a degrade_code naming why the narrowing was
// dropped. Callers render those differently: an unreadable model is a model to
// fix, an unmatched one means the model's coverage is short, and schema_free
// rows may be used as leads but not attributed to the model.
//
//	@Summary Retrieve compiled ontology assertions matching a declared structure
//	@Tags ontology
//	@Security ApiKeyAuth
//	@Accept json
//	@Produce json
//	@Param request body ontologyABoxRetrievalBody true "structure constraints and scope"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/retrieval/ontology [post]
func (h *OntologyABoxRetrievalHandler) Retrieve(c *gin.Context) {
	user, code, msg := GetUser(c)
	if user == nil {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	var body ontologyABoxRetrievalBody
	if err := c.ShouldBindJSON(&body); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Invalid request: "+err.Error())
		return
	}
	result, err := h.service.Retrieve(c.Request.Context(), &service.OntologyABoxRetrievalRequest{
		UserID:              user.ID,
		DatasetIDs:          body.DatasetIDs,
		DocumentIDs:         body.DocumentIDs,
		Question:            body.Question,
		OntologyTemplateID:  body.OntologyTemplateID,
		OntologyVersion:     body.OntologyVersion,
		EntityTypes:         body.EntityTypes,
		Properties:          body.Properties,
		FromTypes:           body.FromTypes,
		ToTypes:             body.ToTypes,
		TopK:                body.TopK,
		SimilarityThreshold: body.SimilarityThreshold,
		DocumentScope:       body.DocumentScope,
		SkipUnreviewed:      body.SkipUnreviewed,
	})
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}
	// The grade is read off the digest, which owns it: a second field on the
	// result would be a copy that can disagree with the receipt beside it.
	evidenceLevel := ""
	if result.Digest != nil {
		evidenceLevel = result.Digest.EvidenceLevel
	}
	common.SuccessWithData(c, gin.H{
		"chunks":            result.Chunks,
		"total":             result.Total,
		"filter":            result.Filter,
		"evidence_level":    evidenceLevel,
		"structure_applied": result.StructureApplied,
		"degrade_code":      result.DegradeCode,
		"degrade_reason":    result.DegradeReason,
		"model": gin.H{
			"id":             result.Model.ID,
			"name":           result.Model.Name,
			"version":        result.Model.Version,
			"publish_status": result.Model.PublishStatus,
		},
		// The constraints derived from the question when the caller gave none.
		// They are echoed so the derivation is arguable: a reader who sees the
		// wrong class matched can say so, which a hidden derivation would not
		// allow.
		"suggested_constraints": result.SuggestedConstraints,
		"digest":                result.Digest,
	}, "success")
}
