---
sidebar_position: -5
slug: /manage_metadata
sidebar_custom_props: {
  categoryIcon: LucideCode
}
---
# Manage metadata

Manage metadata for your dataset and for your individual documents.

---

RAGFlow introduces comprehensive metadata management capabilities, operating both at the overarching dataset level and for individual files within that dataset. This powerful feature provides administrators and users with unprecedented control over how information is categorised, organised, and ultimately retrieved, ensuring that knowledge bases remain both structured and highly searchable. By implementing a dual-level approach to metadata management, the system accommodates both broad, organisational-wide consistency and the nuanced, specific requirements of individual documents.

## Managing metadata

1.  Within your chosen dataset, locate and click on the **Metadata** option. This action will direct you to the **Manage Metadata** page, the central hub for all metadata-related configurations. This page serves as the command centre from which you can oversee the entire metadata landscape of your knowledge base, offering a comprehensive overview of all defined fields and their associated values. It is designed to give you a holistic view, enabling strategic decisions about your data's structure before diving into file-specific details.

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/click_metadata.png)

2.  Once on the **Manage Metadata** page, you are presented with several options to govern your metadata:
    - **Edit values**: You have the ability to modify existing metadata entries. A key feature here is that if you rename two distinct values to the exact same name, the system will automatically recognise this duplication and merge them into a single, consolidated value. This intelligent merging functionality helps maintain data integrity by preventing fragmentation and ensuring that synonymous tags do not proliferate, which could otherwise lead to inconsistent filtering results. For example, if "UK" and "United Kingdom" exist as separate values, renaming one to match the other will prompt the system to unify them.
    - **Delete**: You can remove specific metadata values or delete entire metadata fields. It is important to note that any deletions performed here are not isolated; they will apply globally to every file that is associated with that particular metadata field or value. This global application is a crucial consideration, as it allows for sweeping organisational changes but also requires careful forethought. Removing a field like "Department" will strip that categorisation from every file in the dataset, streamlining the metadata structure but potentially removing a key organisational layer.

    Following these actions, the system will present the configuration page where you can establish rules for the automatic generation of metadata. This automated rule-setting capability is a significant time-saver for large datasets, allowing you to define patterns or conditions that will systematically apply metadata to new and existing files without manual intervention. For instance, you could create a rule that automatically tags any document containing specific keywords with a particular project label.

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/manage_metadata.png)

3.  For more granular control over a single file, you can manage its metadata directly. Navigate to the specific file's details page, which provides a focused view of that document's properties and content. From there, click on its designated parsing method, such as **General**, which indicates how the file's content has been processed by the system. In the menu that appears, select **Set Metadata**. This action opens an interface where you can view and edit the metadata exclusively for this file. Here, you have the freedom to add new fields, delete existing ones, or modify current values, with the understanding that these changes are applied only to this specific file. This file-level precision is invaluable for handling exceptions or adding unique descriptors to a standout document without affecting the broader dataset's schema. All edits made at this individual file level will be aggregated and reflected in the global metadata statistics shown on the main knowledge base's metadata management page, providing a real-time, accurate count of how many files are associated with each metadata value.

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/set_metadata.png)
![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/edit_metadata.png)

4.  The metadata filtering functionality is a powerful tool that operates at two distinct stages: within the knowledge base management interface and during the document retrieval process. When you are inside your dataset, simply click the **Filter** button. This will display a clear breakdown, showing you the number of files that are linked to each specific value under your existing metadata fields. This instant visualisation aids in understanding the distribution of your content; for example, you can immediately see that you have 150 files tagged with "Client A" and only 25 with "Client B". By selecting one or more of these specific values, the view will dynamically update to display only the files that match your chosen criteria, enabling you to quickly isolate and work with a specific subset of your knowledge base for auditing, updating, or export.

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/filter_metadata.png)

5.  The utility of metadata filtering extends into the retrieval stage as well, significantly enhancing the precision of search and question-answering functionalities. For instance, within the Chat feature, after you have configured a knowledge base, you can establish specific metadata filtering rules to refine how documents are retrieved in response to queries. This ensures that the AI's responses are drawn from the most relevant subset of your data, improving accuracy and contextual relevance. The system offers three distinct modes of operation to cater to different levels of user control and automation requirements:

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/metadata_filtering_rules.png)

- **Automatic Mode**: In this mode, the system takes full control. It autonomously filters and selects relevant documents by intelligently interpreting the user's query and cross-referencing it with the existing metadata structure of the knowledge base. This is the most hands-off approach, ideal for general use cases where the user wants the broadest possible, yet still intelligently constrained, search. The system's algorithms work in the background to infer the most likely metadata filters based on the natural language of the question.
- **Semi-automatic Mode**: This mode offers a collaborative approach. Users first establish a high-level filtering scope by defining a field, such as limiting results by a specific **Author**, a particular **Department**, or a defined **Date Range**. Once this scope is set, the system then automatically filters the documents, operating intelligently but strictly within that predefined field-level boundary. This combines the user's domain knowledge (knowing, for example, that only documents from the R&D department are relevant) with the system's efficiency in handling the precise value matching within that field.
- **Manual Mode**: For maximum precision, the manual mode places control entirely in the user's hands. Here, users can construct detailed filter conditions by specifying exact field values. This mode is supported by a range of operators, including **Equals**, **Not equals**, **In**, **Not in**, **Greater than**, **Less than**, and others. This allows for highly targeted and specific document retrieval, such as finding all documents where the "Status" field **Equals** "Approved" and the "Version" field is **Greater than** "2.0". This mode is essential for analytical queries, compliance checks, and any scenario where pinpoint accuracy is paramount.