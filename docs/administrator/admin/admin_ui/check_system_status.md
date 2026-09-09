---
sidebar_position: 2
sidebar_label: Check System Status
title: Check System Status
---

# Check System Status

## Check Whether Services Are Running Normally

After entering the Admin UI, open the **Service status** page to view the runtime status of RAGFlow and its dependent services. The page displays each service's name, service type, host, port, and current status, so administrators can confirm whether all system components are running normally.

![Check Whether Services Are Normal](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/check_whether_services_are_normal.jpg)

When **Status** is `Alive`, the service is running normally. Any other status may affect the corresponding features.

![System Status](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/system_status.jpg)

| Service name | Main purpose | Possible issues when abnormal | Affected features |
| --- | --- | --- | --- |
| RAGFlow Server | Core system service responsible for the Web UI, APIs, knowledge bases, Q&A, Agent, and other business features. | The system cannot be accessed, login fails, pages report errors, or all features become unavailable. | Login, knowledge base management, knowledge applications, Agent, model management, system management, and all other features. |
| MySQL | Stores business data such as users, knowledge bases, model configurations, and system configurations. | Login fails, knowledge base data cannot be read, configurations cannot be saved, or business data becomes abnormal. | User management, knowledge base management, model configuration, system configuration, and business data reads and writes. |
| MinIO | Stores uploaded documents, images, and other object files. | File upload fails, documents cannot be opened, parsing fails, or images cannot be displayed. | File upload, document management, document parsing, file preview, and download. |
| Elasticsearch | Builds full-text and vector indexes and provides knowledge retrieval capabilities. | Documents cannot be retrieved, searches return no results, Q&A cannot cite knowledge, or knowledge recall is abnormal. | Full-text retrieval, vector retrieval, hybrid retrieval, knowledge Q&A, and knowledge citation. |
| Redis (Valkey) | Provides cache and task status management to improve system runtime efficiency. | Pages respond slowly, sessions become abnormal, or some features run abnormally. | System cache, session management, task status management, and some backend features. |
| RabbitMQ | Manages backend asynchronous tasks and sends tasks to executors. | Documents remain in `Waiting for processing`, or backend tasks cannot start. | Asynchronous task scheduling for document parsing, knowledge compilation, Embedding, and index building. |
| Task Executor | Executes backend tasks such as document parsing, OCR, Embedding, and index building. | Documents remain in `Parsing`, knowledge cannot be imported, or indexes cannot be built. | OCR, document parsing, Embedding, knowledge ingestion, index building, and other backend processing tasks. |

## View Service Details

On the **Service status** page, administrators can view the service `ID`, `Name`, `Service type`, `Host`, `Port`, and `Status`. When `Status` is `Alive`, the service is currently alive.

Administrators can open service details from **Actions**. Different services display different details. For example, the `mysql` service details show current database connection and process information, including `command`, `db`, `host`, `id`, `info`, `state`, `time`, and `user`. Administrators can use this information to determine whether there are long-running connections, waiting states, or abnormal queries.

![View Service Details](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_service_details_1.jpg)

![View Service Details](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_service_details_2.jpg)

Some services also provide an **Extra information** dialog that displays supplementary configuration information. For example, an object storage service may display information such as `store_type` and `user`. This information is mainly used to confirm service configuration.

![View Service Details](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_service_details_3.jpg)

![View Service Details](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_service_details_4.jpg)

If a service's `Status` is not `Alive`, first record its `ID`, `Name`, `Service type`, `Host`, `Port`, and any abnormal information visible in the details dialog or **Extra information**.

The Admin UI is mainly used to view service status. It does not provide direct troubleshooting entry points for containers, processes, networks, or logs. For further handling, check the corresponding deployment environment, including service runtime status, port connectivity, service logs, and related configurations. After identifying the cause, restart services, adjust configurations, or perform other recovery operations according to your operations process.
