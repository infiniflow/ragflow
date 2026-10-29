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

//go:build !ee

package dao

import (
	"context"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// tenantDAOTestExtraModels adds no tables beyond the community schema.
func tenantDAOTestExtraModels() []interface{} { return nil }

// permissionDataModelsEE returns no additional entities in the community
// edition.
func permissionDataModelsEE() []interface{} { return nil }

// billingDataModelsEE returns no billing tables in the community edition.
func billingDataModelsEE() []interface{} { return nil }

// migrateBillingSchemaEE is a no-op in the community edition.
func migrateBillingSchemaEE(ctx context.Context, db *gorm.DB) error { return nil }

// initializeSystemSettingsEE is a no-op in the community edition.
func initializeSystemSettingsEE(ctx context.Context, db *gorm.DB) error { return nil }

// migratePermissionDataEE is a no-op in the community edition.
func migratePermissionDataEE(ctx context.Context, db *gorm.DB) error { return nil }

// initializeTenantDefaultRoleEE is a no-op in the community edition.
func initializeTenantDefaultRoleEE(ctx context.Context, db *gorm.DB, tenant *entity.Tenant) error {
	return nil
}

// ApplyDefaultTenantInviteEnabled is a no-op in the community edition.
func ApplyDefaultTenantInviteEnabled(ctx context.Context, db *gorm.DB, tenant *entity.Tenant) error {
	return nil
}

// TenantModelGroupDAO owns the enterprise model groups. The community edition
// has none.
type TenantModelGroupDAO struct{}

// NewTenantModelGroupDAO creates the model group DAO.
func NewTenantModelGroupDAO() *TenantModelGroupDAO { return &TenantModelGroupDAO{} }

// TenantModelGroupMapDAO owns the model-to-group mappings.
type TenantModelGroupMapDAO struct{}

// NewTenantModelGroupMapDAO creates the model group mapping DAO.
func NewTenantModelGroupMapDAO() *TenantModelGroupMapDAO { return &TenantModelGroupMapDAO{} }

// BillingProductDAO owns the billing product catalogue, which the community
// edition does not have.
type BillingProductDAO struct{}

// NewBillingProductDAO creates the billing product DAO.
func NewBillingProductDAO() *BillingProductDAO { return &BillingProductDAO{} }

// BillingSubscriptionDAO owns tenant subscriptions, which the community
// edition does not have.
type BillingSubscriptionDAO struct{}

// NewBillingSubscriptionDAO creates the billing subscription DAO.
func NewBillingSubscriptionDAO() *BillingSubscriptionDAO { return &BillingSubscriptionDAO{} }
