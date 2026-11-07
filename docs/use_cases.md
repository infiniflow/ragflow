---
title: Use Cases
sidebar_label: Use Cases
sidebar_position: 3
slug: /use_cases
sidebar_custom_props: {
  sidebarIcon: LucideCompass
}
---

# Use Cases

The following cases cover document-based question answering, Agentic RAG, knowledge compilation, data synchronization, and team collaboration. Each case includes preparation, configuration, usage, validation, and maintenance. Replace the illustrative facts with information appropriate to your environment. The checks describe what to verify with your documents; generated wording and page organization may vary.

If you are new to RAGFlow, complete the [10-Minute Tutorial](./quickstart.mdx#10-minute-tutorial) first.

| Case | Task | Features |
| --- | --- | --- |
| [Internal policy assistant](#answer-questions-about-internal-policies) | Answer policy questions and check exceptions against cited documents. | Dataset, Naive Chat |
| [Research comparison](#investigate-research-questions-across-documents) | Compare methods, evaluation conditions, and limitations across papers. | Dataset, Agentic RAG |
| [Technical Wiki](#organize-technical-documents-into-a-wiki) | Compile operations documents into linked topic pages. | Knowledge Compilation, Artifacts |
| [Synchronized product documentation](#answer-questions-from-continuously-updated-content) | Verify that source revisions become available in Chat. | Data Sources, Dataset, Chat |
| [Shared product Dataset](#maintain-and-use-a-dataset-as-a-team) | Maintain common documents and use them in member applications. | Team, Dataset, Chat or Search |

## 1. Answer Internal Policy Questions {#answer-questions-about-internal-policies}

### Scenario

A company wants employees to check travel approval and reimbursement rules without searching several policy files. The assistant must answer from the current policies and provide references for verification.

Documents → Dataset → Naive Chat → Cited answer

This case uses **Naive** mode because the questions concern specific rules stated in the policy documents.

### Prepare the Documents and Models

Prepare the current travel policy, reimbursement rules, and any departmental exceptions. Remove superseded versions or clearly identify which policy applies to each group. Configure an Embedding model for document retrieval and a Chat model for answers.

For a controlled test, use documents containing the following example rules, or replace them with equivalent facts from your own policies. These rules are illustrative, not company policy:

| Document | Example content |
| --- | --- |
| Travel policy | International travel requires manager approval before booking. |
| Reimbursement rules | Hotel expenses require an itemized invoice. |
| Department exceptions | The sales team requires director approval for international travel. |

### Configure the Assistant

1. [Create a Dataset](./guides/dataset/dataset_list_and_creation.md) named `Travel policies`. Use **General** parsing for the initial test.
2. [Upload and parse the policies](./guides/dataset/files_dataset_document_management.md#upload-local-documents). Check that the approval rules and departmental exceptions appear in the chunks.
3. [Create a Chat](./guides/chat/feature_overview_and_creation.md#create-a-chat) named `Travel policy assistant`, using the prepared Dataset and a Chat model.
4. Configure the answer requirements: use **Naive** mode, show citations, identify applicable exceptions, and state when the documents do not establish an answer. Retain the existing prompt variables when editing the instructions.
5. If a fallback is required, use `No relevant policy was found. Please contact the policy owner.` for questions where retrieval finds no usable content.

For citation display, prompt instructions, and fallback fields, see [Chat Configuration](./guides/chat/chat_configuration.md).

### Ask Questions and Check the Answers

Use the following checks with the example policies:

| Question | Expected result |
| --- | --- |
| Do I need approval before booking international travel? | Identifies the manager-approval requirement and cites the travel policy. |
| I work in sales. Whose approval do I need before booking international travel? | Applies the sales exception and cites the departmental document. |
| What document do I need for hotel reimbursement? | Identifies the itemized invoice requirement and cites its source. |
| Are personal holiday costs reimbursable? | Does not invent a policy when the documents do not establish one. |

Open the citations and verify the actual passages. A plausible answer is insufficient if the cited text does not support it. If an exception is missed, use Dataset retrieval testing to check whether both the general rule and the exception are retrieved.

**Completion check:** The assistant answers the documented policy questions, applies the sales exception, provides supporting citations, and does not invent an answer to the unsupported question.

### Maintain the Policies

After a policy revision, ensure the applicable version replaces obsolete content in retrieval. Repeat the affected questions, especially departmental exceptions. Use the same Chat with the maintained Dataset.

For file updates and parsing controls, see [Dataset Document Management](./guides/dataset/files_dataset_document_management.md).

## 2. Compare Research Papers with Agentic RAG {#investigate-research-questions-across-documents}

### Scenario

A researcher is reviewing three papers on the same technical problem. They need a comparison of the methods, evaluation conditions, results, and stated limitations, with evidence from each paper.

Papers → Dataset → Agentic RAG → Evidence-based comparison

This case uses **Agentic** mode because the comparison requires evidence from several papers and multiple retrieval steps.

### Prepare the Research Collection

Select three papers with readable text and identifiable titles. Include their experimental results and limitations sections. Prepare an Embedding model, a Chat model, and an available Decision model for Agentic processing.

Before testing, record one method description and one reported limitation from each paper. These notes provide a reference for checking the generated comparison; they do not need to be uploaded as additional evidence.

Files can be uploaded directly. If the collection is maintained in Zotero, configure the [Zotero connector](./guides/data_source/data_source_configuration.md#zotero) to synchronize its PDF attachments.

### Configure Agentic Retrieval

1. [Create a Dataset](./guides/dataset/dataset_list_and_creation.md) named `Research comparison`, then [import and parse the papers](./guides/dataset/files_dataset_document_management.md).
2. Inspect representative chunks from each paper. Equations, tables, and experimental conditions needed for the comparison must be readable.
3. [Create a Chat](./guides/chat/feature_overview_and_creation.md#create-a-chat) using that Dataset and a Chat model, with citations enabled.
4. [Configure Agentic retrieval](./guides/chat/chat_configuration.md#agentic-settings) with an available Decision model and the supplied Agentic template.
5. Select **Agentic** mode for the comparison question. Configuring Agentic settings alone does not select the conversation mode.

### Request a Comparison

Replace the titles below with those of the imported papers:

```text
Compare [Paper A], [Paper B], and [Paper C].
For each paper, summarize its method, evaluation data, reported results,
and limitations. Present the comparison in a table and cite the sources.
Explain which findings can be compared directly and which use different
experimental conditions. Mark information that the papers do not report.
```

Agentic mode can perform several retrieval steps to assemble the answer. Allow more time than for a direct factual lookup in Naive mode.

### Verify the Comparison

A table with columns for **Paper**, **Method**, **Evaluation conditions**, **Reported results**, **Limitations**, and **Sources** is a useful format for this comparison. Other formats are suitable if they cover the same information and identify details that the papers do not report.

Check that all three papers are represented and that the methods and limitations agree with your reference notes. Open each citation to locate the supporting passage in the corresponding paper. A reference number alone is insufficient to verify a claim. Compare reported results only where the evaluation data and metrics allow a meaningful comparison.

Follow up with a narrower question, such as “Which paper discusses computational cost, and what evidence does it provide?” Confirm that missing information is identified rather than supplied without support.

If an important section is absent from the answer, check its parsing and retrieval results before changing the Agentic template.

**Completion check:** All three papers are covered, comparison criteria are consistent, and the reported findings can be checked against citations. Missing information and differences in evaluation conditions are identified.

### Extend the Collection

When adding papers, repeat a comparison question and confirm that the new evidence is represented. Keep the comparison criteria consistent so changes in the answer can be checked against the expanded collection.

For adding and parsing papers, see [Dataset Document Management](./guides/dataset/files_dataset_document_management.md).

## 3. Compile Technical Documents into a Wiki {#organize-technical-documents-into-a-wiki}

### Scenario

An engineering team has separate deployment instructions, configuration references, and troubleshooting records. The team needs a Wiki that groups related information into topics while retaining the original documents for verification.

Technical documents → Compiler pipeline → Dataset Wiki → Content review

This case uses **Wiki** compilation in **Topic mode** to organize information from separate documents into pages grouped by operational topic.

### Prepare the Source Material

Begin with a small set of current documents covering installation, configuration, and common failures for one product. Include related material across more than one file so the test can assess topic organization rather than simply reproduce one document.

Configure an Embedding model for the Dataset and a model for compilation. Identify the topics the Wiki should cover and any version boundaries that must be preserved.

### Define and Apply the Wiki Template

1. [Create a Wiki compilation template](./guides/knowledge_compilation/basic_information_configuration.md) named `Product operations Wiki`. Set **Wiki mode** to **Topic mode** to organize the material by deployment, configuration, and troubleshooting topics.
2. Define the organization and source requirements. The following instructions can be used for this case:

   ```text
   Organize the source material into deployment, configuration, and
   troubleshooting topics. Preserve product versions and prerequisites.
   Keep commands and configuration values faithful to the source.
   Do not add procedures that are not supported by the documents.
   ```

3. [Configure an ingestion pipeline with Compiler](./guides/knowledge_compilation/apply_knowledge_compilation_template.md#configure-the-ingestion-pipeline). Reference the Wiki template group and select a compilation model.
4. [Create the target Dataset](./guides/dataset/dataset_list_and_creation.md) with the prepared Embedding model.
5. Publish the pipeline and [apply its published version to the Dataset](./guides/knowledge_compilation/apply_knowledge_compilation_template.md#apply-the-pipeline-in-dataset).
6. Run the prepared documents through that pipeline. Saving a template without processing documents does not generate a Wiki.

For Wiki-specific fields, see [Template Configuration](./guides/knowledge_compilation/built_in_templates_and_dedicated_configuration.md).

### Browse and Validate the Wiki

Wait for document parsing and background Wiki compilation to finish, then open **Artifacts > Wiki**. Check compilation status and Dataset logs if pages have not appeared yet.

Use an operational task to assess the result: find the deployment prerequisites, locate a configuration parameter, and follow the documented response to a common failure. Confirm that these topics are covered, page links are usable, and commands and values match the original files. The exact page titles and structure may vary.

Correct source errors or adjust the template if the organization is unsuitable. Reprocess the sample documents and review the result before compiling the full collection.

**Completion check:** The Wiki covers deployment, configuration, and troubleshooting topics; its links are usable, and the reviewed commands and values agree with the source documents.

### Update the Wiki

After adding material or revising the template, recheck the affected topics, commands, and page links. A template change alone does not recompile completed documents.

For processing revised documents and background artifact updates, see [Update Knowledge Artifacts](./guides/knowledge_compilation/apply_knowledge_compilation_template.md#update-knowledge-artifacts). Other formats, including Graph, Tree/PageIndex, and Mindmap, are described in [Knowledge Compilation Overview](./guides/knowledge_compilation/overview.md).

## 4. Keep Product Answers in Sync with Confluence {#answer-questions-from-continuously-updated-content}

### Scenario

A product team maintains installation instructions in Confluence. Support staff use Chat to answer customer questions and need revisions to become available without manually uploading each changed page.

Confluence → Synchronization → Dataset parsing → Chat

This case uses a **Confluence data source** because the documents are maintained in Confluence and revisions need to be synchronized into the Dataset.

### Prepare a Test Scope

Choose a small Confluence space or page collection and credentials with read access to it. Include an installation page with a fact you can safely change for testing, such as a stated disk-space requirement. Use test content rather than changing an operational requirement solely for this exercise.

Configure Chat and Embedding models. Decide which pages belong in the assistant's scope and whether source deletions should also remove synchronized content.

### Connect the Source and Prepare Chat

1. [Create and test a Confluence connection](./guides/data_source/data_source_configuration.md#confluence) named `Product documentation`. Limit its scope to the test collection and decide whether source deletions should be synchronized.
2. [Create a Dataset](./guides/dataset/dataset_list_and_creation.md) for the product documentation.
3. [Link the connection to the Dataset](./guides/data_source/add_to_knowledge_base_and_sync.md). Enable automatic parsing so synchronized revisions can become available for retrieval without a separate manual parsing step.
4. Confirm that synchronization and parsing have completed. Note the connection's refresh interval for the update test.
5. [Create a Chat](./guides/chat/feature_overview_and_creation.md#create-a-chat) using the Dataset, with citations enabled. Use Naive mode for the initial factual checks.

### Verify an Update

For this test, let the Confluence page state that installation requires **20 GB** of disk space. This is an illustrative value, not a product requirement. Ask “How much disk space does installation require?” and confirm that the answer reports 20 GB and cites the test page.

Change the requirement on that test page from **20 GB** to **30 GB**.

Wait for the next synchronization and parsing to finish. Confirm the revised content is present in the Dataset, start a new Chat conversation, and ask the same question. The answer should report **30 GB**, and the cited passage should contain the revised requirement. If the old value persists, check synchronization records, parsing status, and duplicate or outdated documents in the Dataset.

Creating or testing a connection alone does not import searchable content. Synchronization brings in the files; parsing makes their content available for retrieval.

**Completion check:** After the source update has synchronized and finished parsing, a new conversation reports the example requirement of 30 GB and cites the revised passage.

### Maintain the Connection

Recheck the update test when the source structure, access permissions, or synchronization scope changes. For credentials, connection checks, and synchronization records, see [Data Source Management](./guides/data_source/overview_and_page_management.md).

After validating the answers, the Chat can be [embedded in a website](./guides/chat/using_chat_sessions.md#embed-into-a-website) or used through a supported [Chat Channel](./guides/chatchannel/chat_channels_overview.md).

## 5. Maintain a Product Dataset with Your Team {#maintain-and-use-a-dataset-as-a-team}

### Scenario

A support lead maintains product manuals and service notes. Team members need to update the common material and use it in their own Chat or Search applications without keeping separate document copies.

Team invitation → Shared Dataset → Member maintenance → Chat or Search

This case shares the **Dataset** so members maintain one collection of documents while using their own Chat or Search applications.

### Prepare the Owner and Member Accounts

Use a workspace owner account and a separate member account. The owner prepares a Dataset with parsed product documents and working parsing models. A member who creates a Chat also needs an available Chat model in their own account.

Review [member permissions](./guides/team/resource_sharing/resource_sharing_and_permissions.md#member-permissions) before sharing. Members will maintain common content, so their changes affect other users of the Dataset.

### Share the Dataset and Use Its Content

1. The owner [invites the member](./guides/team/team_management/manage_team_members.md#invite-members).
2. The member [accepts the invitation](./guides/team/team_management/join_and_leave_teams.md#accept-or-decline-an-invitation).
3. The owner [sets the Dataset sharing scope to Team](./guides/dataset/configuration.md#sharing-scope).
4. The member confirms access, then [creates a Chat using the shared Dataset](./guides/team/resource_sharing/use_datasets_with_your_team.md#use-a-shared-dataset-in-chat). Enable citations for verification. For passage retrieval, the member can instead [create Search](./guides/search/create_search.md).

Sharing a Dataset does not share the owner's Chat or conversations. Each member configures their own application using the common material.

### Test Collaborative Maintenance

1. As the member, upload a short service note containing a recognizable test fact, such as `Model A requires a five-second reset-button press`.
2. Parse the note and wait for completion. Parsing uses the models configured for the Dataset or its pipeline in the owner's workspace.
3. Ask a question about the new fact in the member's Chat and check the citation.
4. As the owner, confirm that the same note and its parsed content are available in the shared Dataset. Use **Retrieval testing** to ask “How long should I press the reset button on Model A?” and check that the results contain the five-second instruction.

If the member cannot see the Dataset, confirm invitation acceptance and the saved sharing scope. If parsing fails because a model is unavailable, the owner should check the Dataset's model configuration.

**Completion check:** The member's Chat cites the new service note, and the owner retrieves the same five-second instruction from the shared Dataset without uploading another copy.

### Maintain Shared Materials

Agree on which documents are authoritative and who maintains them. Coordinate deletions and parsing-configuration changes, then repeat the affected product questions from a member account.

For document operations and model use, see [Use Datasets with Your Team](./guides/team/resource_sharing/use_datasets_with_your_team.md).

For the complete feature documentation, see the [User Guides](./category/user-guides).
