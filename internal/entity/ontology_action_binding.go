package entity

// OntologyActionBinding binds a compiled action to the MCP tool that executes it.
//
// The binding is a separate table rather than action columns because the two
// change on different clocks: an action comes from recompiling a document, a
// binding comes from an operator wiring it to a system. Storing the mapping on
// the row would let a recompile silently rewrite which tool runs.
//
// Three switches, not one, because they answer different questions:
//
//	enabled     the mapping is live (off while it is being prepared or after the
//	            upstream system is decommissioned)
//	read_only   executing this cannot change anything upstream; defaults to false
//	            because "declare read-only" proves presence, not absence
//	qa_callable the agent may call it on its own; defaults to false because a new
//	            binding has not been trusted with an unattended turn yet
//
// Executing always needs the process-wide switch on top of these; a single stored
// row must not be able to turn execution on by itself.
type OntologyActionBinding struct {
	ID         string `gorm:"column:id;primaryKey;size:32" json:"id"`
	TenantID   string `gorm:"column:tenant_id;size:32;not null;index:idx_action_binding_scope" json:"tenant_id"`
	TemplateID string `gorm:"column:template_id;size:32;not null;index:idx_action_binding_scope" json:"template_id"`
	// ActionName is the compiled action's name rather than its row id: the row is
	// rewritten by a recompile (new id, same action), while the mapping is meant to
	// survive that.
	ActionName string   `gorm:"column:action_name;size:255;not null;index:idx_action_binding_scope" json:"action_name"`
	DatasetID  string   `gorm:"column:dataset_id;size:32;not null;default:''" json:"dataset_id"`
	MCPServer  string   `gorm:"column:mcp_server;size:255;not null" json:"mcp_server"`
	MCPTool    string   `gorm:"column:mcp_tool;size:255;not null" json:"mcp_tool"`
	ArgMapping *JSONMap `gorm:"column:arg_mapping;type:json" json:"arg_mapping,omitempty"`
	ReadOnly   bool     `gorm:"column:read_only;not null;default:0" json:"read_only"`
	Enabled    bool     `gorm:"column:enabled;not null;default:1" json:"enabled"`
	QACallable bool     `gorm:"column:qa_callable;not null;default:0" json:"qa_callable"`
	Note       string   `gorm:"column:note;size:512;not null;default:''" json:"note"`
	CreatedBy  string   `gorm:"column:created_by;size:32;not null;default:''" json:"created_by"`
	BaseModel
}

// TableName returns the table name for OntologyActionBinding.
func (OntologyActionBinding) TableName() string {
	return "ontology_action_binding"
}
