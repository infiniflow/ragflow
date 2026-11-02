---
sidebar_position: 10
title: Notes and FAQs
sidebar_label: Notes and FAQs
slug: /dataset_notes_and_faqs
sidebar_custom_props: {
  categoryIcon: LucideDatabaseZap
}
---

# Notes and FAQs

## Do I Need to Parse Again After Modifying Parsing Configuration?

Usually yes. Parsing configuration affects how documents are converted into chunks. For parsed documents, modifying configuration does not necessarily rewrite old chunks automatically. If you want the new configuration to apply to existing documents, parse the relevant documents again.

## When Do Metadata Changes Take Effect?

After metadata is manually modified, interface filters can usually use the new values directly. If metadata participates in retrieval filtering or affects indexing results, you may need to combine it with re-parsing, index refresh, or re-testing to confirm the final effect.

## What Should I Do If Document Parsing Fails?

It is recommended to troubleshoot in the order of **document status -> configuration -> Logs**. First confirm whether the document is still parsing or has failed, then check the parsing method, model, and data source configuration, and finally open document log details to view the error cause.

## Why Does the Log Show "File not found in object storage."?

This message means the source file could not be found in object storage when RAGFlow tried to read it for parsing. The original storage error remains available in the document log.

Possible causes include:

- The disk holding MinIO's data directory ran low on free space, preventing an earlier upload from being written successfully.
- The file was deleted from object storage after it was uploaded.

The message alone does not establish that disk space is the cause. Check the free space on the machine or volume hosting MinIO's data directory and review MinIO's logs from the time of the upload for write failures. In the default Docker Compose deployment, MinIO stores its data in the `minio_data` volume mounted at `/data`.

After resolving any storage issues, upload the missing file again and retry parsing. Retrying parsing alone does not restore a missing file.

## What Should I Do If Retrieval Testing Cannot Retrieve Content?

It is recommended to troubleshoot in the order of **document status -> chunk -> metadata -> retrieval parameters**. First confirm that the document has been parsed successfully and generated usable chunks. Then check whether metadata has filter conditions that exclude related chunks. Finally, check retrieval parameters such as **Similarity threshold**, **Vector similarity weight**, and **Top K**.

Common causes include documents not yet parsed, no usable chunks generated, metadata filters being too strict, **Similarity threshold** being too high, or **Top K** being too small.

## Why Can't I See Artifacts?

Artifacts are related to knowledge compilation results. If the dataset has no usable chunks or knowledge compilation has not been completed, expected artifacts may not appear. First confirm that documents have been parsed successfully, then refer to **Knowledge Compilation** to generate or update artifacts.

## Run Retrieval Testing

Run retrieval testing on your dataset to check whether the expected chunks can be retrieved.

After files are uploaded and parsed, it is recommended to run retrieval testing before configuring a chat assistant. Running retrieval testing is never unnecessary or redundant. Like fine-tuning a precision instrument, RAGFlow requires careful adjustment to deliver the best Q&A performance. Your dataset settings, chat assistant configuration, and specified small and large models all significantly affect the final result. Running retrieval testing can verify whether expected chunks can be retrieved, allowing you to quickly identify areas that need improvement or locate issues that need to be resolved. For example, when debugging a Q&A system, if you know that the correct chunks can be retrieved, you can focus elsewhere. In issue #5627, the issue was found to be caused by LLM limitations.

During retrieval testing, hybrid search is used to retrieve chunks created by the chunking method you specified. This search combines weighted keyword similarity with weighted vector cosine similarity or weighted rerank score, depending on your settings:

- If no rerank model is selected, weighted keyword similarity is combined with weighted vector cosine similarity.
- If a rerank model is selected, weighted keyword similarity is combined with weighted vector rerank score.

### Prerequisites

- Your files have been uploaded and successfully parsed before running retrieval testing.

### Configuration

#### Similarity Threshold

