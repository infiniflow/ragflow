---
sidebar_position: 4
title: Data Source Configuration
sidebar_label: Configuration
slug: /add_data_source/data_source_configuration
sidebar_custom_props: {
  categoryIcon: LucideServer
}
---

# Data Source Configuration
## Confluence

The Confluence data source is used to synchronize spaces, pages, and subpages maintained by a team in Confluence to a RAGFlow knowledge base. After configuration, users can query product documents, project descriptions, R&D standards, and other content in the knowledge base.

**Permission requirements**: Read permissions for Confluence spaces or pages. If attachments or subpages need to be synchronized, read permissions for the corresponding content are also required.

**Account version requirements**: Personal spaces or free team spaces in Confluence Cloud can be used. If enterprise spaces, Server, or Data Center content is synchronized, access permissions for the corresponding organization space are required.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Confluence connection.
- **Confluence username**: Fill in the account used to access Confluence.
- **Confluence access token**: Fill in the access token used to access Confluence.
- **Wiki base URL**: Fill in the base address of the Confluence site.
- **Is cloud version**: Select according to the Confluence deployment type.
- **Index mode**: Select the synchronization scope. The options are **All**, **Space**, and **Page**.
- **Page ID**: Fill this in when the index mode is **Page**.
- **Space key**: Fill this in when the index mode is **Space**.
- **Recursive indexing**: Available when the index mode is **Page**. After it is enabled, the page and its subpages are synchronized.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![Confluence](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Confluence.jpg)

## Notion

The Notion data source is used to synchronize Notion pages and database content to a RAGFlow knowledge base. After configuration, documents, project records, and knowledge cards organized by the team in Notion can be used for retrieval and Q&A.

**Permission requirements**: The Notion integration must be granted access to the target page or database.

**Account version requirements**: Both Notion personal pages and team spaces can be used. As long as the target page or database has been shared with the Notion integration, it can be synchronized to RAGFlow.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Notion connection.
- **Notion integration token**: Fill in the integration token used to access Notion.
- **Root page ID**: Fill in the root page ID that needs to be synchronized.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![Notion](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Notion.jpg)

## Google Drive

The Google Drive data source is used to synchronize files or folders in Google Drive to a RAGFlow knowledge base. After configuration, team cloud documents can be used directly for knowledge base retrieval and Q&A.

**Permission requirements**: The Google account or OAuth authorization must have read permissions for the target files, shared folders, or cloud drives.

**Account version requirements**: Folders that a user is authorized to access in a personal Google Drive can be used. To synchronize organization shared folders or multi-user cloud drives, a Google Workspace administrator or an account with the corresponding authorization is usually required.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Google Drive connection.
- **Primary administrator email**: Fill in the administrator email that has access to the target Drive content.
- **OAuth token JSON**: Fill in or upload the OAuth token JSON used to authorize Google Drive.
- **My Drive email**: Fill in the email address of the user whose **My Drive** needs to be indexed.
- **Shared folder URL**: Fill in the Google Drive shared folder URL that needs to be synchronized.

![Google Drive](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Google_Drive.jpg)

## OneDrive

The OneDrive data source is used to synchronize files in OneDrive or OneDrive for Business to a RAGFlow knowledge base. After configuration, personal or department cloud files can be queried in a unified way.

**Permission requirements**: The Microsoft account or application authorization must have read permissions for the target folders and files.

**Account version requirements**: This is usually configured for Microsoft 365 or Entra ID organization accounts. Personal OneDrive is generally not suitable for the current connection method based on tenant, client ID, and client secret.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this OneDrive connection.
- **Tenant ID**: Fill in the Microsoft Entra ID tenant ID.
- **Client ID**: Fill in the client ID from the app registration.
- **Client secret**: Fill in the client secret from the app registration.
- **Folder path (optional)**: Fill in the OneDrive subfolder path.
- **Batch size**: Set the number of items processed in each batch.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![OneDrive](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/OneDrive.jpg)

## SharePoint

The SharePoint data source is used to synchronize SharePoint site document libraries to a RAGFlow knowledge base. After configuration, enterprise policies, department materials, and project files can be managed and queried in a unified way.

