package component

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"
	"gorm.io/gorm"
	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/dao"
	mcpclient "ragflow/internal/mcp/client"
	"ragflow/internal/permission"
	servercfg "ragflow/internal/server"
)

// buildAgentMCPTools binds the saved tool selection to servers the canvas can use.
// Discovery happens when configuring the server; execution uses those schemas.
func buildAgentMCPTools(ctx context.Context, db *gorm.DB, raw any, timeout time.Duration) ([]einotool.BaseTool, error) {
	if raw == nil {
		return nil, nil
	}
	var selections []struct {
		ID    string                    `json:"mcp_id"`
		Tools map[string]mcpclient.Tool `json:"tools"`
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
		userID, _ := state.Sys["user_id"].(string)
		canvasID, _ := state.Sys["canvas_id"].(string)
		if strings.TrimSpace(userID) == "" || strings.TrimSpace(canvasID) == "" {
			return nil, fmt.Errorf("MCP server %q: missing runtime user or canvas", selection.ID)
		}
		tenantID, _ := state.Sys["tenant_id"].(string)
		checker := permission.NewDatabaseChecker(db)
		err = checker.CheckDependency(
			ctx,
			permission.Subject{UserID: userID, TenantID: tenantID},
			permission.ResourceRef{Kind: permission.ResourceKindCanvas, ID: canvasID},
			permission.ResourceRef{Kind: permission.ResourceKindMCPServer, ID: selection.ID},
			permission.OperationRun,
			permission.OperationUse,
		)
		if err != nil {
			return nil, fmt.Errorf("MCP server %q: authorize canvas dependency: %w", selection.ID, err)
		}
		server, err := dao.NewMCPServerDAO().GetByID(ctx, db, selection.ID)
		if err != nil {
			return nil, fmt.Errorf("load MCP server %q: %w", selection.ID, err)
		}
		if server == nil {
			return nil, fmt.Errorf("load MCP server %q: %w", selection.ID, permission.ErrResourceNotFound)
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
		allowPrivateHosts := servercfg.MCPAllowPrivateHosts()
		for _, name := range slices.Sorted(maps.Keys(selection.Tools)) {
			descriptor := selection.Tools[name]
			descriptor.Name = name
			adapter := agenttool.NewMCPToolAdapterWithOptions(descriptor, mcpclient.CallOptions{
				URL: server.URL, ServerType: server.ServerType, Headers: headers, Variables: variables, Timeout: timeout,
				AllowPrivateHosts: allowPrivateHosts,
			})
			adapter.SetVisibleName(fmt.Sprintf("%s_%d", name, toolIndex))
			toolIndex++
			tools = append(tools, adapter)
		}
	}
	return tools, nil
}
