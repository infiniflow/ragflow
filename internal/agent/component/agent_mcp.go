package component

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"
	"gorm.io/gorm"
	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/dao"
	"ragflow/internal/utility"
)

// buildAgentMCPTools binds the saved tool selection to tenant-owned servers.
// Discovery happens when configuring the server; execution uses those schemas.
func buildAgentMCPTools(ctx context.Context, db *gorm.DB, raw any, timeout time.Duration) ([]einotool.BaseTool, error) {
	if raw == nil {
		return nil, nil
	}
	var selections []struct {
		ID    string                  `json:"mcp_id"`
		Tools map[string]utility.Tool `json:"tools"`
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("encode Agent MCP configuration: %w", err)
	}
	if err := json.Unmarshal(data, &selections); err != nil {
		return nil, fmt.Errorf("decode Agent MCP configuration: %w", err)
	}
	var tools []einotool.BaseTool
	toolIndex := 0
	for _, selection := range selections {
		if len(selection.Tools) == 0 {
			continue
		}
		state, err := runtime.GetStateFromContext(ctx)
		if err != nil || state == nil || db == nil {
			return nil, fmt.Errorf("MCP server %q: missing runtime tenant or database", selection.ID)
		}
		tenantID, _ := state.Sys["tenant_id"].(string)
		if tenantID == "" {
			return nil, fmt.Errorf("MCP server %q: missing runtime tenant", selection.ID)
		}
		server, err := dao.NewMCPServerDAO().GetByIDAndTenant(ctx, db, selection.ID, tenantID)
		if err != nil {
			return nil, fmt.Errorf("load MCP server %q: %w", selection.ID, err)
		}
		headers := make(map[string]string, len(server.Headers))
		for key, value := range server.Headers {
			s, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("MCP server %q: header %q must be a string", selection.ID, key)
			}
			headers[key] = s
		}
		variables := make(map[string]string, len(server.Variables))
		for key, value := range server.Variables {
			variables[key] = fmt.Sprint(value)
		}
		for _, name := range slices.Sorted(maps.Keys(selection.Tools)) {
			descriptor := selection.Tools[name]
			descriptor.Name = name
			adapter := agenttool.NewMCPToolAdapterWithOptions(descriptor, utility.CallOptions{
				URL: server.URL, ServerType: server.ServerType, Headers: headers, Variables: variables, Timeout: timeout,
			})
			adapter.SetVisibleName(fmt.Sprintf("%s_%d", name, toolIndex))
			toolIndex++
			tools = append(tools, adapter)
		}
	}
	return tools, nil
}
