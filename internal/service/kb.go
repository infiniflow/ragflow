package service

import (
	"ragflow/internal/dao"
	"ragflow/internal/model"
)

// KnowledgebaseService knowledge base service
type KnowledgebaseService struct {
	kbDAO     *dao.KnowledgebaseDAO
	tenantDAO  *dao.TenantDAO
}

// NewKnowledgebaseService create knowledge base service
func NewKnowledgebaseService() *KnowledgebaseService {
	return &KnowledgebaseService{
		kbDAO:     dao.NewKnowledgebaseDAO(),
		tenantDAO:  dao.NewTenantDAO(),
	}
}

// ListKbsRequest list knowledge bases request
type ListKbsRequest struct {
	Keywords  *string  `json:"keywords,omitempty"`
	Page      *int     `json:"page,omitempty"`
	PageSize  *int     `json:"page_size,omitempty"`
	ParserID *string  `json:"parser_id,omitempty"`
	Orderby   *string  `json:"orderby,omitempty"`
	Desc      *bool    `json:"desc,omitempty"`
	OwnerIDs *[]string `json:"owner_ids,omitempty"`
}

// ListKbsResponse list knowledge bases response
type ListKbsResponse struct {
	KBs   []*model.Knowledgebase `json:"kbs"`
	Total int64                  `json:"total"`
}

// ListKbs list knowledge bases
func (s *KnowledgebaseService) ListKbs(req *ListKbsRequest, userID string) (*ListKbsResponse, error) {
	// Set defaults
	keywords := ""
	if req.Keywords != nil {
		keywords = *req.Keywords
	}

	page := 0
	if req.Page != nil {
		page = *req.Page
	}

	pageSize := 0
	if req.PageSize != nil {
		pageSize = *req.PageSize
	}

	parserID := ""
	if req.ParserID != nil {
		parserID = *req.ParserID
	}

	orderby := "update_time"
	if req.Orderby != nil {
		orderby = *req.Orderby
	}

	desc := true
	if req.Desc != nil {
		desc = *req.Desc
	}

	var kbs []*model.Knowledgebase
	var total int64
	var err error

	// If owner IDs are provided, filter by them
	if req.OwnerIDs != nil && len(*req.OwnerIDs) > 0 {
		kbs, total, err = s.kbDAO.ListByOwnerIDs(*req.OwnerIDs, page, pageSize, orderby, desc, keywords, parserID)
	} else {
		// Get joined tenants by user ID
		tenants, err := s.tenantDAO.GetJoinedTenantsByUserID(userID)
		if err != nil {
			return nil, err
		}

		// Extract tenant IDs
		tenantIDs := make([]string, len(tenants))
		for i, t := range tenants {
			tenantIDs[i] = t.TenantID
		}

		kbs, total, err = s.kbDAO.ListByTenantIDs(tenantIDs, userID, page, pageSize, orderby, desc, keywords, parserID)
	}

	if err != nil {
		return nil, err
	}

	return &ListKbsResponse{
		KBs:   kbs,
		Total: total,
	}, nil
}
