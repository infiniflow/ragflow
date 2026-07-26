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

package admin

import (
	"github.com/gin-gonic/gin"
)

func SetupEERouter(engine *gin.Engine) {
}

func RegisterEERouter(protected *gin.RouterGroup, r *Router) {
	// Role management
	protected.GET("/roles", r.handler.ListRoles)
	protected.POST("/roles", r.handler.CreateRole)
	protected.GET("/roles/:role_name", r.handler.ShowRole)
	protected.PUT("/roles/:role_name", r.handler.UpdateRole)
	protected.DELETE("/roles/:role_name", r.handler.DropRole)
	protected.GET("/roles/:role_name/permission", r.handler.ShowRolePermission)
	protected.POST("/roles/:role_name/permission", r.handler.GrantRolePermission)
	protected.DELETE("/roles/:role_name/permission", r.handler.RevokeRolePermission)
	protected.GET("/roles/resource", r.handler.ListResources)
	protected.GET("/roles/permission", r.handler.ListRolesWithPermission)
	protected.GET("/roles/:role_name/default-models", r.handler.ShowRoleDefaultModels)
	protected.PATCH("/roles/:role_name/default-models", r.handler.SetRoleDefaultModel)
	protected.DELETE("/roles/:role_name/default-models", r.handler.ResetRoleDefaultModel)

	// Providers and models
	provider := protected.Group("/providers")
	{
		provider.GET("/", r.handler.ListModelProviders)
		provider.POST("/", r.handler.AddModelProvider)
		provider.GET("/:provider_name", r.handler.ShowProvider)
		provider.DELETE("/", r.handler.DeleteModelProvider)
		provider.GET("/:provider_name/models", r.handler.ListModels)
		provider.GET("/:provider_name/models/:model_name", r.handler.ShowProviderModel)

		provider.POST("/:provider_name/instances", r.handler.AddModelInstance)
		provider.GET("/:provider_name/instances", r.handler.ListModelInstances)
		provider.DELETE("/:provider_name/instances", r.handler.DeleteModelInstance)
		provider.GET("/:provider_name/instances/:instance_name", r.handler.ShowProviderInstance)
		provider.GET("/:provider_name/instances/:instance_name/balance", r.handler.ShowProviderInstanceBalance)
		provider.GET("/:provider_name/instances/:instance_name/connection", r.handler.CheckInstanceConnection)
		provider.POST("/:provider_name/connection", r.handler.CheckProviderConnection)
		provider.PUT("/:provider_name/instances/:instance_name", r.handler.AlterProviderInstance)

		provider.GET("/:provider_name/instances/:instance_name/models", r.handler.ListInstanceModels)
		provider.PATCH("/:provider_name/instances/:instance_name/models/*model_name", r.handler.EnableOrDisableModel)
		provider.POST("/:provider_name/instances/:instance_name/models", r.handler.AddModels)
		provider.DELETE("/:provider_name/instances/:instance_name/models", r.handler.DeleteModels)
	}

	// License
	protected.GET("/system/fingerprint", r.handler.GetSystemFingerprint)
	protected.POST("/system/license", r.handler.SetSystemLicense)
	protected.GET("/system/license", r.handler.ShowSystemLicense)
	protected.PUT("/system/license/config", r.handler.UpdateSystemLicenseConfig)

	protected.GET("/fingerprint", r.handler.GetFingerprint)
	protected.POST("/license", r.handler.SetLicense)
	protected.POST("/license/config", r.handler.UpdateLicenseConfig)
	protected.GET("/license", r.handler.ShowLicense)

	// Token statistics
	protected.GET("/stats/token", r.handler.GetTokenStats)
	protected.GET("/stats/token/users", r.handler.GetTokenUsersStats)
	protected.GET("/stats/token/summary", r.handler.GetTokenStatsSummary)

	// Logs
	protected.GET("/logs", r.handler.ListLogs)

	// Stats data info
	protected.GET("/users/:username/activity", r.handler.ShowUserActivity)
	protected.GET("/users/:username/dataset", r.handler.ShowUserDatasetSummary)
	protected.GET("/users/:username/summary", r.handler.ShowUserSummary)
	protected.GET("/users/:username/storage", r.handler.ShowUserStorage)
	protected.GET("/users/:username/quota", r.handler.ShowUserQuota)
	protected.GET("/users/:username/index", r.handler.ShowUserIndex)
	protected.PUT("/users/:username/role", r.handler.UpdateUserRole)
	protected.GET("/users/:username/permission", r.handler.ShowUserPermission)
	protected.GET("/users/:username/datasets", r.handler.ListUserDatasets)
	protected.GET("/users/:username/agents", r.handler.ListUserAgents)
	protected.GET("/users/:username/chats", r.handler.ListUserChats)
	protected.GET("/users/:username/searches", r.handler.ListUserSearches)
	protected.GET("/users/:username/models", r.handler.ListUserModels)
	protected.GET("/users/:username/files", r.handler.ListUserFiles)
	protected.GET("/users/:username/providers", r.handler.ListUserProviders)
	protected.GET("/users/:username/providers/:provider_name/instances", r.handler.ListUserProviderInstances)
	protected.GET("/users/:username/providers/:provider_name/instances/:instance_name/models", r.handler.ListUserProviderInstanceModels)
	protected.GET("/users/:username/default-models", r.handler.ListUserDefaultModels)
	protected.GET("/users/summary", r.handler.ShowUsersSummary)
	protected.GET("/users/activity", r.handler.ShowUsersActivity)
	protected.GET("/users/reports", r.handler.ListUsersReports)
	protected.GET("/users/storage", r.handler.ListUsersStorage)
	protected.GET("/users/documents", r.handler.ListUsersDocuments)
	protected.GET("/users/index", r.handler.ListUsersIndex)
	protected.GET("/users/quota", r.handler.ListUsersQuota)
	protected.GET("/users/plan/summary", r.handler.ShowUsersPlanSummary)
	protected.GET("/users/plan", r.handler.ShowUsersPlan)
	protected.GET("/users/quota/summary", r.handler.ShowUsersQuotaSummary)
	protected.GET("/ingestion/tasks/summary", r.handler.ShowIngestionTasksSummary)
	protected.GET("/data/summary", r.handler.ShowDataSummary)
	protected.GET("/data/orphan", r.handler.ShowDataOrphan)
	protected.GET("/data/storage", r.handler.ShowDataStorage)
	protected.GET("/data/index", r.handler.ShowDataIndex)
	protected.DELETE("/data/orphan", r.handler.PurgeOrphanData)
	protected.DELETE("/users/:username/data", r.handler.PurgeUserData)
	protected.DELETE("/users/data", r.handler.PurgeUsersData)

	// API Keys
	protected.POST("/users/:username/keys", r.handler.GenerateUserAPIKey)
	protected.DELETE("/users/:username/keys/:key", r.handler.DeleteUserAPIKey)
	protected.GET("/users/:username/keys", r.handler.ListUserAPIKeys)

	protected.GET("/users/:username/tokens", r.handler.ListUserAPITokens)
	//protected.POST("/users/:username/keys", r.handler.GenerateUserAPIToken)
	protected.POST("/users/:username/tokens", r.handler.GenerateUserAPIToken)
	protected.DELETE("/users/:username/tokens/:token", r.handler.DeleteUserAPIToken)

	// Sensitive words, EE
	protected.GET("/sensitive-words", r.handler.DownloadSensitiveWords)
	protected.POST("/sensitive-words", r.handler.UploadSensitiveWords)

	// Verification email, EE
	protected.POST("/email/verification", r.handler.BindVerificationEmail)
	protected.GET("/email/verification", r.handler.ShowVerificationEmail)

	// White list, EE
	protected.GET("/white-list", r.handler.ShowWhiteList)
	protected.POST("/white-list", r.handler.AddWhiteList)
	protected.POST("/white-list/batch", r.handler.BatchAddWhiteList)
	protected.PUT("/white-list/:id", r.handler.UpdateWhiteList)
	protected.DELETE("/white-list/:id", r.handler.DeleteWhiteList)
	protected.DELETE("/white-list/batch", r.handler.BatchDeleteWhiteList)

}
