---
sidebar_position: 5
title: "Chunk: Parsing Results and Knowledge Fragment Management"
sidebar_label: "Chunk: Parsing Results and Knowledge Fragment Management"
slug: /chunk_parsing_results_and_knowledge_fragment_management
sidebar_custom_props:
  categoryIcon: LucideDatabaseZap
---

# Chunk: Parsing Results and Knowledge Fragment Management

Chunks are the retrievable knowledge fragments generated after document parsing. They are the basic units used by RAGFlow for retrieval and Q&A. Chunk management is used to view, search, filter, edit, add, enable, disable, and delete chunks.

## View Parsing Results

After document parsing is completed, click the document name on the **Files** page to enter the document parsing result page. The parsing result page displays chunks generated from the document. You can use this page to check whether document content has been correctly parsed, split, and stored.

## View Chunk List

The chunk list displays the chunks generated for the current document. Each chunk usually includes body text, keywords, questions, enabled status, and related operations. For documents that support source preview, the chunk list can be linked with source content. After clicking chunk text, the document preview area jumps to the corresponding source location, making it convenient to compare with the original document and check parsing and splitting results.

For document types that cannot locate source text, parsing results are mainly viewed through chunk body text.

## Search and Filter Chunk

When a document contains many chunks, use the search and filter functions in the toolbar to quickly locate target content. You can enter keywords to search chunks or filter chunks by enabled status. For chunks with long body text, you can switch between full display and collapsed display as needed to browse parsing results.

### View Chunk Details

After opening chunk details, you can further view the body text and related information for the knowledge fragment, such as:

- **Body text**: The main text actually stored in the chunk and used for retrieval. After clicking the chunk body, the document preview area jumps to the corresponding source location.
- **Keywords**: Keywords related to the current chunk, used to enhance content retrieval.
- **Questions**: Questions generated or configured based on chunk content, used to enhance recall for related questions.
- **Enabled status**: Determines whether the current chunk participates in dataset retrieval.

When viewing a chunk, you can check it together with its corresponding source document to confirm whether the chunk content is complete, semantically coherent, and suitable for retrieval.

### Edit Chunk

If parsing results are inaccurate or content needs to be supplemented, you can edit chunks manually. Double-click a chunk to open the edit parsing block window. You can view and modify:

- **Content**: The body text of the chunk.
- **Keywords**: Keywords related to chunk content. Keywords can be added or deleted to enhance retrieval for the chunk.
- **Questions**: Questions associated with the chunk.
- **Tags**: Tags added to the chunk, used to mark and manage knowledge fragments.

After saving the edit, the updated chunk content is used for later retrieval.

### Add Chunk

If you need to add extra knowledge fragments manually, you can add chunks on the chunk management page. Added chunks are stored under the current document and can be used as content sources for dataset retrieval and Q&A.

When adding a chunk, you usually need to enter the chunk content and configure keywords, questions, tags, or enabled status as needed.

### Enable and Disable Chunk

The enabled status of a chunk determines whether it participates in dataset retrieval. A disabled chunk remains in the parsing result but is not recalled during retrieval.

If a chunk contains outdated, incorrect, or temporarily unnecessary content, you can disable it. If the content needs to participate in retrieval again, enable it again.

### Delete Chunk

Deleting a chunk removes it from the current document's parsing results. It no longer participates in later retrieval or Q&A.

Before deleting, confirm that the chunk is no longer needed. If you only want to temporarily exclude it from retrieval, use disable instead.
