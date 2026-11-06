---
sidebar_position: 9
title: "Logs"
sidebar_label: "Logs"
slug: /dataset_logs
sidebar_custom_props:
  categoryIcon: LucideDatabaseZap
---

# Logs

## Log Overview

**Logs** shows task execution records for the current dataset. The top of the page displays file, processing, and downloading statistics. Use the **File**, **Dataset**, and **Data source** tabs to inspect document processing, dataset-level tasks, and external data source synchronization respectively.

## Document Logs

Document logs are used to view and trace task execution related to a single document, such as document parsing, task cancellation, and task success or failure. When a document has not completed parsing for a long time, parsing fails, or the number of generated chunks is abnormal, use document logs to view task status and execution details.

Document logs mainly include:

- **ID**: The unique identifier of the log record or task.
- **Filename**: The name of the document executing the current task.
- **Source**: The document source.
- **Ingestion pipeline**: The parsing method or pipeline used when processing the document.
- **Start date**: The task start time.
- **Task**: The task type, such as **Parse**.
- **Status**: The current task execution status.
- **Operations**: Operation entry. You can view log details for the current task, including execution process and error information.

If parsing fails for only one document, a document stays processing for a long time, or the number of chunks is abnormal, it is recommended to check that document's logs first.

## Dataset-Level Logs

Dataset-level logs are used to view task execution records whose processing object is the entire dataset. Unlike document logs for single-document parsing tasks, dataset-level logs mainly record dataset-level processing tasks, such as **Knowledge Compilation**.

Dataset-level logs mainly include:

- **ID**: The unique identifier of the task record.
- **Start date**: The task start time.
- **Processing type**: The processing type, used to indicate the current dataset-level task, such as **Wiki**.
- **Status**: The current task execution status.
- **Operations**: Operation entry. You can view log details and execution information for the current task.

When a dataset-level processing task fails, does not complete for a long time, or needs execution confirmation, view the corresponding task record and log details here.

> Tip: For tasks executed on a single document, such as document parsing, view document logs. For tasks executed on the entire dataset, such as **Knowledge Compilation**, view dataset-level logs.

## Data Source Logs

Open **Logs** and select **Data source** to check synchronization tasks for the current dataset. The table shows:

- **Time started**: The time shown for the task record.
- **Status**: The task's execution status.
- **Task Type**: `sync` synchronizes content; `prune` removes content deleted from the source.
- **Summary**: Synchronization progress, results, or error information.

Use this tab when source files are missing or synchronization fails. For parsing failures after synchronization, check **File** logs.

## Log Troubleshooting Suggestions

When task execution is abnormal or does not complete for a long time, first select the corresponding logs based on task type, then troubleshoot based on task status and log details.

- **Document processing tasks**: View document logs. Document logs record processing tasks for specific documents. Use information such as **Filename**, **Source**, **Ingestion pipeline**, **Task**, and **Status** to confirm which document and processing flow has the exception. For documents processed with **Data Pipeline**, you can also use the entry in **Operations** to view the corresponding pipeline execution result.
- **Dataset-level processing tasks**: View dataset-level logs. Dataset-level tasks such as **Knowledge Compilation** are recorded here. Use **Processing type** and **Status** to find the corresponding task, and view log details through **Operations**.
- **Data source synchronization**: Select **Data source** and check **Status**, **Task Type**, and **Summary**.
- **Task execution failed**: When **Status** is **Failed**, open the corresponding task log details and view the specific error information.
- **Task does not complete for a long time**: When a task stays in **Pending**, **Running**, or **Schedule** for a long time, first confirm the current task status and start time, then view log details to determine whether the task is still running normally.

> Note: File, dataset, and data source logs cover different task types. Select the tab that matches the task you are troubleshooting.
