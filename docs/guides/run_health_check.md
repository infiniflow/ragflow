---
sidebar_position: 7
slug: /run_health_check
---

# Run health check on RAGFlow's dependencies

Double-check the health status of RAGFlow's dependencies.

---

The operation of RAGFlow depends on four services:

- **Elasticsearch** (default) or [Infinity](https://github.com/infiniflow/infinity) as the document engine
- **MySQL**
- **Redis**
- **MinIO** for object storage

If an exception or error occurs related to any of the above services, such as `Exception: Can't connect to ES cluster`, refer to this document to check their health status.

You can also click you avatar in the top right corner of the page **>** System to view the visualized health status of RAGFlow's core services. The following screenshot shows that all services are 'green' (running healthily). The task executor displays the *cumulative* number of completed and failed document parsing tasks from the past 30 minutes:

![system_status_page](https://github.com/user-attachments/assets/b0c1a11e-93e3-4947-b17a-1bfb4cdab6e4)

Services with a yellow or red light are not running properly. The following is a screenshot of the system page after running `docker stop ragflow-es-10`:

![es_failed](https://github.com/user-attachments/assets/06056540-49f5-48bf-9cc9-a7086bc75790)

You can click on a specific 30-second time interval to view the details of completed and failed tasks:

![done_tasks](https://github.com/user-attachments/assets/49b25ec4-03af-48cf-b2e5-c892f6eaa261)

![done_vs_failed](https://github.com/user-attachments/assets/eaa928d0-a31c-4072-adea-046091e04599)
