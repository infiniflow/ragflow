---
sidebar_position: 9
title: "Logs: Logs"
sidebar_label: "Logs: Logs"
slug: /dataset_logs
sidebar_custom_props:
  categoryIcon: LucideDatabaseZap
---

# Logs: Logs

## Log Overview

**Logs** is used to view execution records of tasks related to the current dataset. The top of the page displays statistics such as **Total files**, **Processing**, and **Downloading**. The lower area is divided into document logs and dataset-level logs by log type. Document logs focus on the processing process of a single document. Dataset-level logs focus on tasks within the entire dataset scope. When troubleshooting, first determine whether the problem occurs in a single document or the entire dataset, then go to the corresponding logs to view details.

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

## Log Troubleshooting Suggestions

When task execution is abnormal or does not complete for a long time, first select the corresponding logs based on task type, then troubleshoot based on task status and log details.

- **Document processing tasks**: View document logs. Document logs record processing tasks for specific documents. Use information such as **Filename**, **Source**, **Ingestion pipeline**, **Task**, and **Status** to confirm which document and processing flow has the exception. For documents processed with **Data Pipeline**, you can also use the entry in **Operations** to view the corresponding pipeline execution result.
- **Dataset-level processing tasks**: View dataset-level logs. Dataset-level tasks such as **Knowledge Compilation** are recorded here. Use **Processing type** and **Status** to find the corresponding task, and view log details through **Operations**.
- **Task execution failed**: When **Status** is **Failed**, open the corresponding task log details and view the specific error information.
- **Task does not complete for a long time**: When a task stays in **Pending**, **Running**, or **Schedule** for a long time, first confirm the current task status and start time, then view log details to determine whether the task is still running normally.

> Note: Document logs and dataset-level logs record different types of processing tasks. They are not parent-child logs or summary logs. When troubleshooting, select the corresponding log based on the actual task.