**Permission requirements**: The Microsoft account or application authorization must have read permissions for the target site, document library, and files.

**Account version requirements**: A Microsoft 365 or SharePoint organization site and Entra ID application authorization are required. Personal accounts are not applicable.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this SharePoint connection.
- **Site URL**: Fill in the complete URL of the SharePoint site.
- **Tenant ID**: Fill in the Microsoft Entra ID tenant ID.
- **Client ID**: Fill in the client ID from the app registration.
- **Client secret**: Fill in the client secret from the app registration.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![SharePoint](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/SharePoint.jpg)

## Box

The Box data source is used to synchronize files and folders in Box cloud drive to a RAGFlow knowledge base. After configuration, enterprise cloud drive materials and externally collaborated files can be queried in a unified way.

**Permission requirements**: The Box application or account must have read permissions for the target folders and files.

**Account version requirements**: Both personal Box folders and enterprise Box spaces can be used. Enterprise spaces require the administrator or application authorization to allow access to the target folder.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Box connection.
- **Box OAuth configuration**: Fill in the Box OAuth configuration information.
- **Folder ID**: Fill in the Box folder ID that needs to be synchronized.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![Box](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Box.jpg)

## Dropbox

The Dropbox data source is used to synchronize files and folders in Dropbox to a RAGFlow knowledge base. After configuration, cloud collaboration files can be used for knowledge base retrieval and Q&A.

**Permission requirements**: The Dropbox application or account must have read permissions for the target folders and files.

**Account version requirements**: Both personal Dropbox and team Dropbox can be used. Team spaces require application authorization with read permissions for the target files or folders.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Dropbox connection.
- **Access token**: Fill in the Dropbox access token.
- **Batch size**: Set the number of files synchronized in each batch.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![Dropbox](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Dropbox.jpg)

## SeaFile

The SeaFile data source is used to synchronize account, library, or directory content in Seafile to a RAGFlow knowledge base. After configuration, private cloud drive materials and department files can be retrieved in a unified way.

**Permission requirements**: The SeaFile account must have read permissions for the target libraries, directories, and files.

**Account version requirements**: Both personal libraries and enterprise libraries can be used. When synchronizing shared libraries or specified directories, the corresponding library token or account permissions are required.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this SeaFile connection.
- **SeaFile server URL**: Fill in the SeaFile service address.
- **Synchronization scope**: Select the entire account, a single library, or a specified directory.
- **Account API token**: Fill in the account API token according to the synchronization scope.
- **Include shared libraries**: When synchronizing the entire account, select whether to include shared libraries.
- **Library token**: Fill this in when synchronizing a single library or specified directory.
- **Library ID**: Fill this in when synchronizing a single library or specified directory.
- **Directory path**: Fill this in when the synchronization scope is **Specified directory**.
- **Batch size**: Set the number of items processed in each batch.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![SeaFile](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/SeaFile.jpg)

## S3

The S3 data source is used to synchronize files in an object storage bucket to a RAGFlow knowledge base. After configuration, documents stored in the cloud or in private object storage can be uniformly included in knowledge base management.

**Permission requirements**: The access key, IAM role, or compatible storage account must have bucket listing and object reading permissions.

**Account version requirements**: An AWS S3 or S3-compatible object storage account is required. This type of data source is determined by cloud account and bucket permissions, and does not distinguish between personal and enterprise editions.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this S3 connection.
- **Bucket name**: Fill in the bucket name that needs to be synchronized.
- **Region**: Fill in or select the region where the bucket is located.
- **Prefix**: Used to limit the synchronization path. Leave it empty to synchronize the entire bucket.
- **Mode**: Select **S3** or **S3 Compatible**.
- **Authentication method**: In S3 mode, select access key, IAM role, or AssumeRole.
- **AWS Access Key ID**: Fill this in when selecting access key or S3-compatible mode.
- **AWS Secret Access Key**: Fill this in when selecting access key or S3-compatible mode.
- **Role ARN**: Fill this in when using an IAM role or AssumeRole.
- **Addressing style**: Select virtual-hosted style or path style in S3-compatible mode.
- **Endpoint URL**: Fill in the endpoint address in S3-compatible mode.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

