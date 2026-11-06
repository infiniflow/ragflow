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

// The model lifecycle a compilation template moves through. It is a closed set:
// the publish endpoints accept exactly these transitions and reject anything
// else, so an unknown value in the column can only come from a direct database
// write and is treated as a configuration error rather than a state.
const (
	// PublishStatusDraft is an editable model that nothing compiles against yet.
	// A template that has never been published is also a draft.
	PublishStatusDraft = "draft"
	// PublishStatusReview is a draft submitted for approval. It is still not
	// what compilation reads; only a published version is.
	PublishStatusReview = "review"
	// PublishStatusPublished is a model with an immutable snapshot that
	// compilation reads.
	PublishStatusPublished = "published"
	// PublishStatusRetired is a published model withdrawn from use. Its
	// snapshots stay readable so assertions extracted under them stay
	// explainable.
	PublishStatusRetired = "retired"
)

// CompilationTemplateVersion is one immutable published snapshot of a
// compilation template's config.
//
// Publishing copies the working config here and increments the template's
// Version; nothing ever updates a snapshot row afterwards. That is the whole
// point of the table: an assertion can name the model version it was extracted
// under, and a later edit cannot silently rewrite what that version said.
//
// The working config stays on the template row, so a snapshot is what was
// published and not necessarily what is being edited right now — the two differ
// exactly while a draft is in progress, which is the state the model library
// shows as "unpublished changes".
type CompilationTemplateVersion struct {
	ID         string `gorm:"column:id;primaryKey;size:32" json:"id"`
	TemplateID string `gorm:"column:template_id;size:32;not null;uniqueIndex:idx_template_version" json:"template_id"`
	TenantID   string `gorm:"column:tenant_id;size:32;index" json:"tenant_id"`
	Version    int    `gorm:"column:version;not null;uniqueIndex:idx_template_version" json:"version"`
	// Name and Kind are copied, not referenced: a snapshot has to stay readable
	// after the template is renamed or its kind is corrected.
	Name   string  `gorm:"column:name;size:128;not null" json:"name"`
	Kind   string  `gorm:"column:kind;size:64;not null;index" json:"kind"`
	Config JSONMap `gorm:"column:config;type:text;not null" json:"config"`
	// Note is the change description entered when the version was published. It
	// is what the version list shows as "what changed and why".
	Note        *string `gorm:"column:note;type:text" json:"note,omitempty"`
	PublishedBy *string `gorm:"column:published_by;size:64" json:"published_by,omitempty"`
	PublishedAt *int64  `gorm:"column:published_at" json:"published_at,omitempty"`
	BaseModel
}

// TableName returns the table name for CompilationTemplateVersion model
func (CompilationTemplateVersion) TableName() string {
	return "compilation_template_version"
}
