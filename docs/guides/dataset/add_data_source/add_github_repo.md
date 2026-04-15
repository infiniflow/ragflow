---
sidebar_position: 5
slug: /add_github_repo
sidebar_custom_props: {
  categoryIcon: SiGoogledrive
}
---

# Add GitHub repo

Link your GitHub repofor pull request or issue synchronization.

---

This document explains how to link your GitHub repository to RAGFlow to synchronize pull requests and issues.

## 1. GitHub configuration

Before configuring RAGFlow, you must prepare your GitHub account and generate the necessary credentials.

### Step a: Public email configuration

To ensure smooth identity matching and permission synchronization between your organization and the RAG engine, it is a best practice to make your email visible.

1. Go to your GitHub **Settings** > **Emails**.
2. Uncheck "Keep my email addresses private".
3. Go to **Public profile** and ensure your primary email is selected in the **Public email** dropdown.

### Step b: Generate a personal access token (PAT)

1. Navigate to **Settings** > **Developer settings** > **Personal access tokens** > **Tokens (classic)**.
2. Click **Generate new token (classic)**.
3. **Required scopes:** - **`repo` (Full control):** Essential for accessing private repositories, PRs, and issues.
    - **`read:org` (Optional):** If you are syncing repositories across an entire organization.
    - **`workflow` (Optional):** Recommended if you intend to index GitHub Action logs or CI/CD metadata.
4. **Copy the token:** Save this immediately; it will not be displayed again.

## 2. RAGFlow connector setup

Once your GitHub token is ready, register the external data source within your RAGFlow instance.

1. **Access data sources:** Click on your profile icon in RAGFlow and select **Data source**.
2. **Add GitHub connector:** Click **+ Add** and select the **GitHub** icon.
3. **Input configuration:**
    - **Source name:** Name it based on the repository (e.g., `ragflow-repo`).
    - **Repo owner:** The username or organization (e.g., `infiniflow`).
    - **Repo name:** The repository identifier (e.g., `ragflow`).
    - **Access token:** Paste the PAT generated in section 1.
    - **Include Pull Request** Whether to include pull requests in the selected repo.
    - **Include Issues** Whether to include issues in the selected repo.
4. Click **Save** to confirm your changes.  
   *RAGFlow validates the connection immediately.*

:::tip NOTE
Currently deleted or modified files are not synchronized automatically. This feature is coming soon. Thanks to Gisselle-Gonzalez for requesting [this feature](https://github.com/infiniflow/ragflow/issues/13708).
:::

## 3. Dataset binding & ingestion

Finally, link the connector to a specific knowledge base to begin the RAG process.

1. **Create/select dataset:** Go to the **Dataset** tab and enter your target dataset.
2. **Link external source:** Click **+ Add file** and select **External data source**.
3. **Select GitHub source:** Pick the connector you just created.
4. **Trigger initial sync:** - The files from the repo will appear in your file list.
    - Select the files and click **Run/parsing**.
    - **Parser selection:** For codebases, use the **"Naive"** parser for general text extraction or a specific code-aware template if available in your current version.