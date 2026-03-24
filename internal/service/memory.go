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
	"errors"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"ragflow/internal/dao"
	"ragflow/internal/model"
)

const (
	// MemoryNameLimit is the maximum length allowed for memory names
	MemoryNameLimit = 128
	// MemorySizeLimit is the maximum memory size in bytes (5MB)
	MemorySizeLimit = 5242880
)

// MemoryType represents different types of memory using bit flags
// Multiple types can be combined using bitwise OR operations
type MemoryType int

const (
	// MemoryTypeRaw represents raw memory type (binary: 0001)
	MemoryTypeRaw MemoryType = 0b0001
	// MemoryTypeSemantic represents semantic memory type (binary: 0010)
	MemoryTypeSemantic MemoryType = 0b0010
	// MemoryTypeEpisodic represents episodic memory type (binary: 0100)
	MemoryTypeEpisodic MemoryType = 0b0100
	// MemoryTypeProcedural represents procedural memory type (binary: 1000)
	MemoryTypeProcedural MemoryType = 0b1000
)

// memoryTypeMap maps memory type names to their corresponding bit flag values
var memoryTypeMap = map[string]MemoryType{
	"raw":        MemoryTypeRaw,
	"semantic":   MemoryTypeSemantic,
	"episodic":   MemoryTypeEpisodic,
	"procedural": MemoryTypeProcedural,
}

// validMemoryTypes defines which memory types are valid
var validMemoryTypes = map[MemoryType]bool{
	MemoryTypeRaw:        true,
	MemoryTypeSemantic:   true,
	MemoryTypeEpisodic:   true,
	MemoryTypeProcedural: true,
}

// TenantPermission defines the access permission levels for memory resources
type TenantPermission string

const (
	// TenantPermissionMe restricts access to the owner only
	TenantPermissionMe TenantPermission = "me"
	// TenantPermissionTeam allows access within the same team
	TenantPermissionTeam TenantPermission = "team"
	// TenantPermissionAll allows access to all tenants
	TenantPermissionAll TenantPermission = "all"
)

// validPermissions defines which permission values are valid
var validPermissions = map[TenantPermission]bool{
	TenantPermissionMe:   true,
	TenantPermissionTeam: true,
	TenantPermissionAll:  true,
}

// ForgettingPolicy defines the strategy for forgetting old memory entries
type ForgettingPolicy string

const (
	// ForgettingPolicyFIFO uses First-In-First-Out strategy for forgetting
	ForgettingPolicyFIFO ForgettingPolicy = "FIFO"
)

// validForgettingPolicies defines which forgetting policies are valid
var validForgettingPolicies = map[ForgettingPolicy]bool{
	ForgettingPolicyFIFO: true,
}

// CalculateMemoryType converts memory type names array to bit flags integer
//
// Parameters:
//   - memoryTypeNames: Array of memory type names (e.g., ["raw", "semantic"])
//
// Returns:
//   - int64: Bit flags integer representing the combined memory types
//
// Example:
//
//	CalculateMemoryType([]string{"raw", "semantic"}) returns 3 (0b0011)
func CalculateMemoryType(memoryTypeNames []string) int64 {
	memoryType := 0
	for _, name := range memoryTypeNames {
		lowerName := strings.ToLower(name)
		if mt, ok := memoryTypeMap[lowerName]; ok {
			memoryType |= int(mt)
		}
	}
	return int64(memoryType)
}

// GetMemoryTypeHuman converts memory type bit flags to human-readable names
//
// Parameters:
//   - memoryType: Bit flags integer representing memory types
//
// Returns:
//   - []string: Array of memory type names
//
// Example:
//
//	GetMemoryTypeHuman(3) returns ["raw", "semantic"]
func GetMemoryTypeHuman(memoryType int64) []string {
	var result []string
	for mt, valid := range validMemoryTypes {
		if valid && int64(memoryType)&int64(mt) != 0 {
			result = append(result, mt.Name())
		}
	}
	return result
}

// Name returns the string representation of a MemoryType
//
// Returns:
//   - string: The memory type name ("raw", "semantic", "episodic", "procedural", or "unknown")
func (m MemoryType) Name() string {
	switch m {
	case MemoryTypeRaw:
		return "raw"
	case MemoryTypeSemantic:
		return "semantic"
	case MemoryTypeEpisodic:
		return "episodic"
	case MemoryTypeProcedural:
		return "procedural"
	default:
		return "unknown"
	}
}

// PromptAssembler handles the assembly of system prompts for memory extraction
type PromptAssembler struct{}

