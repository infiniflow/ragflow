package service

import (
	"ragflow/internal/dao"
)

// TenantService tenant service
type TenantService struct {
	tenantDAO *dao.TenantDAO
}

// NewTenantService create tenant service
func NewTenantService() *TenantService {
	return &TenantService{
		tenantDAO: dao.NewTenantDAO(),
	}
}

// TenantInfoResponse tenant information response
type TenantInfoResponse struct {
	TenantID  string  `json:"tenant_id"`
	Name      *string `json:"name,omitempty"`
	LLMID     string  `json:"llm_id"`
	EmbDID    string  `json:"embd_id"`
	RerankID  string  `json:"rerank_id"`
	ASRID     string  `json:"asr_id"`
	Img2TxtID string  `json:"img2txt_id"`
	TTSID     *string `json:"tts_id,omitempty"`
	ParserIDs string  `json:"parser_ids"`
	Role      string  `json:"role"`
}

// GetTenantInfo get tenant information for the current user (owner tenant)
func (s *TenantService) GetTenantInfo(userID string) (*TenantInfoResponse, error) {
	tenantInfos, err := s.tenantDAO.GetInfoByUserID(userID)
	if err != nil {
		return nil, err
	}
	if len(tenantInfos) == 0 {
		return nil, nil // No tenant found (should not happen for valid user)
	}
	// Return the first tenant (should be only one owner tenant per user)
	ti := tenantInfos[0]
	return &TenantInfoResponse{
		TenantID:  ti.TenantID,
		Name:      ti.Name,
		LLMID:     ti.LLMID,
		EmbDID:    ti.EmbDID,
		RerankID:  ti.RerankID,
		ASRID:     ti.ASRID,
		Img2TxtID: ti.Img2TxtID,
		TTSID:     ti.TTSID,
		ParserIDs: ti.ParserIDs,
		Role:      ti.Role,
	}, nil
}