## Google Cloud Storage

The Google Cloud Storage data source is used to synchronize files in a GCS bucket to a RAGFlow knowledge base. After configuration, project materials, reports, and documents on Google Cloud can enter the knowledge base for team retrieval.

**Permission requirements**: The GCS credentials must have listing and object reading permissions for the target bucket.

**Account version requirements**: Google Cloud project and bucket permissions are required. They are usually configured according to cloud project permissions, and do not distinguish between personal and enterprise editions.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Google Cloud Storage connection.
- **GCS access key ID**: Fill in the GCS access key ID.
- **GCS access key**: Fill in the GCS access key.
- **Bucket name**: Fill in the bucket name that needs to be synchronized.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![Google Cloud Storage](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Google_Cloud_Storage.jpg)

## Oracle Storage

The Oracle Storage data source is used to synchronize files in Oracle Cloud Object Storage to a RAGFlow knowledge base. After configuration, documents and archive materials stored on OCI can be retrieved in a unified way.

**Permission requirements**: The OCI credentials must have listing and object reading permissions for the target bucket.

**Account version requirements**: Oracle Cloud tenancy and object storage permissions are required. They are usually configured according to cloud account permissions.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Oracle Storage connection.
- **Namespace**: Fill in the OCI object storage namespace.
- **OCI region**: Fill in the OCI region.
- **OCI access key ID**: Fill in the OCI access key ID.
- **OCI access key**: Fill in the OCI access key.
- **Bucket name**: Fill in the bucket name that needs to be synchronized.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![Oracle Storage](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Oracle_Storage.jpg)

## R2

The R2 data source is used to synchronize files in a Cloudflare R2 bucket to a RAGFlow knowledge base. After configuration, public materials, business files, or historical documents stored in R2 can be queried in a unified way.

**Permission requirements**: The R2 access key must have listing and object reading permissions for the target bucket.

**Account version requirements**: A Cloudflare account and R2 bucket permissions are required. Both personal and enterprise Cloudflare accounts can be used, depending on whether they have access to the target bucket.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this R2 connection.
- **R2 account ID**: Fill in the Cloudflare account ID.
- **R2 access key ID**: Fill in the R2 access key ID.
- **R2 access key**: Fill in the R2 access key.
- **Bucket name**: Fill in the bucket name that needs to be synchronized.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![R2](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/R2.jpg)

## Azure Blob Storage

The Azure Blob Storage data source is used to synchronize files in an Azure Blob container to a RAGFlow knowledge base. After configuration, materials, reports, and archive files in an Azure cloud environment can be queried in a unified way.

**Permission requirements**: The Azure credentials must have read permissions for the target container and blobs.

**Account version requirements**: An Azure storage account, connection string, or SAS authorization is required. They are usually configured according to Azure subscription and storage container permissions.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Azure Blob Storage connection.
- **Authentication mode**: Select account key, connection string, or SAS URL.
- **Account name**: Fill this in when using account key mode.
- **Account key**: Fill this in when using account key mode.
- **Connection string**: Fill this in when using connection string mode.
- **Container URL**: Fill this in when using SAS URL mode.
- **SAS token**: Fill this in when using SAS URL mode.
- **Container name**: Fill in the container name that needs to be synchronized.
- **Prefix (optional)**: Used to limit the synchronization path.
- **Batch size**: Set the number of files processed in each batch.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![Azure Blob Storage](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Azure_Blob_Storage.jpg)

## MySQL

The MySQL data source is used to synchronize table records or query results in MySQL to a RAGFlow knowledge base. After configuration, structured business data can be used for retrieval and Q&A.

**Permission requirements**: The database account must have SELECT permission on the target database tables. A read-only account is recommended.

**Account version requirements**: Both personal self-built databases and enterprise databases can be used. It is recommended to use a read-only account to connect to the target database tables.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this MySQL connection.
- **Host**: Fill in the database host address.
- **Port**: Fill in the database port.
- **Database**: Fill in the database name that needs to be synchronized.
- **Username**: Fill in the database username.
- **Password**: Fill in the database password.
- **SQL query**: Fill in the SQL statement used to read data.
- **Content column**: Select the field to be written as document content.
- **Metadata column**: Select the field to be written as metadata.
- **ID column**: Select the unique ID field.
- **Timestamp column**: Select the field used to determine incremental updates.
- **Sync deleted files**: After this is enabled, deleted records can be removed from the knowledge base index according to the field mapping or cleanup task.