// SYSTEM_BASE_TEMPLATE is the base template for the system prompt used in memory extraction
// It includes placeholders for type-specific instructions, timestamp format, and max items
var SYSTEM_BASE_TEMPLATE = `**Memory Extraction Specialist**
You are an expert at analyzing conversations to extract structured memory.

{type_specific_instructions}


**OUTPUT REQUIREMENTS:**
1. Output MUST be valid JSON
2. Follow the specified output format exactly
3. Each extracted item MUST have: content, valid_at, invalid_at
4. Timestamps in {timestamp_format} format
5. Only extract memory types specified above
6. Maximum {max_items} items per type
`

// TYPE_INSTRUCTIONS contains specific instructions for each memory type extraction
var TYPE_INSTRUCTIONS = map[string]string{
	"semantic": `
**EXTRACT SEMANTIC KNOWLEDGE:**
- Universal facts, definitions, concepts, relationships
- Time-invariant, generally true information

**Timestamp Rules:**
- valid_at: When the fact became true
- invalid_at: When it becomes false or empty if still true
`,
	"episodic": `
**EXTRACT EPISODIC KNOWLEDGE:**
- Specific experiences, events, personal stories
- Time-bound, person-specific, contextual

**Timestamp Rules:**
- valid_at: Event start/occurrence time
- invalid_at: Event end time or empty if instantaneous
`,
	"procedural": `
**EXTRACT PROCEDURAL KNOWLEDGE:**
- Processes, methods, step-by-step instructions
- Goal-oriented, actionable, often includes conditions

**Timestamp Rules:**
- valid_at: When procedure becomes valid/effective
- invalid_at: When it expires/becomes obsolete or empty if current
`,
}

// OUTPUT_TEMPLATES defines the output format for each memory type
var OUTPUT_TEMPLATES = map[string]string{
	"semantic":   `"semantic": [{"content": "Clear factual statement", "valid_at": "timestamp or empty", "invalid_at": "timestamp or empty"}]`,
	"episodic":   `"episodic": [{"content": "Narrative event description", "valid_at": "event start timestamp", "invalid_at": "event end timestamp or empty"}]`,
	"procedural": `"procedural": [{"content": "Actionable instructions", "valid_at": "procedure effective timestamp", "invalid_at": "procedure expiration timestamp or empty"}]`,
}

// AssembleSystemPrompt generates a complete system prompt for memory extraction
//
// Parameters:
//   - memoryTypes: Array of memory type names to extract (e.g., ["semantic", "episodic"])
//
// Returns:
//   - string: Complete system prompt with type-specific instructions and output format
//
// Example:
//
//	AssembleSystemPrompt([]string{"semantic", "episodic"}) returns a prompt with instructions
//	for both semantic and episodic memory extraction
func (PromptAssembler) AssembleSystemPrompt(memoryTypes []string) string {
	typesToExtract := getTypesToExtract(memoryTypes)
	if len(typesToExtract) == 0 {
		typesToExtract = []string{"raw"}
	}

	typeInstructions := generateTypeInstructions(typesToExtract)
	outputFormat := generateOutputFormat(typesToExtract)

	fullPrompt := strings.Replace(SYSTEM_BASE_TEMPLATE, "{type_specific_instructions}", typeInstructions, 1)
	fullPrompt = strings.Replace(fullPrompt, "{timestamp_format}", "ISO 8601", 1)
	fullPrompt = strings.Replace(fullPrompt, "{max_items}", "5", 1)

	fullPrompt += fmt.Sprintf("\n**REQUIRED OUTPUT FORMAT (JSON):\n```json\n{\n%s\n}\n```\n", outputFormat)

	return fullPrompt
}

// getTypesToExtract filters out "raw" type and returns valid memory types
//
// Parameters:
//   - requestedTypes: Array of requested memory type names
//
// Returns:
//   - []string: Filtered array of memory type names (excluding "raw")
func getTypesToExtract(requestedTypes []string) []string {
	types := make(map[string]bool)
	for _, rt := range requestedTypes {
		lowerRT := strings.ToLower(rt)
		if lowerRT != "raw" {
			if _, ok := memoryTypeMap[lowerRT]; ok {
				types[lowerRT] = true
			}
		}
	}
	result := make([]string, 0, len(types))
	for t := range types {
		result = append(result, t)
	}
	return result
}

// generateTypeInstructions concatenates type-specific instructions
//
// Parameters:
//   - typesToExtract: Array of memory type names
//
// Returns:
//   - string: Concatenated instructions for all specified types
func generateTypeInstructions(typesToExtract []string) string {
	var instructions []string
	for _, mt := range typesToExtract {
		if instr, ok := TYPE_INSTRUCTIONS[mt]; ok {
			instructions = append(instructions, instr)
		}
	}
	return strings.Join(instructions, "\n")
}

