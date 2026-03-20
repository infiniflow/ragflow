//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package service

import (
	"context"
	"fmt"

	"ragflow/internal/engine"
	"ragflow/internal/engine/elasticsearch"
)

// indexName generates the index name for a given tenant ID
// The index name format is: memory_{tenant_id}
//
// Parameters:
//   - tenantID: The tenant identifier
//
// Returns:
//   - string: The index name in format "memory_{tenant_id}"
//
// Example:
//
//	indexName("tenant123") returns "memory_tenant123"
func indexName(tenantID string) string {
	return fmt.Sprintf("memory_%s", tenantID)
}

// MessageService handles message operations for memory storage
// It provides methods for managing messages in the document engine
type MessageService struct{}

// NewMessageService creates a new MessageService instance
//
// Returns:
//   - *MessageService: Initialized message service instance
//
// Example:
//
//	svc := NewMessageService()
func NewMessageService() *MessageService {
	return &MessageService{}
}

// HasIndex checks if an index exists for the given memory
//
// Parameters:
//   - tenantID: The tenant identifier
//   - memoryID: The memory identifier
//
// Returns:
//   - bool: true if the index exists, false otherwise
//   - error: Error if the check fails
//
// Example:
//
//	exists, err := svc.HasIndex("tenant123", "mem456")
//	if exists {
//	    // Index exists
//	}
func (s *MessageService) HasIndex(tenantID string, memoryID string) (bool, error) {
	ctx := context.Background()
	index := indexName(tenantID)

	eng := engine.Get()
	if eng == nil {
		return false, fmt.Errorf("document engine not initialized")
	}

	// Check if index exists
	exists, err := eng.IndexExists(ctx, index)
	if err != nil {
		return false, fmt.Errorf("failed to check index existence: %w", err)
	}

	if !exists {
		return false, nil
	}

	// Check if there are documents for this memory_id
	// Use a simple search to check if memory has any documents
	searchReq := &elasticsearch.SearchRequest{
		IndexNames: []string{index},
		Filters: map[string]interface{}{
			"kb_id": memoryID, // memory_id is stored as kb_id in the document
		},
		Size: 1,
	}

	resp, err := eng.Search(ctx, searchReq)
	if err != nil {
		return false, nil
	}

	if searchResp, ok := resp.(*elasticsearch.SearchResponse); ok {
		return searchResp.Hits.Total.Value > 0, nil
	}

	return false, nil
}

// DeleteMessage deletes messages matching the given filter condition
//
// Parameters:
//   - filter: The filter condition to match messages for deletion
//   - tenantID: The tenant identifier
//   - memoryID: The memory identifier
//
// Returns:
//   - int64: Number of documents deleted
//   - error: Error if the deletion fails
//
// Example:
//
//	filter := map[string]interface{}{
//	    "term": map[string]interface{}{"message_id": 123},
//	}
//	count, err := svc.DeleteMessage(filter, "tenant123", "mem456")
func (s *MessageService) DeleteMessage(filter map[string]interface{}, tenantID string, memoryID string) (int64, error) {
	ctx := context.Background()
	index := indexName(tenantID)

	eng := engine.Get()
	if eng == nil {
		return 0, fmt.Errorf("document engine not initialized")
	}

	// Build query with memory_id filter
	query := s.buildQueryWithMemoryID(filter, memoryID)

	count, err := eng.DeleteByQuery(ctx, index, query)
	if err != nil {
		return 0, fmt.Errorf("failed to delete messages: %w", err)
	}

	return count, nil
}

// UpdateMessage updates messages matching the given filter condition
//
// Parameters:
//   - filter: The filter condition to match messages for update
//   - updateDict: The fields to update
//   - tenantID: The tenant identifier
//   - memoryID: The memory identifier
//
// Returns:
//   - int64: Number of documents updated
//   - error: Error if the update fails
//
// Example:
//
//	filter := map[string]interface{}{
//	    "term": map[string]interface{}{"message_id": 123},
//	}
//	updateDict := map[string]interface{}{"status": 1}
//	count, err := svc.UpdateMessage(filter, updateDict, "tenant123", "mem456")
func (s *MessageService) UpdateMessage(filter map[string]interface{}, updateDict map[string]interface{}, tenantID string, memoryID string) (int64, error) {
	ctx := context.Background()
	index := indexName(tenantID)

	eng := engine.Get()
	if eng == nil {
		return 0, fmt.Errorf("document engine not initialized")
	}

	// Convert status from bool to int if present
	if status, ok := updateDict["status"]; ok {
		if statusBool, ok := status.(bool); ok {
			if statusBool {
				updateDict["status"] = 1
			} else {
				updateDict["status"] = 0
			}
		}
	}

	// Build query with memory_id filter
	query := s.buildQueryWithMemoryID(filter, memoryID)

	count, err := eng.UpdateByQuery(ctx, index, query, updateDict)
	if err != nil {
		return 0, fmt.Errorf("failed to update messages: %w", err)
	}

	return count, nil
}

