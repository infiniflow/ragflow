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

// OntologyInsight is the measured half of a model: what the corpus actually
// produced, folded into the numbers the workspace reads.
//
// It is a table rather than a live query for one reason. Answering "does my
// declaration match the corpus" means scanning up to twenty thousand rows of a
// dataset that can hold millions; doing that when a tab is opened makes the tab
// cost grow with the corpus, and it makes two readers of the same screen see two
// different numbers while a compile is running. A row written when the compile
// finished is cheap to read, stable between reads, and carries the moment it was
// measured — which is what "as of the last compile" has to mean.
//
// One row covers drift and competency coverage together because both are folds
// of the same scan. Splitting them would double the scan and let the two halves
// disagree about which compile they describe.
type OntologyInsight struct {
	ID         string `gorm:"column:id;primaryKey;size:32" json:"id"`
	TenantID   string `gorm:"column:tenant_id;size:32;not null;uniqueIndex:idx_ontology_insight_scope,priority:1" json:"tenant_id"`
	TemplateID string `gorm:"column:template_id;size:32;not null;uniqueIndex:idx_ontology_insight_scope,priority:2" json:"template_id"`
	DatasetID  string `gorm:"column:dataset_id;size:32;not null;uniqueIndex:idx_ontology_insight_scope,priority:3" json:"dataset_id"`
	// Version is the model version the measurement was taken under. A newer
	// published version does not invalidate the row — the rows being measured
	// were compiled under this one — but a reader has to be told the difference
	// instead of comparing a v7 snapshot against a v8 draft.
	Version int `gorm:"column:version;not null" json:"version"`
	// AsOf is when the scan ran, in milliseconds. It is the only honest answer
	// to "how fresh is this number".
	AsOf int64 `gorm:"column:as_of;not null" json:"as_of"`
	// Payload holds the drift ledger and the per-question coverage. It is kept
	// opaque to the schema: the shape is the workspace's, and every change to it
	// would otherwise be a column change on a table that is rewritten on every
	// compile.
	Payload JSONMap `gorm:"column:payload;type:text;not null" json:"payload"`
	BaseModel
}

// TableName returns the table name for OntologyInsight model
func (OntologyInsight) TableName() string {
	return "ontology_insight"
}