// generateOutputFormat concatenates output format templates
//
// Parameters:
//   - typesToExtract: Array of memory type names
//
// Returns:
//   - string: Concatenated output format templates
func generateOutputFormat(typesToExtract []string) string {
	var outputParts []string
	for _, mt := range typesToExtract {
		if tmpl, ok := OUTPUT_TEMPLATES[mt]; ok {
			outputParts = append(outputParts, tmpl)
		}
	}
	return strings.Join(outputParts, ",\n")
}

// MemoryService handles business logic for memory operations
// It provides methods for creating, updating, deleting, and querying memories
type MemoryService struct {
	memoryDAO *dao.MemoryDAO
}

// NewMemoryService creates a new MemoryService instance
//
// Returns:
//   - *MemoryService: Initialized service instance with DAO
func NewMemoryService() *MemoryService {
	return &MemoryService{
		memoryDAO: dao.NewMemoryDAO(),
	}
}

// splitNameCounter splits a filename into base name and counter
// Handles names in format "filename(123)" pattern
//
// Parameters:
//   - filename: The filename to split
//
// Returns:
//   - string: The base name without counter
//   - *int: The counter value, or nil if no counter exists
//
// Example:
//
//	splitNameCounter("test(5)") returns ("test", 5)
//	splitNameCounter("test") returns ("test", nil)
func splitNameCounter(filename string) (string, *int) {
	re := regexp.MustCompile(`^(.+)\((\d+)\)$`)
	matches := re.FindStringSubmatch(filename)
	if len(matches) >= 3 {
		counter := -1
		fmt.Sscanf(matches[2], "%d", &counter)
		stem := strings.TrimRight(matches[1], " ")
		return stem, &counter
	}
	return filename, nil
}

// duplicateName generates a unique name by appending a counter if the name already exists
// It tries up to 1000 times to generate a unique name
//
// Parameters:
//   - queryFunc: Function to check if a name already exists (returns true if exists)
//   - name: The original name
//   - tenantID: The tenant ID for name uniqueness check
//
// Returns:
//   - string: A unique name (either original or with counter appended)
//
// Example:
//
//	duplicateName(func(name string, tid string) bool { return false }, "test", "tenant1") returns "test"
//	duplicateName(func(name string, tid string) bool { return true }, "test", "tenant1") returns "test(1)"
func duplicateName(queryFunc func(name string, tenantID string) bool, name string, tenantID string) string {
	const maxRetries = 1000

	originalName := name
	currentName := name
	retries := 0

	for retries < maxRetries {
		if !queryFunc(currentName, tenantID) {
			return currentName
		}

		stem, counter := splitNameCounter(currentName)
		ext := path.Ext(stem)
		stemBase := strings.TrimSuffix(stem, ext)

		newCounter := 1
		if counter != nil {
			newCounter = *counter + 1
		}

		currentName = fmt.Sprintf("%s(%d)%s", stemBase, newCounter, ext)
		retries++
	}

	panic(fmt.Sprintf("Failed to generate unique name within %d attempts. Original: %s", maxRetries, originalName))
}

// CreateMemoryRequest defines the request structure for creating a memory
type CreateMemoryRequest struct {
	// Name is the memory name (required, max 128 characters)
	Name string `json:"name" binding:"required"`
	// MemoryType is the array of memory type names (required)
	MemoryType []string `json:"memory_type" binding:"required"`
	// EmbdID is the embedding model ID (required)
	EmbdID string `json:"embd_id" binding:"required"`
	// LLMID is the language model ID (required)
	LLMID string `json:"llm_id" binding:"required"`
	// TenantEmbdID is the tenant-specific embedding model ID (optional)
	TenantEmbdID *string `json:"tenant_embd_id"`
	// TenantLLMID is the tenant-specific language model ID (optional)
	TenantLLMID *string `json:"tenant_llm_id"`
}

// UpdateMemoryRequest defines the request structure for updating a memory
// All fields are optional, only provided fields will be updated
type UpdateMemoryRequest struct {
	// Name is the new memory name (optional)
	Name *string `json:"name"`
	// Permissions is the new permission level (optional)
	Permissions *string `json:"permissions"`
	// LLMID is the new language model ID (optional)
	LLMID *string `json:"llm_id"`
	// EmbdID is the new embedding model ID (optional)
	EmbdID *string `json:"embd_id"`
	// TenantLLMID is the new tenant-specific language model ID (optional)
	TenantLLMID *string `json:"tenant_llm_id"`
	// TenantEmbdID is the new tenant-specific embedding model ID (optional)
	TenantEmbdID *string `json:"tenant_embd_id"`
	// MemoryType is the new array of memory type names (optional)
	MemoryType []string `json:"memory_type"`
	// MemorySize is the new memory size in bytes (optional, max 5MB)
	MemorySize *int64 `json:"memory_size"`
	// ForgettingPolicy is the new forgetting policy (optional)
	ForgettingPolicy *string `json:"forgetting_policy"`
	// Temperature is the new temperature value (optional, range [0, 1])
	Temperature *float64 `json:"temperature"`
	// Avatar is the new avatar URL (optional)
	Avatar *string `json:"avatar"`
	// Description is the new description (optional)
	Description *string `json:"description"`
	// SystemPrompt is the new system prompt (optional)
	SystemPrompt *string `json:"system_prompt"`
	// UserPrompt is the new user prompt (optional)
	UserPrompt *string `json:"user_prompt"`
}