// GetByMessageID retrieves a message by its ID
//
// Parameters:
//   - memoryID: The memory identifier
//   - messageID: The message identifier
//   - tenantID: The tenant identifier
//
// Returns:
//   - map[string]interface{}: The message document
//   - error: Error if the retrieval fails
//
// Example:
//
//	msg, err := svc.GetByMessageID("mem456", 123, "tenant123")
//	if msg != nil {
//	    content := msg["content"]
//	}
func (s *MessageService) GetByMessageID(memoryID string, messageID int64, tenantID string) (map[string]interface{}, error) {
	ctx := context.Background()
	index := indexName(tenantID)
	docID := fmt.Sprintf("%s_%d", memoryID, messageID)

	eng := engine.Get()
	if eng == nil {
		return nil, fmt.Errorf("document engine not initialized")
	}

	doc, err := eng.GetDocument(ctx, index, docID)
	if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}

	if doc == nil {
		return nil, fmt.Errorf("message not found")
	}

	if docMap, ok := doc.(map[string]interface{}); ok {
		return docMap, nil
	}

	return nil, fmt.Errorf("invalid document format")
}

// GetRecentMessages retrieves recent messages from multiple memories
//
// Parameters:
//   - tenantIDs: List of tenant identifiers
//   - memoryIDs: List of memory identifiers
//   - agentID: The agent identifier to filter by
//   - sessionID: The session identifier to filter by
//   - limit: Maximum number of messages to retrieve
//
// Returns:
//   - []map[string]interface{}: List of message documents
//   - error: Error if the retrieval fails
//
// Example:
//
//	messages, err := svc.GetRecentMessages(
//	    []string{"tenant123", "tenant456"},
//	    []string{"mem1", "mem2"},
//	    "agent789",
//	    "session012",
//	    10,
//	)
func (s *MessageService) GetRecentMessages(tenantIDs []string, memoryIDs []string, agentID string, sessionID string, limit int) ([]map[string]interface{}, error) {
	ctx := context.Background()

	eng := engine.Get()
	if eng == nil {
		return nil, fmt.Errorf("document engine not initialized")
	}

	// Build index names from tenant IDs
	indexNames := make([]string, len(tenantIDs))
	for i, tenantID := range tenantIDs {
		indexNames[i] = indexName(tenantID)
	}

	// Build filter conditions
	mustClauses := []map[string]interface{}{}

	if agentID != "" {
		mustClauses = append(mustClauses, map[string]interface{}{
			"term": map[string]interface{}{"agent_id": agentID},
		})
	}

	if sessionID != "" {
		mustClauses = append(mustClauses, map[string]interface{}{
			"term": map[string]interface{}{"session_id": sessionID},
		})
	}

	// Build search request
	searchReq := &elasticsearch.SearchRequest{
		IndexNames: indexNames,
		Query: map[string]interface{}{
			"bool": map[string]interface{}{
				"must":   mustClauses,
				"filter": []map[string]interface{}{{"terms": map[string]interface{}{"kb_id": memoryIDs}}},
			},
		},
		Size: limit,
		Sort: []interface{}{
			map[string]interface{}{"valid_at": map[string]interface{}{"order": "desc"}},
		},
		Source: []string{
			"message_id", "message_type", "source_id", "memory_id",
			"user_id", "agent_id", "session_id", "valid_at",
			"invalid_at", "forget_at", "status", "content",
		},
	}

	resp, err := eng.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("failed to search messages: %w", err)
	}

	var results []map[string]interface{}
	if searchResp, ok := resp.(*elasticsearch.SearchResponse); ok {
		for _, hit := range searchResp.Hits.Hits {
			results = append(results, hit.Source)
		}
	}

	if results == nil {
		results = []map[string]interface{}{}
	}

	return results, nil
}

// buildQueryWithMemoryID builds an Elasticsearch query that includes the memory_id filter
//
// Parameters:
//   - filter: The original filter condition
//   - memoryID: The memory identifier to add to the filter
//
// Returns:
//   - map[string]interface{}: The combined query with memory_id filter
//
// Example:
//
//	query := s.buildQueryWithMemoryID(
//	    map[string]interface{}{"term": map[string]interface{}{"status": 1}},
//	    "mem456",
//	)
func (s *MessageService) buildQueryWithMemoryID(filter map[string]interface{}, memoryID string) map[string]interface{} {
	// Build bool query with memory_id filter
	boolQuery := map[string]interface{}{
		"filter": []map[string]interface{}{
			{
				"term": map[string]interface{}{"kb_id": memoryID},
			},
		},
	}

	// Add original filter conditions
	if filter != nil {
		for key, value := range filter {
			boolQuery["filter"] = append(boolQuery["filter"].([]map[string]interface{}), map[string]interface{}{
				key: value,
			})
		}
	}

	return map[string]interface{}{
		"bool": boolQuery,
	}
}

