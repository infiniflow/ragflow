package service

import (
	"ragflow/internal/dao"
)

// LLMService LLM service
type LLMService struct {
	tenantLLMDAO *dao.TenantLLMDAO
}

// NewLLMService create LLM service
func NewLLMService() *LLMService {
	return &LLMService{
		tenantLLMDAO: dao.NewTenantLLMDAO(),
	}
}

// MyLLMItem represents a single LLM item in the response
type MyLLMItem struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	UsedToken int64  `json:"used_token"`
	Status    string `json:"status"`
	APIBase   string `json:"api_base,omitempty"`
	MaxTokens int64  `json:"max_tokens,omitempty"`
}

// MyLLMResponse represents the response structure for my LLMs
type MyLLMResponse struct {
	Tags string      `json:"tags"`
	LLM  []MyLLMItem `json:"llm"`
}

// GetMyLLMs get my LLMs for a tenant
func (s *LLMService) GetMyLLMs(tenantID string, includeDetails bool) (map[string]MyLLMResponse, error) {
	// Get LLM list from database
	myLLMs, err := s.tenantLLMDAO.GetMyLLMs(tenantID, includeDetails)
	if err != nil {
		return nil, err
	}

	// Group by factory
	result := make(map[string]MyLLMResponse)
	for _, llm := range myLLMs {
		// Get or create factory entry
		resp, exists := result[llm.LLMFactory]
		if !exists {
			resp = MyLLMResponse{
				Tags: llm.Tags,
				LLM:  []MyLLMItem{},
			}
		}

		// Create LLM item
		item := MyLLMItem{
			Type:      llm.ModelType,
			Name:      llm.LLMName,
			UsedToken: llm.UsedTokens,
			Status:    llm.Status,
		}
		
		// Add detailed fields if requested
		if includeDetails {
			item.APIBase = llm.APIBase
			item.MaxTokens = llm.MaxTokens
		}
		
		resp.LLM = append(resp.LLM, item)
		result[llm.LLMFactory] = resp
	}

	return result, nil
}