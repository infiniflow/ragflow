---
sidebar_position: 4
slug: /add_confluence
sidebar_custom_props: {
  categoryIcon: SiGoogledrive
}
---

# Add Confluence

Integrate Confluence as a data source.

---

This guide outlines the integration of Confluence as a data source for RAGFlow.

## Prerequisites

Before configuring the connector, obtain the following credentials from your Atlassian account:

- **Confluence user Email**: The email address of the account performing the indexing.
- **Atlassian API Token**: Generated via [Atlassian Account Settings](https://id.atlassian.com/manage-profile/security/api-tokens).
- **Confluence base URL**: The instance URL (e.g., `https://<your-org>.atlassian.net/wiki`).

## Configuration steps

### Define Confluence as an external data source

Navigate to the **Connectors** or **External Data Source** section in the RAGFlow Admin Panel and select **Confluence**. Enter the following in the popup window:

- **Is Cloud** - A toggle indicating whether this is a Confluence Cloud instance.
  - `Yes` (default): Confluence Cloud.
  - `False`: Confluence Server/Data Center.
- **Name**: *Required* A unique identifier for your Confluence connector (e.g., `Engineering-Wiki`).
- **Confluence Username**: *Required*
  - For Confluence Cloud: The full email address you use to log into Confluence.
  - For Confluence Server/Data Center: Your login ID, often a shorthand name.
- **Confluence Access Token**: *Required* The authentication key that allows RAGFlow to act on your behalf to read and index your wiki pages.
  - For Confluence Cloud: An Atlassian API Token, a secure string generated from your global Atlassian account. Create one from id.atlassian.com/manage-profile/security/api-tokens.
  - For Confluence Server/Data Center: Your Personal Access Token (PAT). You are required to log in to your company’s Confluence, click on your Profile Picture in the top right, select Settings, then, in the left-hand sidebar, look for Personal Access Tokens.
- **Wiki Base URL**： The base URL of your confluence instance, e.g., https://your-domain.atlassian.net/wiki.
- **Index Mode**
  - `Everything`: (Default) Indexes all pages the provided credentials have access to.
  - `Space`： RAGFlow restricts indexing only to the Space Keys you explicitly list in the configuration.
    - **Space Keys:** Specify keys (e.g., `ENG, HR`) separated by commas to restrict indexing. Leave blank to index all accessible spaces.

Once configuration is complete, click **Confirm** to save your changes.

*RAGFlow validates the connection immediately.*

### Link to a dataset

Credentials alone do not trigger indexing. You must link the data source to a specific dataset:

1. Navigate to the **Dataset** tab.
2. Select or create the target Dataset.
3. Navigate to the Dataset's **Configuration** page and select **Link data source**.
4. Choose the previously created Confluence connector in the popup window.
