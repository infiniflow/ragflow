---
sidebar_position: 17
slug: /add_feishu_drive
sidebar_custom_props: {
  categoryIcon: SiGoogledrive
}
---

# Add Feishu/Lark Drive

Integrate Feishu/Lark Drive as a data source.

---

This guide outlines the integration of Feishu (Lark) Drive as a data source for RAGFlow.

The connector syncs **uploaded files** (PDF, Word, Excel, images, and other binaries) from a Feishu/Lark Drive folder, including its subfolders. Feishu-native online documents (Docs, Sheets, Bitable) are not downloadable files and are **not** synced by this connector.

## Prerequisites

Create a self-built app in the Feishu/Lark developer console and obtain the following:

- **App ID** and **App Secret**: From your app's **Credentials & Basic Info** page.
  - Feishu (China): [open.feishu.cn](https://open.feishu.cn)
  - Lark (International): [open.larksuite.com](https://open.larksuite.com)
- **Permission scope**: Add `drive:drive:readonly` (View, comment, and download all files in cloud docs) under **Permissions & Scopes**, then publish a version of the app so the scope takes effect.
- **Folder access**: A tenant-token app can only see content shared with it. Share the target folder with the app, either:
  - directly, by adding the app as a collaborator with read access, or
  - by adding the app's bot to a group and sharing the folder with that group.
- **Folder Token** (optional): Open the target folder and copy the token at the end of its URL: `https://<your-domain>/drive/folder/<folder_token>`.

## Configuration steps

### Define Feishu/Lark Drive as an external data source

Navigate to the **Data sources** section in the RAGFlow user settings and select **Feishu/Lark Drive**. Enter the following in the popup window:

- **Name**: *Required* A unique identifier for this connector (e.g., `Tender-Documents`).
- **App ID**: *Required* Your self-built app's App ID (starts with `cli_`).
- **App Secret**: *Required* Your self-built app's App Secret.
- **Domain**: *Required* The API host that matches your account.
  - `Feishu (open.feishu.cn)` (default): for Feishu/China apps.
  - `Lark (open.larksuite.com)`: for Lark international apps.
- **Folder Token**: *Optional* The folder to sync. Leave empty to use the app's Drive root. The folder must be shared with the app.
- **Batch Size**: *Optional* Number of documents indexed per batch. Defaults to `2`.

Once configuration is complete, click **Confirm** to save your changes.

*RAGFlow validates the connection immediately. A `1061004` permission error means the folder has not been shared with the app, or the `drive:drive:readonly` scope has not been published.*

### Link to a dataset

Credentials alone do not trigger indexing. You must link the data source to a specific dataset:

1. Navigate to the **Dataset** tab.
2. Select or create the target Dataset.
3. Navigate to the Dataset's **Configuration** page and select **Link data source**.
4. Choose the previously created Feishu/Lark Drive connector in the popup window.

Files are pulled in on the connector's refresh schedule; subsequent changes in the folder are synced incrementally.