// InsertMessage inserts messages into the memory storage
//
// Parameters:
//   - messages: List of message documents to insert
//   - tenantID: The tenant identifier
//   - memoryID: The memory identifier
//
// Returns:
//   - error: Error if the insertion fails
//
// Example:
//
//	messages := []map[string]interface{}{
//	    {"message_id": 1, "content": "Hello", "status": true},
//	    {"message_id": 2, "content": "World", "status": true},
//	}
//	err := svc.InsertMessage(messages, "tenant123", "mem456")
func (s *MessageService) InsertMessage(messages []map[string]interface{}, tenantID string, memoryID string) error {
	ctx := context.Background()
	index := indexName(tenantID)

	eng := engine.Get()
	if eng == nil {
		return fmt.Errorf("document engine not initialized")
	}

	// Prepare messages with ID and status conversion
	docs := make([]interface{}, len(messages))
	for i, msg := range messages {
		// Create a copy of the message
		msgCopy := make(map[string]interface{})
		for k, v := range msg {
			msgCopy[k] = v
		}

		// Set document ID
		if messageID, ok := msg["message_id"]; ok {
			msgCopy["_id"] = fmt.Sprintf("%s_%v", memoryID, messageID)
		}

		// Convert status from bool to int
		if status, ok := msg["status"]; ok {
			if statusBool, ok := status.(bool); ok {
				if statusBool {
					msgCopy["status"] = 1
				} else {
					msgCopy["status"] = 0
				}
			}
		}

		docs[i] = msgCopy
	}

	_, err := eng.BulkIndex(ctx, index, docs)
	if err != nil {
		return fmt.Errorf("failed to insert messages: %w", err)
	}

	return nil
}

// ListMessage retrieves messages with pagination and filters
//
// Parameters:
//   - tenantID: The tenant identifier
//   - memoryID: The memory identifier
//   - agentIDs: List of agent identifiers to filter by (optional)
//   - keywords: Keywords to search in session_id (optional)
//   - page: Page number (1-based)
//   - pageSize: Number of items per page
//
// Returns:
//   - map[string]interface{}: Contains "message_list" and "total_count"
//   - error: Error if the retrieval fails
//
// Example:
//
//	result, err := svc.ListMessage("tenant123", "mem456", []string{"agent1"}, "session", 1, 10)
//	messages := result["message_list"].([]map[string]interface{})
//	total := result["total_count"].(int64)
func (s *MessageService) ListMessage(tenantID string, memoryID string, agentIDs []string, keywords string, page int, pageSize int) (map[string]interface{}, error) {
	ctx := context.Background()
	index := indexName(tenantID)

	eng := engine.Get()
	if eng == nil {
		return nil, fmt.Errorf("document engine not initialized")
	}

	offset := (page - 1) * pageSize

	// Build filter conditions
	mustClauses := []map[string]interface{}{
		{
			"term": map[string]interface{}{"message_type": "raw"},
		},
	}

	if len(agentIDs) > 0 {
		mustClauses = append(mustClauses, map[string]interface{}{
			"terms": map[string]interface{}{"agent_id": agentIDs},
		})
	}

	if keywords != "" {
		mustClauses = append(mustClauses, map[string]interface{}{
			"term": map[string]interface{}{"session_id": keywords},
		})
	}

	// Build search request
	searchReq := &elasticsearch.SearchRequest{
		IndexNames: []string{index},
		Query: map[string]interface{}{
			"bool": map[string]interface{}{
				"must": mustClauses,
				"filter": []map[string]interface{}{
					{"term": map[string]interface{}{"kb_id": memoryID}},
				},
			},
		},
		Size: pageSize,
		From: offset,
		Sort: []interface{}{
			map[string]interface{}{"valid_at": map[string]interface{}{"order": "desc"}},
		},
		Source: []string{
			"message_id", "message_type", "source_id", "memory_id",
			"user_id", "agent_id", "session_id", "valid_at",
			"invalid_at", "forget_at", "status",
		},
	}

	resp, err := eng.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}

	var messages []map[string]interface{}
	var total int64

	if searchResp, ok := resp.(*elasticsearch.SearchResponse); ok {
		total = searchResp.Hits.Total.Value
		for _, hit := range searchResp.Hits.Hits {
			messages = append(messages, hit.Source)
		}
	}

	if messages == nil {
		messages = []map[string]interface{}{}
	}

	return map[string]interface{}{
		"message_list": messages,
		"total_count":  total,
	}, nil
}

