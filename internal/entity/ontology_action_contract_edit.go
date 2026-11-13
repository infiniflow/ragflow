package entity

// OntologyActionContractEdit is one manual change to an action's execution
// contract.
//
// The row it changes already records who signed the current contract
// (review_by_kwd / review_at_flt / review_note), but that is only the latest
// state: an overwritten note cannot answer "who loosened the authorization last
// month, and from what to what". The action row lives in the document engine as
// one current value, so the history needs its own table.
//
// Only the fields a call actually changed are stored: an edit that touched
// `authorization` must not look like it also rewrote `failure_policy`.
type OntologyActionContractEdit struct {
	ID         string `gorm:"column:id;primaryKey;size:32" json:"id"`
	TenantID   string `gorm:"column:tenant_id;size:32;not null;index:idx_action_contract_edit_scope" json:"tenant_id"`
	TemplateID string `gorm:"column:template_id;size:32;not null;default:'';index:idx_action_contract_edit_scope" json:"template_id"`
	ActionName string `gorm:"column:action_name;size:255;not null;default:'';index:idx_action_contract_edit_scope" json:"action_name"`
	// ActionID is the compiled row id. It is kept for debugging, not for lookup:
	// a recompile hashes new content into a new id, and the name is what stays.
	ActionID   string `gorm:"column:action_id;size:64;not null;default:''" json:"action_id"`
	DatasetID  string `gorm:"column:dataset_id;size:32;not null;default:''" json:"dataset_id"`
	OperatorID string `gorm:"column:operator_id;size:32;not null;default:''" json:"operator_id"`
	// Reason is required by the edit path: a contract change alters what the
	// system may do, so "why" is part of the record rather than a comment.
	Reason string `gorm:"column:reason;type:text" json:"reason"`
	// Before and After hold only the edited fields, as the JSON the contract
	// column stores: {"authorization": {"v": "...", "q": "..."}}.
	Before JSONMap `gorm:"column:before;type:json" json:"before,omitempty"`
	After  JSONMap `gorm:"column:after;type:json" json:"after,omitempty"`
	BaseModel
}

// TableName returns the table name for OntologyActionContractEdit.
func (OntologyActionContractEdit) TableName() string {
	return "ontology_action_contract_edit"
}
