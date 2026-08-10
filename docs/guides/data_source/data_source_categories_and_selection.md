---
sidebar_position: 3
title: Data Source Categories and Selection
sidebar_label: Categories and Selection
slug: /add_data_source/data_source_categories_and_selection
sidebar_custom_props: {
  categoryIcon: LucideServer
}
---

# Data Source Categories and Selection

RAGFlow supports many data sources. It is recommended to first select a major category according to the content source, and then go to the corresponding data source configuration. The following categories help users quickly determine which connection method to choose.

**Documents and collaboration platforms**: Used to synchronize team documents, wikis, cloud drive files, and collaborative documents. This category is suitable for building enterprise knowledge bases or project repositories. It includes Confluence, Notion, Google Drive, OneDrive, SharePoint, Box, Dropbox, WebDAV, and SeaFile.

**Object storage**: Used to synchronize large numbers of files in buckets or containers. This category is suitable for historical data archiving, batch file import, and centralized management of cloud files. It includes S3, Google Cloud Storage, Oracle Storage, R2, and Azure Blob Storage.

**Databases and data warehouses**: Used to synchronize structured records or query results. This category is suitable for business data Q&A, metric knowledge retention, and retrieval of data analysis results. It includes MySQL, PostgreSQL, and BigQuery.

**Code repositories and project management**: Used to synchronize code collaboration, issues, pull requests, tasks, and project records. This category is suitable for technical knowledge retention and project history queries. It includes GitHub, GitLab, Bitbucket, Jira, and Asana.

**Email, messages, and collaborative communication**: Used to synchronize emails, channel messages, and team discussion content. This category is suitable for customer service communication, business correspondence, and collaborative record queries. It includes Gmail, Outlook, IMAP, Microsoft Teams, Slack, and Discord.

**Business systems and tables**: Used to synchronize CRM, tickets, online tables, courses, or operation records. This category is suitable for building business ledgers and service knowledge bases. It includes Salesforce, Airtable, Dingtalk AI Table, Zendesk, and Moodle.

**General APIs and subscriptions**: Used to connect custom APIs or public subscription feeds. This category is suitable for business systems without standard connectors and for collecting public content. It includes REST API and RSS.

Account version note: In this manual, **Personal Edition** and **Enterprise Edition** refer to the external system account or service version, not the RAGFlow license. Whether synchronization is available ultimately depends on whether the external system allows the current account to access the target content and API.