![MySQL](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/MySQL.jpg)

## PostgreSQL

The PostgreSQL data source is used to synchronize table records or query results in PostgreSQL to a RAGFlow knowledge base. After configuration, structured business data can be used for retrieval and Q&A.

**Permission requirements**: The database account must have SELECT permission on the target database tables. A read-only account is recommended.

**Account version requirements**: Both personal self-built databases and enterprise databases can be used. It is recommended to use a read-only account to connect to the target database tables.

**Configuration parameters**: The configuration fields are the same as those for MySQL, including name, host, port, database, username, password, SQL query, content column, metadata column, ID column, timestamp column, and sync deleted files.

![PostgreSQL](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/PostgreSQL.jpg)

## BigQuery

The BigQuery data source is used to synchronize table records or query results in BigQuery to a RAGFlow knowledge base. After configuration, structured data and analysis results in Google Cloud can be retrieved and used for Q&A.

**Permission requirements**: The Google Cloud account or service account must have query and read permissions for the target BigQuery project, dataset, and table.

**Account version requirements**: Google Cloud project and BigQuery permissions are required. They are configured according to cloud project permissions.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this BigQuery connection.
- **Service account JSON**: Fill in or upload the service account JSON used to access BigQuery.
- **Project ID**: Fill in the Google Cloud project ID.
- **Dataset**: Fill in the dataset name.
- **Table**: Fill in the table name.
- **SQL query**: Fill in the SQL statement used to read data.
- **Content column**: Select the field to be written as document content.
- **Metadata column**: Select the field to be written as metadata.
- **ID column**: Select the unique ID field.
- **Timestamp column**: Select the field used to determine incremental updates.
- **Sync deleted files**: After this is enabled, deleted records can be removed from the knowledge base index according to the field mapping or cleanup task.

![BigQuery](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/BigQuery.jpg)

## GitHub

The GitHub data source is used to synchronize issues and pull requests in GitHub repositories to a RAGFlow knowledge base. After configuration, open-source project materials, development discussions, and issue handling records can be queried.

**Permission requirements**: The GitHub token must have read permissions for the target repositories, issues, and pull requests.

**Account version requirements**: Both personal repositories and organization repositories can be used. Organization private repositories require the token to have read permissions for the corresponding repositories, issues, and pull requests.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this GitHub connection.
- **Repository owner**: Fill in the GitHub repository owner.
- **Repository name**: Fill in the GitHub repository name.
- **GitHub access token**: Fill in the GitHub access token.
- **Include Pull Requests**: Select whether to synchronize pull requests.
- **Include Issues**: Select whether to synchronize issues.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![GitHub](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/GitHub.jpg)

## GitLab

The GitLab data source is used to synchronize GitLab repositories, issues, merge requests, and code files to a RAGFlow knowledge base. After configuration, project development records and code-related knowledge can be queried in a unified way.

**Permission requirements**: The GitLab access token must have read permissions for the target project, issues, merge requests, and repository files.

**Account version requirements**: Both personal projects and group projects can be used. Private group projects require the token to have the corresponding read permissions.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this GitLab connection.
- **GitLab instance URL**: Fill in the GitLab service address.
- **Project ID or path**: Fill in the GitLab project ID or project path.
- **Access token**: Fill in the GitLab access token.
- **Include merge requests**: Select whether to synchronize merge requests.
- **Include issues**: Select whether to synchronize issues.
- **Include repository files**: Select whether to synchronize repository files.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![GitLab](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/GitLab.jpg)

## Bitbucket

The Bitbucket data source is used to synchronize Bitbucket repository content, issues, and pull requests to a RAGFlow knowledge base. After configuration, team code collaboration records can be queried.

**Permission requirements**: The Bitbucket account, app password, or token must have read permissions for the target workspace and repository.