// CreateMemoryResponse defines the response structure for memory operations
type CreateMemoryResponse struct {
	// ID is the unique memory identifier
	ID string `json:"id"`
	// Name is the memory name
	Name string `json:"name"`
	// Avatar is the avatar URL (optional)
	Avatar *string `json:"avatar,omitempty"`
	// TenantID is the tenant identifier
	TenantID string `json:"tenant_id"`
	// OwnerName is the owner's name (optional)
	OwnerName *string `json:"owner_name,omitempty"`
	// MemoryType is the array of memory type names
	MemoryType []string `json:"memory_type"`
	// StorageType is the storage type (e.g., "table")
	StorageType string `json:"storage_type"`
	// EmbdID is the embedding model ID
	EmbdID string `json:"embd_id"`
	// LLMID is the language model ID
	LLMID string `json:"llm_id"`
	// TenantEmbdID is the tenant-specific embedding model ID (optional)
	TenantEmbdID *string `json:"tenant_embd_id,omitempty"`
	// TenantLLMID is the tenant-specific language model ID (optional)
	TenantLLMID *string `json:"tenant_llm_id,omitempty"`
	// Permissions is the permission level
	Permissions string `json:"permissions"`
	// Description is the memory description (optional)
	Description *string `json:"description,omitempty"`
	// MemorySize is the memory size in bytes
	MemorySize int64 `json:"memory_size"`
	// ForgettingPolicy is the forgetting policy
	ForgettingPolicy string `json:"forgetting_policy"`
	// Temperature is the temperature value
	Temperature float64 `json:"temperature"`
	// SystemPrompt is the system prompt (optional)
	SystemPrompt *string `json:"system_prompt,omitempty"`
	// UserPrompt is the user prompt (optional)
	UserPrompt *string `json:"user_prompt,omitempty"`
	// CreateTime is the creation timestamp in milliseconds (optional)
	CreateTime *int64 `json:"create_time,omitempty"`
	// CreateDate is the creation date string (optional)
	CreateDate *string `json:"create_date,omitempty"`
	// UpdateTime is the update timestamp in milliseconds (optional)
	UpdateTime *int64 `json:"update_time,omitempty"`
	// UpdateDate is the update date string (optional)
	UpdateDate *string `json:"update_date,omitempty"`
}

// ListMemoryResponse defines the response structure for listing memories
type ListMemoryResponse struct {
	// MemoryList is the array of memory objects
	MemoryList []map[string]interface{} `json:"memory_list"`
	// TotalCount is the total number of memories
	TotalCount int64 `json:"total_count"`
}

