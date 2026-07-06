---
sidebar_position: 5
slug: /add_notion
sidebar_custom_props: {
  categoryIcon: SiGoogledrive
}
---

# Add Notion

Connecting your Notion workspace to RAGFlow allows you to ingest and sync your notes, databases, and documents directly into your dataset. Once configured, RAGFlow fetches data from the specified Notion pages to provide context for your RAG applications.

## Prerequisites

Before you begin, ensure you have:
* A Notion account with **Workspace Owner** permissions (required to create integrations).
* The specific pages or databases you intend to sync.

---

## Create an internal integration

To allow RAGFlow to access your Notion data, you must first create an internal integration in the Notion developer portal to generate a secret token.

1. Navigate to the [Notion My Integrations](https://www.notion.com/my-integrations) page.
2. Click **+ New integration**.
3. In the **Name** field, enter a name (e.g., "RAGFlow Connector").
4. Select the **Associated workspace** where your data resides.
5. Under **Capabilities**, ensure **Read content** is selected. RAGFlow does not require write or user-related permissions.
6. Click **Submit**.
7. Under the **Secrets** tab, click **Show** and then **Copy** to save your **Internal Integration Token**.

---

## Grant access to your pages

By default, an integration has no access to any pages in your workspace. You must explicitly share the pages you want RAGFlow to index.

1. Open the Notion page or database you wish to use as the root of your data source.
2. Click the **...** (three dots) menu in the top-right corner.
3. Scroll down to **Connect to** (or **Add connections**).
4. Search for the integration you created (e.g., "RAGFlow Connector") and select it.
5. Confirm the connection when prompted.

:::tip NOTE
If you share a parent page, all its nested child pages and databases will automatically be accessible to the integration.
:::

---

## Identify the root page id

The **Root Page Id** tells RAGFlow where to start indexing. You can find this in the URL of your Notion page.

1. Open your target root page in a web browser.
2. Look at the URL in the address bar. The page ID is the 32-character alphanumeric string at the end of the URL.
   * **Format:** `https://www.notion.so/workspace-name/Page-Title-`**`11a047149aef80578303e705001bb90e`**
3. Copy only the 32-character string (exclude any parameters following a `?`).

---

## Configure the notion connector in RAGFlow

Once you have your token and ID, add the connector within the RAGFlow interface.

| Field | Description | Required |
| :--- | :--- | :--- |
| **Name**                     | A unique label for this data source (e.g., `Engineering Wiki`).             | Yes |
| **Notion Integration Token** | The "Internal Integration Secret" copied from your Notion developer portal. | Yes |
| **Root Page Id**             | The 32-character ID of the top-level page you want to sync.                 | No  |

Once configuration is complete, click **Confirm** to save your changes.

*RAGFlow validates the connection immediately.*

### Link to a dataset

Credentials alone do not trigger indexing. You must link the data source to a specific dataset:

1. Navigate to the **Dataset** tab.
2. Select or create the target Dataset.
3. Navigate to the Dataset's **Configuration** page and select **Link data source**.
4. Choose the previously created Notion connector in the popup window.