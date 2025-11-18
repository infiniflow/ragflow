---
sidebar_position: 6
slug: /use_tag_sets
---

# Use tag set

Use a tag set to auto-tag chunks in your datasets.

---

Retrieval accuracy is the touchstone for a production-ready RAG framework. In addition to retrieval-enhancing approaches like auto-keyword, auto-question, and knowledge graph, RAGFlow introduces an auto-tagging feature to address semantic gaps. The auto-tagging feature automatically maps tags in the user-defined tag sets to relevant chunks within your dataset based on similarity with each chunk. This automation mechanism allows you to apply an additional "layer" of domain-specific knowledge to existing datasets, which is particularly useful when dealing with a large number of chunks.

To use this feature, ensure you have at least one properly configured tag set, specify the tag set(s) on the **Configuration** page of your dataset, and then re-parse your documents to initiate the auto-tagging process. During this process, each chunk in your dataset is compared with every entry in the specified tag set(s), and tags are automatically applied based on similarity.

## Scenarios

Auto-tagging applies in situations where chunks are so similar to each other that the intended chunks cannot be distinguished from the rest. For example, when you have a few chunks about iPhone and a majority about iPhone case or iPhone accessaries, it becomes difficult to retrieve those chunks about iPhone without additional information.

## 1. Create tag set

You can consider a tag set as a closed set, and the tags to attach to the chunks in your dataset are *exclusively* from the specified tag set. You use a tag set to "inform" RAGFlow which chunks to tag and which tags to apply.

### Prepare a tag table file

A tag set can comprise one or multiple table files in XLSX, CSV, or TXT formats. Each table file in the tag set contains two columns, **Description** and **Tag**:

- The first column provides descriptions of the tags listed in the second column. These descriptions can be example chunks or example queries. Similarity will be calculated between each entry in this column and every chunk in your dataset.
- The **Tag** column includes tags to pair with the description entries. Multiple tags should be separated by a comma (,).

:::tip NOTE
As a rule of thumb, consider including the following entries in your tag table:

- Descriptions of intended chunks, along with their corresponding tags.
- User queries that fail to retrieve the correct responses using other methods, ensuring their tags match the intended chunks in your dataset.
:::

### Create a tag set

:::danger IMPORTANT
A tag set is *not* involved in document indexing or retrieval. Do not specify a tag set when configuring your chat assistant or agent.
:::

1. Click **+ Create dataset** to create a dataset.
2. Navigate to the **Configuration** page of the created dataset, select **Built-in** in **Ingestion pipeline**, then choose **Tag** as the default chunking method from the **Built-in** drop-down menu.
3. Go back to the **Files** page and upload and parse your table file in XLSX, CSV, or TXT formats.  
   _A tag cloud appears under the **Tag view** section, indicating the tag set is created:_  
   ![Image](https://github.com/user-attachments/assets/abefbcbf-c130-4abe-95e1-267b0d2a0505)
4. Click the **Table** tab to view the tag frequency table:  
   ![Image](https://github.com/user-attachments/assets/af91d10c-5ea5-491f-ab21-3803d5ebf59f)

## 2. Tag chunks

Once a tag set is created, you can apply it to your dataset:

1. Navigate to the **Configuration** page of your dataset.
2. Select the tag set from the **Tag sets** dropdown and click **Save** to confirm.

:::tip NOTE
If the tag set is missing from the dropdown, check that it has been created or configured correctly.
:::

3. Re-parse your documents to start the auto-tagging process.  
   _In an AI chat scenario using auto-tagged datasets, each query will be tagged using the corresponding tag set(s) and chunks with these tags will have a higher chance to be retrieved._

## 3. Update tag set

Creating a tag set is *not* for once and for all. Oftentimes, you may find it necessary to update or delete existing tags or add new entries. 

- You can update the existing tag set in the tag frequency table.
- To add new entries, you can add and parse new table files in XLSX, CSV, or TXT formats.

### Update tag set in tag frequency table

1. Navigate to the **Configuration** page in your tag set.
2. Click the **Table** tab under **Tag view** to view the tag frequency table, where you can update tag names or delete tags.

:::danger IMPORTANT
When a tag set is updated, you must re-parse the documents in your dataset so that their tags can be updated accordingly.
:::

### Add new table files

1. Navigate to the **Configuration** page in your tag set.
2. Navigate to the **Dataset** page and upload and parse your table file in XLSX, CSV, or TXT formats.

:::danger IMPORTANT
If you add new table files to your tag set, it is at your own discretion whether to re-parse your documents in your datasets.
:::

## Frequently asked questions

### Can I reference more than one tag set?

Yes, you can. Usually one tag set suffices. When using multiple tag sets, ensure they are independent of each other; otherwise, consider merging your tag sets.

### Difference between a tag set and a standard dataset?

A standard dataset is a dataset. It will be searched by RAGFlow's document engine and the retrieved chunks will be fed to the LLM. In contrast, a tag set is used solely to attach tags to chunks within your dataset. It does not directly participate in the retrieval process, and you should not choose a tag set when selecting datasets for your chat assistant or agent.

### Difference between auto-tag and auto-keyword?

Both features enhance retrieval in RAGFlow. The auto-keyword feature relies on the LLM and consumes a significant number of tokens, whereas the auto-tag feature is based on vector similarity and predefined tag set(s). You can view the keywords applied in the auto-keyword feature as an open set, as they are generated by the LLM. In contrast, a tag set can be considered a user-defined close set, requiring upload tag set(s) in specified formats before use.