// CreateMemory creates a new memory with the given parameters
// It validates the request, generates a unique name if needed, and creates the memory record
//
// Parameters:
//   - tenantID: The tenant ID for which to create the memory
//   - req: The memory creation request containing name, memory_type, embd_id, llm_id, etc.
//
// Returns:
//   - *CreateMemoryResponse: The created memory details
//   - error: Error if validation fails or creation fails
//
// Example:
//
//	req := &CreateMemoryRequest{Name: "MyMemory", MemoryType: []string{"semantic"}, EmbdID: "embd1", LLMID: "llm1"}
//	resp, err := service.CreateMemory("tenant123", req)
func (s *MemoryService) CreateMemory(tenantID string, req *CreateMemoryRequest) (*CreateMemoryResponse, error) {
	// Ensure tenant model IDs are populated for LLM and embedding model parameters
	// This automatically fills tenant_llm_id and tenant_embd_id based on llm_id and embd_id
	tenantLLMService := NewTenantLLMService()
	params := map[string]interface{}{
		"llm_id":  req.LLMID,
		"embd_id": req.EmbdID,
	}
	params = tenantLLMService.EnsureTenantModelIDForParams(tenantID, params)

	// Update request with tenant model IDs from the processed params
	if tenantLLMID, ok := params["tenant_llm_id"].(int64); ok {
		tenantLLMIDStr := strconv.FormatInt(tenantLLMID, 10)
		req.TenantLLMID = &tenantLLMIDStr
	}
	if tenantEmbdID, ok := params["tenant_embd_id"].(int64); ok {
		tenantEmbdIDStr := strconv.FormatInt(tenantEmbdID, 10)
		req.TenantEmbdID = &tenantEmbdIDStr
	}

	memoryName := strings.TrimSpace(req.Name)
	if len(memoryName) == 0 {
		return nil, errors.New("memory name cannot be empty or whitespace")
	}
	if len(memoryName) > MemoryNameLimit {
		return nil, fmt.Errorf("memory name '%s' exceeds limit of %d", memoryName, MemoryNameLimit)
	}

	if !isList(req.MemoryType) {
		return nil, errors.New("memory type must be a list")
	}

	memoryTypeSet := make(map[string]bool)
	for _, mt := range req.MemoryType {
		lowerMT := strings.ToLower(mt)
		if _, ok := memoryTypeMap[lowerMT]; !ok {
			return nil, fmt.Errorf("memory type '%s' is not supported", mt)
		}
		memoryTypeSet[lowerMT] = true
	}
	uniqueMemoryTypes := make([]string, 0, len(memoryTypeSet))
	for mt := range memoryTypeSet {
		uniqueMemoryTypes = append(uniqueMemoryTypes, mt)
	}

	memoryName = duplicateName(func(name string, tid string) bool {
		existing, _ := s.memoryDAO.GetByNameAndTenant(name, tid)
		return len(existing) > 0
	}, memoryName, tenantID)

	if len(memoryName) > MemoryNameLimit {
		return nil, fmt.Errorf("memory name %s exceeds limit of %d", memoryName, MemoryNameLimit)
	}

	memoryTypeInt := CalculateMemoryType(uniqueMemoryTypes)
	timestamp := time.Now().UnixMilli()

	systemPrompt := PromptAssembler{}.AssembleSystemPrompt(uniqueMemoryTypes)

	newID := strings.ReplaceAll(uuid.New().String(), "-", "")
	if len(newID) > 32 {
		newID = newID[:32]
	}

	memory := &model.Memory{
		ID:               newID,
		Name:             memoryName,
		TenantID:         tenantID,
		MemoryType:       memoryTypeInt,
		StorageType:      "table",
		EmbdID:           req.EmbdID,
		LLMID:            req.LLMID,
		Permissions:      "me",
		MemorySize:       MemorySizeLimit,
		ForgettingPolicy: string(ForgettingPolicyFIFO),
		Temperature:      0.5,
		SystemPrompt:     &systemPrompt,
	}

	// Convert tenant model IDs from string to int64 for database
	if req.TenantEmbdID != nil {
		if embdID, err := strconv.ParseInt(*req.TenantEmbdID, 10, 64); err == nil {
			memory.TenantEmbdID = &embdID
		}
	}
	if req.TenantLLMID != nil {
		if llmID, err := strconv.ParseInt(*req.TenantLLMID, 10, 64); err == nil {
			memory.TenantLLMID = &llmID
		}
	}
	memory.CreateTime = &timestamp
	memory.UpdateTime = &timestamp

	if err := s.memoryDAO.Create(memory); err != nil {
		return nil, errors.New("could not create new memory")
	}

	createdMemory, err := s.memoryDAO.GetByID(newID)
	if err != nil {
		return nil, errors.New("could not create new memory")
	}

	return formatRetDataFromMemory(createdMemory), nil
}

