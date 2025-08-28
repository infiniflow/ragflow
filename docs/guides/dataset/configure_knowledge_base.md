---
sidebar_position: -1
slug: /configure_knowledge_base
---

# Configure knowledge base

Knowledge base, hallucination-free chat, and file management are the three pillars of RAGFlow. RAGFlow's AI chats are based on knowledge bases. Each of RAGFlow's knowledge bases serves as a knowledge source, *parsing* files uploaded from your local machine and file references generated in **File Management** into the real 'knowledge' for future AI chats. This guide demonstrates some basic usages of the knowledge base feature, covering the following topics:

- Create a knowledge base
- Configure a knowledge base
- Search for a knowledge base
- Delete a knowledge base

## Create knowledge base

With multiple knowledge bases, you can build more flexible, diversified question answering. To create your first knowledge base:

![create knowledge base](https://github.com/infiniflow/ragflow/assets/93570324/110541ed-6cea-4a03-a11c-414a0948ba80)

_Each time a knowledge base is created, a folder with the same name is generated in the **root/.knowledgebase** directory._

## Configure knowledge base

The following screenshot shows the configuration page of a knowledge base. A proper configuration of your knowledge base is crucial for future AI chats. For example, choosing the wrong embedding model or chunking method would cause unexpected semantic loss or mismatched answers in chats. 

![knowledge base configuration](https://github.com/infiniflow/ragflow/assets/93570324/384c671a-8b9c-468c-b1c9-1401128a9b65)

This section covers the following topics:

- Select chunking method
- Select embedding model
- Upload file
- Parse file
- Intervene with file parsing results
- Run retrieval testing

### Select chunking method

RAGFlow offers multiple chunking template to facilitate chunking files of different layouts and ensure semantic integrity. In **Chunking method**, you can choose the default template that suits the layouts and formats of your files. The following table shows the descriptions and the compatible file formats of each supported chunk template:

| **Template** | Description                                                           | File format                                                                                   |
|--------------|-----------------------------------------------------------------------|-----------------------------------------------------------------------------------------------|
| General      | Files are consecutively chunked based on a preset chunk token number. | MD, MDX, DOCX, XLSX, XLS (Excel 97-2003), PPT, PDF, TXT, JPEG, JPG, PNG, TIF, GIF, CSV, JSON, EML, HTML |
| Q&A          |                                                                       | XLSX, XLS (Excel 97-2003), CSV/TXT                                                             |
| Resume       | Enterprise edition only. You can also try it out on demo.ragflow.io.  | DOCX, PDF, TXT                                                                                |
| Manual       |                                                                       | PDF                                                                                           |
| Table        |                                                                       | XLSX, XLS (Excel 97-2003), CSV/TXT                                                             |
| Paper        |                                                                       | PDF                                                                                           |
| Book         |                                                                       | DOCX, PDF, TXT                                                                                |
| Laws         |                                                                       | DOCX, PDF, TXT                                                                                |
| Presentation |                                                                       | PDF, PPTX                                                                                     |
| Picture      |                                                                       | JPEG, JPG, PNG, TIF, GIF                                                                      |
| One          | Each document is chunked in its entirety (as one).                    | DOCX, XLSX, XLS (Excel 97-2003), PDF, TXT                                                      |
| Tag          | The knowledge base functions as a tag set for the others.             | XLSX, CSV/TXT                                                                                 |

You can also change a file's chunking method on the **Datasets** page.

![change chunking method](https://github.com/infiniflow/ragflow/assets/93570324/ac116353-2793-42b2-b181-65e7082bed42)

### Select embedding model

An embedding model converts chunks into embeddings. It cannot be changed once the knowledge base has chunks. To switch to a different embedding model, you must delete all existing chunks in the knowledge base. The obvious reason is that we *must* ensure that files in a specific knowledge base are converted to embeddings using the *same* embedding model (ensure that they are compared in the same embedding space).

The following embedding models can be deployed locally:

- BAAI/bge-large-zh-v1.5
- maidalun1020/bce-embedding-base_v1

:::danger IMPORTANT
These two embedding models are optimized specifically for English and Chinese, so performance may be compromised if you use them to embed documents in other languages.
:::

### Upload file

- RAGFlow's **File Management** allows you to link a file to multiple knowledge bases, in which case each target knowledge base holds a reference to the file.
- In **Knowledge Base**, you are also given the option of uploading a single file or a folder of files (bulk upload) from your local machine to a knowledge base, in which case the knowledge base holds file copies. 

While uploading files directly to a knowledge base seems more convenient, we *highly* recommend uploading files to **File Management** and then linking them to the target knowledge bases. This way, you can avoid permanently deleting files uploaded to the knowledge base. 

### Parse file

File parsing is a crucial topic in knowledge base configuration. The meaning of file parsing in RAGFlow is twofold: chunking files based on file layout and building embedding and full-text (keyword) indexes on these chunks. After having selected the chunking method and embedding model, you can start parsing a file:

![parse file](https://github.com/infiniflow/ragflow/assets/93570324/5311f166-6426-447f-aa1f-bd488f1cfc7b)

- Click the play button next to **UNSTART** to start file parsing.
- Click the red-cross icon and then refresh, if your file parsing stalls for a long time. 
- As shown above, RAGFlow allows you to use a different chunking method for a particular file, offering flexibility beyond the default method. 
- As shown above, RAGFlow allows you to enable or disable individual files, offering finer control over knowledge base-based AI chats. 

### Intervene with file parsing results

RAGFlow features visibility and explainability, allowing you to view the chunking results and intervene where necessary. To do so: 

1. Click on the file that completes file parsing to view the chunking results: 

   _You are taken to the **Chunk** page:_

   ![chunks](https://github.com/infiniflow/ragflow/assets/93570324/0547fd0e-e71b-41f8-8e0e-31649c85fd3d)

2. Hover over each snapshot for a quick view of each chunk.

3. Double-click the chunked texts to add keywords or make *manual* changes where necessary:

   ![update chunk](https://github.com/infiniflow/ragflow/assets/93570324/1d84b408-4e9f-46fd-9413-8c1059bf9c76)

:::caution NOTE
You can add keywords to a file chunk to increase its ranking for queries containing those keywords. This action increases its keyword weight and can improve its position in search list.  
:::

4. In Retrieval testing, ask a quick question in **Test text** to double-check if your configurations work:

   _As you can tell from the following, RAGFlow responds with truthful citations._

   ![retrieval test](https://github.com/infiniflow/ragflow/assets/93570324/c03f06f6-f41f-4b20-a97e-ae405d3a950c)

### Run retrieval testing

RAGFlow uses multiple recall of both full-text search and vector search in its chats. Prior to setting up an AI chat, consider adjusting the following parameters to ensure that the intended information always turns up in answers:

- Similarity threshold: Chunks with similarities below the threshold will be filtered. By default, it is set to 0.2.
- Vector similarity weight: The percentage by which vector similarity contributes to the overall score. By default, it is set to 0.3.

See [Run retrieval test](./run_retrieval_test.md) for details.

![retrieval test](https://github.com/infiniflow/ragflow/assets/93570324/c03f06f6-f41f-4b20-a97e-ae405d3a950c)

## Search for knowledge base

As of RAGFlow v0.20.4, the search feature is still in a rudimentary form, supporting only knowledge base search by name.

![search knowledge base](https://github.com/infiniflow/ragflow/assets/93570324/836ae94c-2438-42be-879e-c7ad2a59693e)

## Delete knowledge base

You are allowed to delete a knowledge base. Hover your mouse over the three dot of the intended knowledge base card and the **Delete** option appears. Once you delete a knowledge base, the associated folder under **root/.knowledge** directory is AUTOMATICALLY REMOVED. The consequence is:

- The files uploaded directly to the knowledge base are gone;  
- The file references, which you created from within **File Management**, are gone, but the associated files still exist in **File Management**. 

![delete knowledge base](https://github.com/infiniflow/ragflow/assets/93570324/fec7a508-6cfe-4bca-af90-81d3fdb94098)
