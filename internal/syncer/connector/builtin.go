//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package connector

import (
	"context"

	"ragflow/internal/dao"
)

// RegisterBuiltIns wires every connector shipped in the server binary. Each
// source registers both the task-context factory (used by the syncer runtime)
// and the raw-config factory (used by the test-connection endpoint).
func RegisterBuiltIns(registry *Registry) {
	registerBuiltIn(registry, "confluence", NewConfluenceConnector)
	registerBuiltIn(registry, "rss", NewRSSConnector)
	registerBuiltIn(registry, "salesforce", NewSalesforceConnector)
	registerBuiltIn(registry, "bitbucket", NewBitbucketConnector)
	registerBuiltIn(registry, "azure_devops", NewAzureDevOpsConnector)
	registerBuiltIn(registry, "dropbox", NewDropboxConnector)
	registerBuiltIn(registry, "box", NewBoxConnector)
	registerBuiltIn(registry, "github", NewGitHubConnector)
	registerBuiltIn(registry, "gitlab", NewGitlabConnector)
	registerBuiltIn(registry, "gmail", NewGmailConnector)
	registerBuiltIn(registry, "google-drive", NewGoogleDriveConnector)
	registerBuiltIn(registry, "google_drive", NewGoogleDriveConnector)
	registerBuiltIn(registry, "google_cloud_storage", NewGoogleCloudStorageConnector)
	registerBuiltIn(registry, "oci_storage", NewOCIStorageConnector)
	registerBuiltIn(registry, "azure_blob", NewAzureBlobStorageConnector)
	registerBuiltIn(registry, "r2", NewR2Connector)
	registerBuiltIn(registry, "s3", NewS3Connector)
	registerBuiltIn(registry, "s3_compatible", NewS3CompatibleConnector)
	registerBuiltIn(registry, "dingtalk_ai_table", NewDingTalkAITableConnector)
	registerBuiltIn(registry, "imap", NewIMAPConnector)
	registerBuiltIn(registry, "jira", NewJiraConnector)
	registerBuiltIn(registry, "outlook", NewOutlookConnector)
	registerBuiltIn(registry, "notion", NewNotionConnector)
	registerBuiltIn(registry, "rest_api", NewRestAPIConnector)
	registerBuiltIn(registry, "xquik", NewXquikConnector)
	registerBuiltIn(registry, "moodle", NewMoodleConnector)
	registerBuiltIn(registry, "mysql", NewMySQLConnector)
	registerBuiltIn(registry, "postgresql", NewPostgreSQLConnector)
	registerBuiltIn(registry, "slack", NewSlackConnector)
	registerBuiltIn(registry, "teams", NewTeamsConnector)
	registerBuiltIn(registry, "sharepoint", NewSharePointConnector)
	registerBuiltIn(registry, "discord", NewDiscordConnector)
	registerBuiltIn(registry, "webdav", NewWebDAVConnector)
}

func registerBuiltIn[T Connector](registry *Registry, source string, factory func(map[string]any) (T, error)) {
	registry.RegisterConfigFactory(source, func(config map[string]any) (Connector, error) {
		return factory(config)
	})
	registry.Register(source, func(ctx context.Context, taskContext dao.SyncTaskContext) (Connector, error) {
		return factory(map[string]any(taskContext.Connector.Config))
	})
}