// UpdateMemory updates an existing memory with the provided fields
// Only the fields specified in the request will be updated (partial update)
//
// Parameters:
//   - tenantID: The tenant ID for ownership verification
//   - memoryID: The ID of the memory to update
//   - req: The update request with optional fields to update
//
// Returns:
//   - *CreateMemoryResponse: The updated memory details
//   - error: Error if validation fails or update fails
//
// Example:
//
//	req := &UpdateMemoryRequest{Name: ptr("NewName"), MemorySize: ptr(int64(1000000))}
//	resp, err := service.UpdateMemory("tenant123", "memory456", req)
func (s *MemoryService) UpdateMemory(tenantID string, memoryID string, req *UpdateMemoryRequest) (*CreateMemoryResponse, error) {
	updateDict := make(map[string]interface{})

	if req.Name != nil {
		memoryName := strings.TrimSpace(*req.Name)
		if len(memoryName) == 0 {
			return nil, errors.New("memory name cannot be empty or whitespace")
		}
		if len(memoryName) > MemoryNameLimit {
			return nil, fmt.Errorf("memory name '%s' exceeds limit of %d", memoryName, MemoryNameLimit)
		}
		memoryName = duplicateName(func(name string, tid string) bool {
			existing, _ := s.memoryDAO.GetByNameAndTenant(name, tid)
			return len(existing) > 0
		}, memoryName, tenantID)
		if len(memoryName) > MemoryNameLimit {
			return nil, fmt.Errorf("memory name %s exceeds limit of %d", memoryName, MemoryNameLimit)
		}
		updateDict["name"] = memoryName
	}

	if req.Permissions != nil {
		perm := TenantPermission(strings.ToLower(*req.Permissions))
		if !validPermissions[perm] {
			return nil, fmt.Errorf("unknown permission '%s'", *req.Permissions)
		}
		updateDict["permissions"] = perm
	}

	if req.LLMID != nil {
		updateDict["llm_id"] = *req.LLMID
	}

	if req.EmbdID != nil {
		updateDict["embd_id"] = *req.EmbdID
	}

	if req.TenantLLMID != nil {
		if llmID, err := strconv.ParseInt(*req.TenantLLMID, 10, 64); err == nil {
			updateDict["tenant_llm_id"] = llmID
		}
	}

	if req.TenantEmbdID != nil {
		if embdID, err := strconv.ParseInt(*req.TenantEmbdID, 10, 64); err == nil {
			updateDict["tenant_embd_id"] = embdID
		}
	}

	if req.MemoryType != nil && len(req.MemoryType) > 0 {
		memoryTypeSet := make(map[string]bool)
		for _, mt := range req.MemoryType {
			lowerMT := strings.ToLower(mt)
			if _, ok := memoryTypeMap[lowerMT]; !ok {
				return nil, fmt.Errorf("memory type '%s' is not supported", mt)
			}
			memoryTypeSet[lowerMT] = true
		}
		uniqueMemoryTypes := make([]string, 0, len(memoryTypeSet))
		for mt := range memoryTypeSet {
			uniqueMemoryTypes = append(uniqueMemoryTypes, mt)
		}
		updateDict["memory_type"] = uniqueMemoryTypes
	}

	if req.MemorySize != nil {
		memorySize := *req.MemorySize
		if !(memorySize > 0 && memorySize <= MemorySizeLimit) {
			return nil, fmt.Errorf("memory size should be in range (0, %d] Bytes", MemorySizeLimit)
		}
		updateDict["memory_size"] = memorySize
	}

	if req.ForgettingPolicy != nil {
		fp := ForgettingPolicy(strings.ToLower(*req.ForgettingPolicy))
		if !validForgettingPolicies[fp] {
			return nil, fmt.Errorf("forgetting policy '%s' is not supported", *req.ForgettingPolicy)
		}
		updateDict["forgetting_policy"] = fp
	}

	if req.Temperature != nil {
		temp := *req.Temperature
		if !(temp >= 0 && temp <= 1) {
			return nil, errors.New("temperature should be in range [0, 1]")
		}
		updateDict["temperature"] = temp
	}

	for _, field := range []string{"avatar", "description", "system_prompt", "user_prompt"} {
		switch field {
		case "avatar":
			if req.Avatar != nil {
				updateDict["avatar"] = *req.Avatar
			}
		case "description":
			if req.Description != nil {
				updateDict["description"] = *req.Description
			}
		case "system_prompt":
			if req.SystemPrompt != nil {
				updateDict["system_prompt"] = *req.SystemPrompt
			}
		case "user_prompt":
			if req.UserPrompt != nil {
				updateDict["user_prompt"] = *req.UserPrompt
			}
		}
	}

	currentMemory, err := s.memoryDAO.GetByID(memoryID)
	if err != nil {
		return nil, fmt.Errorf("memory '%s' not found", memoryID)
	}

	if len(updateDict) == 0 {
		return formatRetDataFromMemory(currentMemory), nil
	}

	memorySize := currentMemory.MemorySize
	notAllowedUpdate := []string{}
	for _, f := range []string{"tenant_embd_id", "embd_id", "memory_type"} {
		if _, ok := updateDict[f]; ok && memorySize > 0 {
			notAllowedUpdate = append(notAllowedUpdate, f)
		}
	}
	if len(notAllowedUpdate) > 0 {
		return nil, fmt.Errorf("can't update %v when memory isn't empty", notAllowedUpdate)
	}

	if _, ok := updateDict["memory_type"]; ok {
		if _, ok := updateDict["system_prompt"]; !ok {
			memoryTypes := GetMemoryTypeHuman(currentMemory.MemoryType)
			if len(memoryTypes) > 0 && currentMemory.SystemPrompt != nil {
				defaultPrompt := PromptAssembler{}.AssembleSystemPrompt(memoryTypes)
				if *currentMemory.SystemPrompt == defaultPrompt {
					if types, ok := updateDict["memory_type"].([]string); ok {
						updateDict["system_prompt"] = PromptAssembler{}.AssembleSystemPrompt(types)
					}
				}
			}
		}
	}

	if err := s.memoryDAO.UpdateByID(memoryID, updateDict); err != nil {
		return nil, errors.New("failed to update memory")
	}

	updatedMemory, err := s.memoryDAO.GetByID(memoryID)
	if err != nil {
		return nil, errors.New("failed to get updated memory")
	}

	return formatRetDataFromMemory(updatedMemory), nil
}

