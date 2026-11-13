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

// DatasetModelRef is one compilation model a dataset compiles against.
//
// It is a materialization of the references inside knowledgebase.parser_config,
// which the compiler reads by walking that document: without this table "which
// datasets does changing this model affect" could only be answered by scanning
// every dataset's config and matching group ids in Go. The table is written from
// the same config by the same walker (common.TemplateRefsFromConfig), so the
// answer and the compiler cannot disagree about what a dataset references.
type DatasetModelRef struct {
	ID string `gorm:"column:id;primaryKey;size:32" json:"id"`
	// TenantID is copied from the dataset so the impact query never has to join
	// back to knowledgebase to scope itself.
	TenantID string `gorm:"column:tenant_id;size:32;not null;index:idx_dataset_model_ref_target,priority:1" json:"tenant_id"`
	// RefType is "group" or "template" (common.ModelRefGroup / ModelRefTemplate).
	RefType string `gorm:"column:ref_type;size:16;not null;index:idx_dataset_model_ref_target,priority:2;uniqueIndex:uni_dataset_model_ref,priority:2" json:"ref_type"`
	// ModelRef is the group id or the template id the config names.
	ModelRef  string `gorm:"column:model_ref;size:32;not null;index:idx_dataset_model_ref_target,priority:3;uniqueIndex:uni_dataset_model_ref,priority:3" json:"model_ref"`
	DatasetID string `gorm:"column:dataset_id;size:32;not null;uniqueIndex:uni_dataset_model_ref,priority:1" json:"dataset_id"`
	BaseModel
}

// TableName returns the table name for the DatasetModelRef model.
func (DatasetModelRef) TableName() string {
	return "dataset_model_ref"
}