**Account version requirements**: Both personal workspaces and team workspaces can be used. Private repositories require authorization with read permissions.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Bitbucket connection.
- **Workspace**: Fill in the Bitbucket workspace name.
- **Repository slug**: Fill in the Bitbucket repository identifier.
- **Username**: Fill in the Bitbucket username.
- **App password or access token**: Fill in the credential used to access Bitbucket.
- **Include pull requests**: Select whether to synchronize pull requests.
- **Include issues**: Select whether to synchronize issues.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![Bitbucket](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Bitbucket.jpg)

## Jira

The Jira data source is used to synchronize issues, comments, and project records in Jira to a RAGFlow knowledge base. After configuration, project tasks, requirements, bugs, and handling records can be queried.

**Permission requirements**: The Jira account or API token must have read permissions for the target project and issues.

**Account version requirements**: Jira Cloud and Jira Server or Data Center can be used. Enterprise projects require the account to have access permissions for the corresponding project.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Jira connection.
- **Jira base URL**: Fill in the Jira site address.
- **Email or username**: Fill in the Jira account.
- **API token or password**: Fill in the Jira API token or password.
- **Project key**: Fill in the Jira project key that needs to be synchronized.
- **JQL**: Fill in the JQL condition used to limit the synchronization scope.
- **Sync comments**: Select whether to synchronize issue comments.
- **Sync attachments**: Select whether to synchronize attachments.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![Jira](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Jira.jpg)

## Asana

The Asana data source is used to synchronize tasks, projects, and comments in Asana to a RAGFlow knowledge base. After configuration, team task records and project progress can be queried.

**Permission requirements**: The Asana account or access token must have read permissions for the target workspace, project, and tasks.

**Account version requirements**: Both personal projects and organization workspaces can be used. Organization workspaces require the account to have access permissions for the target project.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Asana connection.
- **Access token**: Fill in the Asana access token.
- **Workspace ID**: Fill in the Asana workspace ID.
- **Project ID**: Fill in the Asana project ID that needs to be synchronized.
- **Sync comments**: Select whether to synchronize task comments.
- **Sync attachments**: Select whether to synchronize attachments.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![Asana](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Asana.jpg)

## Gmail

The Gmail data source is used to synchronize Gmail email content to a RAGFlow knowledge base. After configuration, customer service emails, business correspondence, and historical communication records can be queried and reused.

**Permission requirements**: Google OAuth authorization must have read permissions for the target mailbox emails.

**Account version requirements**: The current interface includes a primary administrator email and OAuth JSON, which is more suitable for Google Workspace administrators or organization mailbox synchronization. Ordinary personal Gmail is recommended only after the actual authorization flow supports it.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Gmail connection.
- **Primary administrator email**: Fill in the Google Workspace administrator email.
- **OAuth JSON**: Fill in or upload the OAuth JSON used for Gmail authorization.
- **User email**: Fill in the mailbox address that needs to be synchronized.
- **Label or folder**: Fill in the Gmail label or folder scope that needs to be synchronized.
- **Batch size**: Set the number of emails processed in each batch.
- **Sync deleted files**: After this is enabled, emails deleted from the external system are removed from the knowledge base index.

![Gmail](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Gmail.jpg)

## Outlook

The Outlook data source is used to synchronize Outlook mailbox emails to a RAGFlow knowledge base. After configuration, business emails and communication records in Microsoft 365 can be queried.

**Permission requirements**: The Microsoft account or application authorization must have read permissions for the target mailbox emails.

**Account version requirements**: Microsoft 365 or Outlook organization accounts are recommended. Personal Outlook depends on whether the authorization flow and API permissions are available.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Outlook connection.
- **Tenant ID**: Fill in the Microsoft Entra ID tenant ID.
- **Client ID**: Fill in the client ID from the app registration.
- **Client secret**: Fill in the client secret from the app registration.
- **Mailbox address**: Fill in the mailbox address that needs to be synchronized.
- **Folder**: Fill in the mail folder that needs to be synchronized.
- **Batch size**: Set the number of emails processed in each batch.
- **Sync deleted files**: After this is enabled, emails deleted from the external system are removed from the knowledge base index.

![Outlook](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Outlook.jpg)