// DeleteMemory deletes a memory by ID
// It also deletes associated message indexes before removing the memory record
//
// Parameters:
//   - memoryID: The ID of the memory to delete
//
// Returns:
//   - error: Error if memory not found or deletion fails
//
// Example:
//
//	err := service.DeleteMemory("memory456")
func (s *MemoryService) DeleteMemory(memoryID string) error {
	_, err := s.memoryDAO.GetByID(memoryID)
	if err != nil {
		return fmt.Errorf("memory '%s' not found", memoryID)
	}

	// TODO: Delete associated message index - Implementation pending MessageService
	// messageService := NewMessageService()
	// hasIndex, _ := messageService.HasIndex(memory.TenantID, memoryID)
	// if hasIndex {
	//     messageService.DeleteMessage(nil, memory.TenantID, memoryID)
	// }

	// Delete memory record
	if err := s.memoryDAO.DeleteByID(memoryID); err != nil {
		return errors.New("failed to delete memory")
	}

	return nil
}

// ListMemories retrieves a paginated list of memories with optional filters
// When tenantIDs is empty, it retrieves all tenants associated with the user
//
// Parameters:
//   - userID: The user ID for tenant filtering when tenantIDs is empty
//   - tenantIDs: Array of tenant IDs to filter by (empty means all user's tenants)
//   - memoryTypes: Array of memory type names to filter by (empty means all types)
//   - storageType: Storage type to filter by (empty means all types)
//   - keywords: Keywords to search in memory names (empty means no keyword filter)
//   - page: Page number (1-based)
//   - pageSize: Number of items per page
//
// Returns:
//   - *ListMemoryResponse: Contains memory list and total count
//   - error: Error if query fails
//
// Example:
//
//	resp, err := service.ListMemories("user123", []string{}, []string{"semantic"}, "table", "test", 1, 10)
func (s *MemoryService) ListMemories(userID string, tenantIDs []string, memoryTypes []string, storageType string, keywords string, page int, pageSize int) (*ListMemoryResponse, error) {
	// If tenantIDs is empty, get all tenants associated with the user
	if len(tenantIDs) == 0 {
		userTenantService := NewUserTenantService()
		userTenants, err := userTenantService.GetUserTenantRelationByUserID(userID)
		if err != nil {
			return nil, fmt.Errorf("failed to get user tenants: %w", err)
		}
		tenantIDs = make([]string, len(userTenants))
		for i, tenant := range userTenants {
			tenantIDs[i] = tenant.TenantID
		}
	}

	memories, total, err := s.memoryDAO.GetByFilter(tenantIDs, memoryTypes, storageType, keywords, page, pageSize)
	if err != nil {
		return nil, err
	}

	memoryList := make([]map[string]interface{}, 0, len(memories))
	for _, m := range memories {
		resp := formatRetDataFromMemory(m)
		memoryMap := map[string]interface{}{
			"id":           resp.ID,
			"name":         resp.Name,
			"avatar":       resp.Avatar,
			"tenant_id":    resp.TenantID,
			"owner_name":   resp.OwnerName,
			"memory_type":  resp.MemoryType,
			"storage_type": resp.StorageType,
			"permissions":  resp.Permissions,
			"description":  resp.Description,
			"create_time":  resp.CreateTime,
			"create_date":  resp.CreateDate,
		}
		memoryList = append(memoryList, memoryMap)
	}

	return &ListMemoryResponse{
		MemoryList: memoryList,
		TotalCount: total,
	}, nil
}

// GetMemoryConfig retrieves the full configuration of a memory by ID
//
// Parameters:
//   - memoryID: The ID of the memory to retrieve
//
// Returns:
//   - *CreateMemoryResponse: The memory configuration details
//   - error: Error if memory not found
//
// Example:
//
//	resp, err := service.GetMemoryConfig("memory456")
func (s *MemoryService) GetMemoryConfig(memoryID string) (*CreateMemoryResponse, error) {
	memory, err := s.memoryDAO.GetWithOwnerNameByID(memoryID)
	if err != nil {
		return nil, fmt.Errorf("memory '%s' not found", memoryID)
	}
	return formatRetDataFromMemory(memory), nil
}

