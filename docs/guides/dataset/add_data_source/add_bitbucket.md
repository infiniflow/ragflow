---
sidebar_position: 16
slug: /add_confluence
sidebar_custom_props: {
  categoryIcon: SiGoogledrive
}
---

# Add Bitbucket

Integrate Bitbucket as a data source.

---

This guide outlines the integration of Bitbucket as a data source for RAGFlow.

## Prerequisites

Before starting, ensure you have the following:

- **Bitbucket API token:** A Personal Access Token (PAT) with the appropriate scopes or permissions.
- **Repository URL:** The full URL of the repository you wish to index.
- **Workspace ID:** The unique identifier for your Bitbucket workspace.

## Configuration steps

### Define Bitbucket as an external data source

Navigate to the **Connectors** or **External Data Source** section in the RAGFlow Admin Panel and select **Bitbucket**. Fill in the connector details in the popup window:

- **Name**: A descriptive name for this connector.
- **Bitbucket Account Email**: The email address for your Bitbucket account.
- **Bitbucket API Token**: The API token with proper permissions created in the previous step.
- **Workspace** The `WORKSPACE_NAME` from your Bitbucket URL, e.g., `https://bitbucket.org/{WORKSPACE_NAME}/...`
- **Index Mode**
  - **Workspace**: (Default) Indexes all repositories in the workspace.
  - **Repositories**: Indexes specified repositories in the workspace.
    - **Repository Slugs**: A comma-separated list of repository slugs, e.g., `repo2,repo2`.
  - **Projects**: Indexes specified projects in the workspace.
    - **Projects**: A comma-separated list of project keys, e.g., `PROJ1,PROJ2`.

*RAGFlow validates the connection immediately and indexes all pull requests from the specified repos or projects.*

### Link to a dataset

Credentials alone do not trigger indexing. You must link the data source to a specific dataset:

1. Navigate to the **Dataset** tab.
2. Select or create the target Dataset.
3. Navigate to the Dataset's **Configuration** page and select **Link data source**.
4. Choose the previously created Bitbucket connector in the popup window.