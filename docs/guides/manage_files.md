---
sidebar_position: 6
slug: /manage_files
---

# Files

Knowledge base, hallucination-free chat, and file management are the three pillars of RAGFlow. RAGFlow's file management allows you to upload files individually or in bulk. You can then link an uploaded file to multiple target knowledge bases. This guide showcases some basic usages of the file management feature.

:::info IMPORTANT
Compared to uploading files directly to various knowledge bases, uploading them to RAGFlow's file management and then linking them to different knowledge bases is *not* an unnecessary step, particularly when you want to delete some parsed files or an entire knowledge base but retain the original files.
:::

## Create folder

RAGFlow's file management allows you to establish your file system with nested folder structures. To create a folder in the root directory of RAGFlow: 

![create new folder](https://github.com/infiniflow/ragflow/assets/93570324/3a37a5f4-43a6-426d-a62a-e5cd2ff7a533)

:::caution NOTE
Each knowledge base in RAGFlow has a corresponding folder under the **root/.knowledgebase** directory. You are not allowed to create a subfolder within it.
:::

## Upload file

RAGFlow's file management supports file uploads from your local machine, allowing both individual and bulk uploads: 

![upload file](https://github.com/infiniflow/ragflow/assets/93570324/5d7ded14-ce2b-4703-8567-9356a978f45c)

![bulk upload](https://github.com/infiniflow/ragflow/assets/93570324/def0db55-824c-4236-b809-a98d8c8674e3)

## Preview file

RAGFlow's file management supports previewing files in the following formats:

- Documents (PDF, DOCS)
- Tables (XLSX)
- Pictures (JPEG, JPG, PNG, TIF, GIF)

![preview](https://github.com/infiniflow/ragflow/assets/93570324/2e931362-8bbf-482c-ac86-b68b09d331bc)

## Link file to knowledge bases

RAGFlow's file management allows you to *link* an uploaded file to multiple knowledge bases, creating a file reference in each target knowledge base. Therefore, deleting a file in your file management will AUTOMATICALLY REMOVE all related file references across the knowledge bases.

![link knowledgebase](https://github.com/infiniflow/ragflow/assets/93570324/6c6b8db4-3269-4e35-9434-6089887e3e3f)

You can link your file to one knowledge base or multiple knowledge bases at one time: 

![link multiple kb](https://github.com/infiniflow/ragflow/assets/93570324/6c508803-fb1f-435d-b688-683066fd7fff)

## Move file to a specific folder

![move files](https://github.com/user-attachments/assets/3a2db469-6811-4ea0-be80-403b61ffe257)

## Search files or folders

**File Management** only supports file name and folder name filtering in the current directory (files or folders in the child directory will not be retrieved).

![search file](https://github.com/infiniflow/ragflow/assets/93570324/77ffc2e5-bd80-4ed1-841f-068e664efffe)

## Rename file or folder

RAGFlow's file management allows you to rename a file or folder:

![rename_file](https://github.com/infiniflow/ragflow/assets/93570324/5abb0704-d9e9-4b43-9ed4-5750ccee011f)


## Delete files or folders

RAGFlow's file management allows you to delete files or folders individually or in bulk. 

To delete a file or folder: 

![delete file](https://github.com/infiniflow/ragflow/assets/93570324/85872728-125d-45e9-a0ee-21e9d4cedb8b)

To bulk delete files or folders:

![bulk delete](https://github.com/infiniflow/ragflow/assets/93570324/519b99ab-ec7f-4c8a-8cea-e0b6dcb3cb46)

> - You are not allowed to delete the **root/.knowledgebase** folder. 
> - Deleting files that have been linked to knowledge bases will **AUTOMATICALLY REMOVE** all associated file references across the knowledge bases.

## Download uploaded file

RAGFlow's file management allows you to download an uploaded file:

![download_file](https://github.com/infiniflow/ragflow/assets/93570324/cf3b297f-7d9b-4522-bf5f-4f45743e4ed5)

> As of RAGFlow v0.20.4, bulk download is not supported, nor can you download an entire folder. 
