---
sidebar_position: 0
slug: /admin_service
sidebar_custom_props: {
  categoryIcon: LucideActivity
}
---
# Admin Service

The Admin Service is the core backend management service of the RAGFlow system, providing comprehensive system administration capabilities through centralized API interfaces for managing and controlling the entire platform. Adopting a client-server architecture, it supports access and operations via both a Web UI and an Admin CLI, ensuring flexible and efficient execution of administrative tasks.

The core functions of the Admin Service include real-time monitoring of the operational status of the RAGFlow server and its critical dependent components—such as MySQL, Elasticsearch, Redis, and MinIO—along with full-featured user management. In administrator mode, it enables key operations such as viewing user information, creating users, updating passwords, modifying activation status, and performing complete user data deletion. These functions remain accessible via the Admin CLI even when the web management interface is disabled, ensuring the system stays under control at all times.

With its unified interface design, the Admin Service combines the convenience of visual administration with the efficiency and stability of command-line operations, serving as a crucial foundation for the reliable operation and secure management of the RAGFlow system.

## Starting the Admin Service

### Launching from source code

1. Before start Admin Service, please make sure RAGFlow system is already started.

2. Launch from source code:

   ```bash
   python admin/server/admin_server.py
   ```

   The service will start and listen for incoming connections from the CLI on the configured port. 

### Using docker image

1. Before startup, please configure the `docker_compose.yml`  file to enable admin server:

   ```bash
   command:
     - --enable-adminserver
   ```

2. Start the containers, the service will start and listen for incoming connections from the CLI on the configured port.