// TODO: GetMemoryMessages - Implementation pending - depends on CanvasService and TaskService
// func (s *MemoryService) GetMemoryMessages(memoryID string, agentIDs []string, keywords string, page int, pageSize int) (map[string]interface{}, error) { ... }

// TODO: queryMessages - Implementation pending - depends on CanvasService and TaskService
// func (s *MemoryService) queryMessages(tenantID string, memoryID string, filterDict map[string]interface{}, page int, pageSize int) ([]map[string]interface{}, int64, error) { ... }

// TODO: AddMessage - Implementation pending - depends on embedding engine
// func (s *MemoryService) AddMessage(memoryIDs []string, messageDict map[string]interface{}) (bool, string, error) { ... }

// TODO: ForgetMessage - Implementation pending - depends on embedding engine
// func (s *MemoryService) ForgetMessage(memoryID string, messageID int) (bool, error) { ... }

// TODO: UpdateMessageStatus - Implementation pending - depends on embedding engine
// func (s *MemoryService) UpdateMessageStatus(memoryID string, messageID int, status bool) (bool, error) { ... }

// TODO: SearchMessage - Implementation pending - depends on embedding engine
// func (s *MemoryService) SearchMessage(filterDict map[string]interface{}, params map[string]interface{}) ([]map[string]interface{}, error) { ... }

// TODO: GetMessages - Implementation pending - depends on embedding engine
// func (s *MemoryService) GetMessages(memoryIDs []string, agentID string, sessionID string, limit int) ([]map[string]interface{}, error) { ... }

// TODO: GetMessageContent - Implementation pending - depends on embedding engine
// func (s *MemoryService) GetMessageContent(memoryID string, messageID int) (map[string]interface{}, error) { ... }

// isList checks if a value is a list or array type
// This is a utility function for type validation
//
// Parameters:
//   - v: The value to check
//
// Returns:
//   - bool: true if v is []interface{} or []string, false otherwise
//
// Example:
//
//	isList([]string{"a", "b"}) returns true
//	isList("test") returns false
func isList(v interface{}) bool {
	switch v.(type) {
	case []interface{}, []string:
		return true
	default:
		return false
	}
}

// formatRetDataFromMemory converts a Memory model to CreateMemoryResponse format
// This is a utility function for formatting memory data for API responses
//
// Parameters:
//   - memory: The Memory model to format
//
// Returns:
//   - *CreateMemoryResponse: Formatted memory response with human-readable types and dates
//
// Example:
//
//	resp := formatRetDataFromMemory(memoryModel)
func formatRetDataFromMemory(memory *model.Memory) *CreateMemoryResponse {
	memoryTypes := GetMemoryTypeHuman(memory.MemoryType)

	var createDateStr, updateDateStr *string
	if memory.CreateDate != nil {
		s := memory.CreateDate.Format("2006-01-02 15:04:05")
		createDateStr = &s
	}
	if memory.UpdateDate != nil {
		s := memory.UpdateDate.Format("2006-01-02 15:04:05")
		updateDateStr = &s
	}

	// Convert tenant model IDs from int64 to string for response
	var tenantEmbdIDStr *string
	if memory.TenantEmbdID != nil {
		s := strconv.FormatInt(*memory.TenantEmbdID, 10)
		tenantEmbdIDStr = &s
	}
	var tenantLLMIDStr *string
	if memory.TenantLLMID != nil {
		s := strconv.FormatInt(*memory.TenantLLMID, 10)
		tenantLLMIDStr = &s
	}

	return &CreateMemoryResponse{
		ID:               memory.ID,
		Name:             memory.Name,
		Avatar:           memory.Avatar,
		TenantID:         memory.TenantID,
		OwnerName:        memory.OwnerName,
		MemoryType:       memoryTypes,
		StorageType:      memory.StorageType,
		EmbdID:           memory.EmbdID,
		TenantEmbdID:     tenantEmbdIDStr,
		LLMID:            memory.LLMID,
		TenantLLMID:      tenantLLMIDStr,
		Permissions:      memory.Permissions,
		Description:      memory.Description,
		MemorySize:       memory.MemorySize,
		ForgettingPolicy: memory.ForgettingPolicy,
		Temperature:      memory.Temperature,
		SystemPrompt:     memory.SystemPrompt,
		UserPrompt:       memory.UserPrompt,
		CreateTime:       memory.CreateTime,
		CreateDate:       createDateStr,
		UpdateTime:       memory.UpdateTime,
		UpdateDate:       updateDateStr,
	}
}
