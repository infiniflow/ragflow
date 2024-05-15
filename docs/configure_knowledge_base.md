# Configure a knowledge base

Knowledge base, hallucination-free chat, and file management are three pillars of RAGFlow. RAGFlow's AI chats are based on knowledge bases. Each of RAGFlow's knowledge bases serves as a knowledge source, *parsing* uploaded files from your local machines and file references generated in file management into the real 'knowledge' for future AI chats. This guide demonstrates some basic usages of the knowledge base feature, covering the following topics:

- Create a knowledge base
- Configure a knowledge base
- Search for a knowledge base
- Delete a knowledge base

## Create a knowledge base

a **root/.knowledgebase** folder is created each time a knowledge base is created. 

## Configure a knowledge base

File parsing is an indispensable topic in knowledge base configuration. The meaning of file parsing in RAGFlow is twofold: file chunking based on file layout and converting chunks into vector embeddings. 

### Select an embedding model

once 

### Select a chunk method

### Upload file

- RAGFlow's file management allows you to link a file to multiple knowledge bases, in which case each target knowledge base holds a reference to the file
- In **Knowledge Base**, you are also given the option of uploading files from your local machines to a knowledge base, in which case the knowledge base holds file copies. 

does it support bulk load?

As of RAGFlow v0.5.0, 

## Search for a knowledge base

As of RAGFlow v0.5.0, the search feature is still in a rudimentary form, supporting only knowledge base search by name.

![search knowledge base](https://github.com/infiniflow/ragflow/assets/93570324/836ae94c-2438-42be-879e-c7ad2a59693e)

## Delete a knowledge base

You are allowed to delete a knowledge base. Once you delete a knowledge base, the associated folder under **root/.knowledge** directory is AUTOMATICALLY REMOVED. the files and file references inside it are AUTOMATICALLY REMOVED. 