This setting is the threshold for retrieving chunks. Chunks with similarity below the threshold are filtered out. By default, the threshold is set to `0.2`. This means only chunks with a hybrid similarity score of `0.2` or higher are retrieved.

#### Vector Similarity Weight

This setting controls the weight of vector similarity in the comprehensive similarity score, whether it is combined with vector cosine similarity or rerank score. By default, it is set to `0.3`, so the other component's weight is `0.7` (`1 - 0.3`).

#### Rerank Model

- If left empty, RAGFlow uses a combination of weighted keyword similarity and weighted vector cosine similarity.
- If a rerank model is selected, weighted keyword similarity is combined with weighted vector rerank score.

> Important: Using a rerank model significantly increases the time required to receive a response.

#### Use Knowledge Graph

In dataset retrieval testing, **Use Knowledge Graph** (`use_kg`) does not currently enable graph-based retrieval. Setting `use_kg` to `true` does not add graph traversal, entity or relationship retrieval, or community-report retrieval. The request continues through the ordinary hybrid retrieval path.

Knowledge graph generation and viewing are separate capabilities. You can still generate and view graph artifacts through Knowledge Compilation. Searchable compilation chunks may participate in ordinary retrieval according to the knowledge-compilation inclusion and scope settings; `use_kg` does not control their inclusion. See the [retrieval API reference](../../references/http_api_reference.md#retrieve-chunks) for these settings.

#### Cross-Language Search

To perform cross-language search, select one or more target languages from the drop-down menu. Then the system's default chat model translates the query you entered in the test text field into the selected target languages. This translation ensures accurate cross-language semantic matching, allowing you to retrieve relevant results regardless of language differences.

> Tip: When selecting target languages, make sure these languages exist in the dataset to ensure effective search. If no target language is selected, the system searches only in the language of your query, which may cause relevant information in other languages to be missed.

#### Test Text

This field is used to enter your test query.

### Operation Steps

1. Navigate to the dataset's **Retrieval Testing** page, enter your query in **Test text**, and click **Test** to run the test.
2. If the results are unsatisfactory, adjust the options listed in the configuration section and run the test again.

For example, hybrid retrieval combines weighted keyword similarity and weighted vector cosine similarity. A term similarity score of `25.17` multiplied by `0.7` plus a vector similarity score of `36.49` multiplied by `0.3` gives an overall score of approximately `28.57`.

> Warning: If you adjusted default settings, such as keyword similarity weight or similarity threshold, to obtain the best results, note that these changes are not saved automatically. You must apply them to your chat assistant settings or the **Retrieval Agent** component settings.

### FAQ

#### Is an LLM Used When the Use Knowledge Graph Switch Is Enabled?

In dataset retrieval testing, `use_kg` does not trigger graph retrieval or additional LLM calls for that step, even when set to `true`. Other enabled features, such as cross-language search or keyword extraction, may still call an LLM independently.

#### Why does it take longer for RAGFlow to parse a document than LangChain?

We put painstaking effort into document pre-processing tasks like layout analysis, table structure recognition, and OCR (Optical Character Recognition) using our vision models. This contributes to the additional time required.


## Best Practices: Index Acceleration

A checklist for accelerating document parsing and indexing.

Please note that some of your settings may consume a large amount of time. If you often find document parsing time-consuming, use the following checklist:

- On the dataset configuration page, turn off **Use RAPTOR to enhance retrieval**.
- Extracting the knowledge graph (GraphRAG) is time-consuming.
- On the dataset configuration page, disable **Auto keyword** and **Auto question**, because both depend on the LLM.
- v0.17.0+: If all PDFs in your dataset are pure text and do not require compute-intensive processing such as OCR (optical character recognition), TSR (table structure recognition), or DLA (document layout analysis), select **Naive** instead of **DeepDoc** or other time-consuming large model options in the **Document parser** drop-down menu. This significantly reduces document parsing time. In the RAGFlow open-source 1.0 release, DeepDoc performs these operations with CPU inference.
