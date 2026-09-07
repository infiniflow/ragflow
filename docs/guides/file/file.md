---
sidebar_position: 1
slug: /file_management/files_and_folders
sidebar_label: Files and Folders
title: Files and Folders
sidebar_custom_props: {
  categoryIcon: LucideFolderDot
}
---

# Files and Folders

## Files and Folders

Folders help organize files by project, business topic, or data source. Regular users only need to focus on folders they created or folders they have permission to access.

## Create Folder

RAGFlow file management lets you build your own file system through nested folder structures. To create a folder in the root directory of RAGFlow:

1. In the target directory, click **Add File**.
2. Select **New Folder**.
3. Enter the folder name in the **Name** input field. The name cannot be empty. It is recommended to use a short name that reflects the business source.
4. Click **Save**.

## Upload Files or Folders

After clicking **Add File** in the upper-right corner of the page, select **Upload File** to open the upload window. The upload control contains two tabs: **File** and **Folder**. You can select one or more files, or upload an entire folder.

![Upload files](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/upload_files.jpg)

### Upload File

1. Go to the **File** page and confirm that the current directory is the target save location.
2. Click **Add File**, then select **Upload File**.
3. In the **File** tab, click the upload area to select files, or drag files into the upload area.
4. Confirm the list of files to be uploaded. If you selected a file by mistake, click the delete button on the right side of the file row to remove it.
5. Click **Confirm** or **Save**, then wait for the upload progress to complete.

![Upload file 1](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/upload_file_1.jpg)

### Upload Folder

1. Click **Add File**, then select **Upload File**.
2. Switch to the **Folder** tab.
3. Click the **Upload Folder** area and select a local folder. The system will upload the files inside that folder.
4. After the upload is complete, check the result in the current directory or the corresponding folder.

![Upload file 2](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/upload_file_2.jpg)

The upload process displays progress. If an upload failure message appears, check the file size, network connection, login status, and whether the file format is allowed by the current deployment.