## IMAP

The IMAP data source is used to synchronize email content in mail services that support the IMAP protocol to a RAGFlow knowledge base. After configuration, mailboxes that are not covered by standard email connectors can also be included in knowledge base retrieval.

**Permission requirements**: The mailbox account must enable IMAP and have read permissions for the target folder.

**Account version requirements**: Personal mailboxes and enterprise mailboxes can both be used, as long as the mail service supports IMAP login and the account has the required permissions.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this IMAP connection.
- **IMAP server**: Fill in the IMAP server address.
- **Port**: Fill in the IMAP port.
- **Username**: Fill in the mailbox username.
- **Password or authorization code**: Fill in the mailbox password or application authorization code.
- **Mailbox folder**: Fill in the folder that needs to be synchronized.
- **SSL/TLS**: Select whether to enable secure connection.
- **Batch size**: Set the number of emails processed in each batch.
- **Sync deleted files**: After this is enabled, emails deleted from the external system are removed from the knowledge base index.

![IMAP](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/IMAP.jpg)

## Microsoft Teams

The Microsoft Teams data source is used to synchronize Teams channel messages and related collaboration records to a RAGFlow knowledge base. After configuration, team discussion content can be retrieved and reused.

**Permission requirements**: The Microsoft account or application authorization must have read permissions for the target team, channel, and messages.

**Account version requirements**: Microsoft 365 organization accounts and Teams permissions are required. Personal accounts are generally not applicable.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Microsoft Teams connection.
- **Tenant ID**: Fill in the Microsoft Entra ID tenant ID.
- **Client ID**: Fill in the client ID from the app registration.
- **Client secret**: Fill in the client secret from the app registration.
- **Team ID**: Fill in the Teams team ID that needs to be synchronized.
- **Channel ID**: Fill in the channel ID that needs to be synchronized.
- **Batch size**: Set the number of messages processed in each batch.
- **Sync deleted files**: After this is enabled, messages deleted from the external system are removed from the knowledge base index.

![Microsoft Teams](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Microsoft_Teams.jpg)

## Slack

The Slack data source is used to synchronize Slack channel messages and collaboration content to a RAGFlow knowledge base. After configuration, team communication records can be retrieved and queried.

**Permission requirements**: The Slack application or token must have read permissions for the target workspace and channels.

**Account version requirements**: Both personal workspaces and enterprise workspaces can be used. Private channels require the application to be invited or granted the corresponding permissions.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Slack connection.
- **Slack bot token**: Fill in the Slack bot token.
- **Workspace ID**: Fill in the Slack workspace ID.
- **Channel IDs**: Fill in the channels that need to be synchronized.
- **Batch size**: Set the number of messages processed in each batch.
- **Sync deleted files**: After this is enabled, messages deleted from the external system are removed from the knowledge base index.

![Slack](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Slack.jpg)

## Discord

The Discord data source is used to synchronize Discord server and channel messages to a RAGFlow knowledge base. After configuration, community communication records can be queried and analyzed.

**Permission requirements**: The Discord bot must have permission to read the target servers and channels.

**Account version requirements**: Personal servers and community servers can both be used, as long as the bot has been added to the target server and granted message reading permissions.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Discord connection.
- **Discord bot token**: Fill in the Discord bot access token.
- **Server IDs**: Fill in the list of Discord server IDs that need to be synchronized.
- **Channels**: Fill in the Discord channel IDs or channel names that need to be synchronized.
- **Batch size**: Set the number of messages processed in each batch.
- **Sync deleted files**: After this is enabled, messages deleted from the external system are removed from the knowledge base index.

![Discord](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Discord.jpg)

## Dingtalk AI Table

The Dingtalk AI Table data source is used to synchronize records in Dingtalk AI Table to a RAGFlow knowledge base. After configuration, online table records, business ledgers, and collaborative data can be queried.

**Permission requirements**: The Dingtalk account or application authorization must have read permissions for the target AI Table.

