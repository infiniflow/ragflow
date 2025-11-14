---
sidebar_position: 8
slug: /run_health_check
---

# Monitoring

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

## API Health Check

In addition to checking the system dependencies from the **avatar > System** page in the UI, you can directly query the backend health check endpoint:

```bash
http://IP_OF_YOUR_MACHINE/v1/system/healthz
```

Here `<port>` refers to the actual port of your backend service (e.g., `7897`, `9222`, etc.).

Key points:
- **No login required** (no `@login_required` decorator)
- Returns results in JSON format
- If all dependencies are healthy → HTTP **200 OK**
- If any dependency fails → HTTP **500 Internal Server Error**

### Example 1: All services healthy (HTTP 200)

```bash
http://127.0.0.1/v1/system/healthz
```

Response:

```http
HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 120

{
  "db": "ok",
  "redis": "ok",
  "doc_engine": "ok",
  "storage": "ok",
  "status": "ok"
}
```

Explanation:
- Database (MySQL/Postgres), Redis, document engine (Elasticsearch/Infinity), and object storage (MinIO) are all healthy.
- The `status` field returns `"ok"`.

### Example 2: One service unhealthy (HTTP 500)

For example, if Redis is down:

Response:

```http
HTTP/1.1 500 INTERNAL SERVER ERROR
Content-Type: application/json
Content-Length: 300

{
  "db": "ok",
  "redis": "nok",
  "doc_engine": "ok",
  "storage": "ok",
  "status": "nok",
  "_meta": {
    "redis": {
      "elapsed": "5.2",
      "error": "Lost connection!"
    }
  }
}
```

Explanation:
- `redis` is marked as `"nok"`, with detailed error info under `_meta.redis.error`.
- The overall `status` is `"nok"`, so the endpoint returns 500.

---

This endpoint allows you to monitor RAGFlow’s core dependencies programmatically in scripts or external monitoring systems, without relying on the frontend UI.
