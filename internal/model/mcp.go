package model

// MCPServer MCP server model
type MCPServer struct {
	ID          string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	Name        string  `gorm:"column:name;size:255;not null" json:"name"`
	TenantID    string  `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	URL         string  `gorm:"column:url;size:2048;not null" json:"url"`
	ServerType  string  `gorm:"column:server_type;size:32;not null" json:"server_type"`
	Description *string `gorm:"column:description;type:longtext" json:"description,omitempty"`
	Variables   JSONMap `gorm:"column:variables;type:json;default:'{}'" json:"variables,omitempty"`
	Headers     JSONMap `gorm:"column:headers;type:json;default:'{}'" json:"headers,omitempty"`
	BaseModel
}

// TableName specify table name
func (MCPServer) TableName() string {
	return "mcp_server"
}
