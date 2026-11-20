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

package entity

// OntologyDriftDismissal records a finding the operator has seen and decided not
// to act on, together with why.
//
// Without it the same undeclared class comes back after every compile and the
// panel trains its reader to ignore the whole list. The row is scoped to one
// model × one dataset because the same class can be a real gap in one corpus and
// an artefact of another: dismissing it in the first must not silence it in the
// second.
//
// It stores a decision, not a measurement: recompiling a dataset must not erase
// it, which is why it lives outside the insight row.
type OntologyDriftDismissal struct {
	ID         string `gorm:"column:id;primaryKey;size:32" json:"id"`
	TenantID   string `gorm:"column:tenant_id;size:32;not null;uniqueIndex:idx_ontology_drift_dismissal_scope,priority:1" json:"tenant_id"`
	TemplateID string `gorm:"column:template_id;size:32;not null;uniqueIndex:idx_ontology_drift_dismissal_scope,priority:2" json:"template_id"`
	DatasetID  string `gorm:"column:dataset_id;size:32;not null;uniqueIndex:idx_ontology_drift_dismissal_scope,priority:3" json:"dataset_id"`
	ClassName  string `gorm:"column:class_name;size:255;not null;uniqueIndex:idx_ontology_drift_dismissal_scope,priority:4" json:"class_name"`
	// Reason is required. A dismissal without one is indistinguishable from
	// forgetting, and the next reader cannot tell whether it still holds.
	Reason string `gorm:"column:reason;type:text;not null" json:"reason"`
	// DismissedBy is the operator who decided; the audit answer to "who silenced
	// this".
	DismissedBy string `gorm:"column:dismissed_by;size:32" json:"dismissed_by,omitempty"`
	BaseModel
}

// TableName returns the table name for OntologyDriftDismissal model
func (OntologyDriftDismissal) TableName() string {
	return "ontology_drift_dismissal"
}