**Account version requirements**: Dingtalk organization accounts and AI Table permissions are required. Whether it can be synchronized depends on the organization permissions and application authorization.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Dingtalk AI Table connection.
- **Application key**: Fill in the Dingtalk application key.
- **Application secret**: Fill in the Dingtalk application secret.
- **Table ID**: Fill in the AI Table ID that needs to be synchronized.
- **View ID**: Fill in the view ID that needs to be synchronized.
- **Content field**: Select the field to be written as document content.
- **Metadata field**: Select the field to be written as metadata.
- **Sync deleted files**: After this is enabled, deleted records are removed from the knowledge base index.

![Dingtalk AI Table](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Dingtalk_AI_Table.jpg)

## Zendesk

The Zendesk data source is used to synchronize Zendesk tickets, comments, and knowledge base content to a RAGFlow knowledge base. After configuration, customer service records and support knowledge can be queried in a unified way.

**Permission requirements**: The Zendesk account, API token, or application authorization must have read permissions for the target tickets and knowledge base content.

**Account version requirements**: Zendesk team or enterprise accounts can be used. The account must have access permissions for the target tickets or help center content.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Zendesk connection.
- **Subdomain**: Fill in the Zendesk subdomain.
- **Email**: Fill in the Zendesk account email.
- **API token**: Fill in the Zendesk API token.
- **Synchronization scope**: Select tickets, help center articles, or both.
- **Batch size**: Set the number of records processed in each batch.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

## Moodle

The Moodle data source is used to synchronize course content, forums, resources, and learning records in Moodle to a RAGFlow knowledge base. After configuration, course materials and teaching records can be queried.

**Permission requirements**: The Moodle account or token must have read permissions for the target courses, activities, and resources.

**Account version requirements**: Self-built Moodle and organization Moodle platforms can be used. The account must have access permissions for the target courses.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this Moodle connection.
- **Moodle site URL**: Fill in the Moodle service address.
- **Access token**: Fill in the Moodle access token.
- **Course ID**: Fill in the course ID that needs to be synchronized.
- **Synchronization scope**: Select course resources, forums, assignments, or other content.
- **Batch size**: Set the number of records processed in each batch.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![Moodle](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/Moodle.jpg)

## REST API

The REST API data source is used to synchronize data returned by custom business APIs to a RAGFlow knowledge base. After configuration, records in internal systems or third-party systems can enter the knowledge base according to the API structure.

**Permission requirements**: The API credentials must have permission to call the target API, and the data returned by the API must be readable.

**Account version requirements**: Personal or enterprise custom systems can both be used, provided that the target API is accessible and provides a stable data structure and authentication method.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this REST API connection.
- **Request URL**: Fill in the API request address.
- **Request method**: Select GET, POST, or another request method.
- **Request headers**: Fill in the header information required for API authentication or requests.
- **Request body**: Fill in the request body when POST or other methods require it.
- **Pagination method**: Configure the pagination method to ensure that the next page of data can be continuously read.
- **Content field**: Specify the content field returned by the API.
- **Metadata field**: Specify the metadata field returned by the API.
- **Unique ID field**: Specify the unique ID field of each record.
- **Sync deleted files**: After this is enabled, deleted records are removed from the knowledge base index according to the synchronization or cleanup task.

![REST API](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/REST_API.jpg)

## RSS

The RSS data source is used to subscribe public websites, blogs, announcements, or product updates to a RAGFlow knowledge base. After configuration, the knowledge base can continuously obtain new content from the subscription source, making it convenient for users to view and ask questions in a unified way.

**Permission requirements**: The subscription source must be accessible. If the subscription source is protected, make sure the access address or credentials have read permissions.

**Account version requirements**: Public subscription sources can usually be used directly. If the RSS address is protected, provide an account or access address that can access the subscription source.

**Configuration parameters**:

- **Name**: Customize the name in RAGFlow to identify this RSS connection.
- **Feed URL**: Fill in the RSS or Atom subscription address on the page that needs to be synchronized.
- **Batch size**: Set the number of subscription entries processed in each batch.
- **Sync deleted files**: After this is enabled, content deleted from the external system is removed from the knowledge base index.

![RSS](https://raw.githubusercontent.com/infiniflow/ragflow-docs/2ee87008723d56cb6ebf0e9c92f6ef2ad1a45254/images/RSS.jpg)