// SearchMessage searches for messages based on filters and parameters
//
// Parameters:
//   - memoryIDs: List of memory identifiers to search in
//   - conditionDict: Filter conditions
//   - tenantIDs: List of tenant identifiers
//   - topN: Maximum number of results to return
//
// Returns:
//   - []map[string]interface{}: List of matching messages
//   - error: Error if the search fails
//
// Example:
//
//	messages, err := svc.SearchMessage(
//	    []string{"mem1", "mem2"},
//	    map[string]interface{}{"status": 1},
//	    []string{"tenant123"},
//	    10,
//	)
func (s *MessageService) SearchMessage(memoryIDs []string, conditionDict map[string]interface{}, tenantIDs []string, topN int) ([]map[string]interface{}, error) {
	ctx := context.Background()

	eng := engine.Get()
	if eng == nil {
		return nil, fmt.Errorf("document engine not initialized")
	}

	// Build index names from tenant IDs
	indexNames := make([]string, len(tenantIDs))
	for i, tenantID := range tenantIDs {
		indexNames[i] = indexName(tenantID)
	}

	// Default filter: only valid messages
	if _, ok := conditionDict["status"]; !ok {
		conditionDict["status"] = 1
	}

	// Build filter clauses
	filterClauses := []map[string]interface{}{
		{"terms": map[string]interface{}{"kb_id": memoryIDs}},
	}

	for key, value := range conditionDict {
		filterClauses = append(filterClauses, map[string]interface{}{
			"term": map[string]interface{}{key: value},
		})
	}

	// Build search request
	searchReq := &elasticsearch.SearchRequest{
		IndexNames: indexNames,
		Query: map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": filterClauses,
			},
		},
		Size: topN,
		Sort: []interface{}{
			map[string]interface{}{"valid_at": map[string]interface{}{"order": "desc"}},
		},
		Source: []string{
			"message_id", "message_type", "source_id", "memory_id",
			"user_id", "agent_id", "session_id", "valid_at",
			"invalid_at", "forget_at", "status", "content",
		},
	}

	resp, err := eng.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("failed to search messages: %w", err)
	}

	var results []map[string]interface{}
	if searchResp, ok := resp.(*elasticsearch.SearchResponse); ok {
		for _, hit := range searchResp.Hits.Hits {
			results = append(results, hit.Source)
		}
	}

	if results == nil {
		results = []map[string]interface{}{}
	}

	return results, nil
}

// GetMaxMessageID retrieves the maximum message ID from the given memories
//
// Parameters:
//   - tenantIDs: List of tenant identifiers
//   - memoryIDs: List of memory identifiers
//
// Returns:
//   - int64: The maximum message ID (returns 1 if no messages found)
//   - error: Error if the query fails
//
// Example:
//
//	maxID, err := svc.GetMaxMessageID([]string{"tenant123"}, []string{"mem456"})
func (s *MessageService) GetMaxMessageID(tenantIDs []string, memoryIDs []string) (int64, error) {
	ctx := context.Background()

	eng := engine.Get()
	if eng == nil {
		return 1, fmt.Errorf("document engine not initialized")
	}

	// Build index names from tenant IDs
	indexNames := make([]string, len(tenantIDs))
	for i, tenantID := range tenantIDs {
		indexNames[i] = indexName(tenantID)
	}

	// Build search request
	searchReq := &elasticsearch.SearchRequest{
		IndexNames: indexNames,
		Query: map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": []map[string]interface{}{
					{"terms": map[string]interface{}{"kb_id": memoryIDs}},
				},
			},
		},
		Size: 1,
		Sort: []interface{}{
			map[string]interface{}{"message_id": map[string]interface{}{"order": "desc"}},
		},
		Source: []string{"message_id"},
	}

	resp, err := eng.Search(ctx, searchReq)
	if err != nil {
		return 1, fmt.Errorf("failed to get max message ID: %w", err)
	}

	if searchResp, ok := resp.(*elasticsearch.SearchResponse); ok {
		if searchResp.Hits.Total.Value > 0 && len(searchResp.Hits.Hits) > 0 {
			if msgID, ok := searchResp.Hits.Hits[0].Source["message_id"]; ok {
				switch v := msgID.(type) {
				case float64:
					return int64(v), nil
				case int64:
					return v, nil
				case int:
					return int64(v), nil
				}
			}
		}
	}

	return 1, nil
}
