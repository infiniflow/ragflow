package service

import (
	"ragflow/internal/dao"
	"ragflow/internal/model"
)

// ChatService chat service
type ChatService struct {
	chatDAO       *dao.ChatDAO
	kbDAO         *dao.KnowledgebaseDAO
	userTenantDAO *dao.UserTenantDAO
}

// NewChatService create chat service
func NewChatService() *ChatService {
	return &ChatService{
		chatDAO:       dao.NewChatDAO(),
		kbDAO:         dao.NewKnowledgebaseDAO(),
		userTenantDAO: dao.NewUserTenantDAO(),
	}
}

// ChatWithKBNames chat with knowledge base names
type ChatWithKBNames struct {
	*model.Chat
	KBNames []string `json:"kb_names"`
}

// ListChatsResponse list chats response
type ListChatsResponse struct {
	Chats []*ChatWithKBNames `json:"chats"`
}

// ListChats list chats for a user
func (s *ChatService) ListChats(userID string, status string) (*ListChatsResponse, error) {
	// Get tenant IDs by user ID
	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return nil, err
	}

	// For now, use the first tenant ID (primary tenant)
	// This matches the Python implementation behavior
	var tenantID string
	if len(tenantIDs) > 0 {
		tenantID = tenantIDs[0]
	} else {
		tenantID = userID
	}

	// Query chats by tenant ID
	chats, err := s.chatDAO.ListByTenantID(tenantID, status)
	if err != nil {
		return nil, err
	}

	// Enrich with knowledge base names
	var chatsWithKBNames []*ChatWithKBNames
	for _, chat := range chats {
		kbNames := s.getKBNames(chat.KBIDs)
		chatsWithKBNames = append(chatsWithKBNames, &ChatWithKBNames{
			Chat:    chat,
			KBNames: kbNames,
		})
	}

	return &ListChatsResponse{
		Chats: chatsWithKBNames,
	}, nil
}

// ListChatsNextRequest list chats next request
type ListChatsNextRequest struct {
	OwnerIDs []string `json:"owner_ids,omitempty"`
}

// ListChatsNextResponse list chats next response
type ListChatsNextResponse struct {
	Chats []*ChatWithKBNames `json:"dialogs"`
	Total int64              `json:"total"`
}

// ListChatsNext list chats with advanced filtering (equivalent to list_dialogs_next)
func (s *ChatService) ListChatsNext(userID string, keywords string, page, pageSize int, orderby string, desc bool, ownerIDs []string) (*ListChatsNextResponse, error) {
	var chats []*model.Chat
	var total int64
	var err error

	if len(ownerIDs) == 0 {
		// Get tenant IDs by user ID (joined tenants)
		tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
		if err != nil {
			return nil, err
		}

		// Use database pagination
		chats, total, err = s.chatDAO.ListByTenantIDs(tenantIDs, userID, page, pageSize, orderby, desc, keywords)
		if err != nil {
			return nil, err
		}
	} else {
		// Filter by owner IDs, manual pagination
		chats, total, err = s.chatDAO.ListByOwnerIDs(ownerIDs, userID, orderby, desc, keywords)
		if err != nil {
			return nil, err
		}

		// Manual pagination
		if page > 0 && pageSize > 0 {
			start := (page - 1) * pageSize
			end := start + pageSize
			if start < int(total) {
				if end > int(total) {
					end = int(total)
				}
				chats = chats[start:end]
			} else {
				chats = []*model.Chat{}
			}
		}
	}

	// Enrich with knowledge base names
	var chatsWithKBNames []*ChatWithKBNames
	for _, chat := range chats {
		kbNames := s.getKBNames(chat.KBIDs)
		chatsWithKBNames = append(chatsWithKBNames, &ChatWithKBNames{
			Chat:    chat,
			KBNames: kbNames,
		})
	}

	return &ListChatsNextResponse{
		Chats: chatsWithKBNames,
		Total: total,
	}, nil
}

// getKBNames gets knowledge base names by IDs
func (s *ChatService) getKBNames(kbIDs model.JSONSlice) []string {
	var names []string
	for _, kbID := range kbIDs {
		kbIDStr, ok := kbID.(string)
		if !ok {
			continue
		}
		kb, err := s.kbDAO.GetByID(kbIDStr)
		if err != nil || kb == nil {
			continue
		}
		// Only include valid KBs
		if kb.Status != nil && *kb.Status == "1" {
			names = append(names, kb.Name)
		}
	}
	return names
}
