# RAGFlow Admin Service & CLI

### Introduction

Admin Service is a dedicated management component designed to monitor, maintain, and administrate the RAGFlow system. It provides comprehensive tools for ensuring system stability, performing operational tasks, and managing users and permissions efficiently.

The service offers real-time monitoring of critical components, including the RAGFlow server, Task Executor processes, and dependent services such as MySQL, Infinity, Elasticsearch, Redis, and MinIO. It automatically checks their health status, resource usage, and uptime, and performs restarts in case of failures to minimize downtime.

For user and system management, it supports listing, creating, modifying, and deleting users and their associated resources like knowledge bases and Agents.

Built with scalability and reliability in mind, the Admin Service ensures smooth system operation and simplifies maintenance workflows.

It consists of a server-side Service and a command-line client (CLI), both implemented in Python. User commands are parsed using the Lark parsing toolkit.

- **Admin Service**: A backend service that interfaces with the RAGFlow system to execute administrative operations and monitor its status.
- **Admin CLI**: A command-line interface that allows users to connect to the Admin Service and issue commands for system management.



### Starting the Admin Service

#### Launching from source code

1. Before start Admin Service, please make sure RAGFlow system is already started.

2. Launch from source code:

   ```bash
   python admin/server/admin_server.py
   ```
   The service will start and listen for incoming connections from the CLI on the configured port. 

#### Using docker image

1. Before startup, please configure the `docker_compose.yml`  file to enable admin server:

   ```bash
   command:
     - --enable-adminserver
   ```

2. Start the containers, the service will start and listen for incoming connections from the CLI on the configured port.



### Using the Admin CLI

1.  Ensure the Admin Service is running.
2.  Install ragflow-cli.
    ```bash
    pip install ragflow-cli==0.23.1
    ```
3.  Launch the CLI client:
    ```bash
    ragflow-cli -h 127.0.0.1 -p 9381
    ```
    You will be prompted to enter the superuser's password to log in.
    The default password is admin.

    **Parameters:**
    
    - -h: RAGFlow admin server host address
    
    - -p: RAGFlow admin server port



## Supported Commands

Commands are case-insensitive and must be terminated with a semicolon (`;`).

### Service Management Commands

-   `LIST SERVICES;`
    -   Lists all available services within the RAGFlow system.
-   `SHOW SERVICE <id>;`
    -   Shows detailed status information for the service identified by `<id>`.


### User Management Commands

-   `LIST USERS;`
    -   Lists all users known to the system.
-   `SHOW USER '<username>';`
    -   Shows details and permissions for the specified user. The username must be enclosed in single or double quotes.

- `CREATE USER <username> <password>;`
  - Create user by username and password. The username and password must be enclosed in single or double quotes.

-   `DROP USER '<username>';`
    -   Removes the specified user from the system. Use with caution.
-   `ALTER USER PASSWORD '<username>' '<new_password>';`
    -   Changes the password for the specified user.
-   `ALTER USER ACTIVE <username> <on/off>;`
    -   Changes the user to active or inactive.


### Data and Agent Commands

-   `LIST DATASETS OF '<username>';`
    -   Lists the datasets associated with the specified user.
-   `LIST AGENTS OF '<username>';`
    -   Lists the agents associated with the specified user.

### Meta-Commands

Meta-commands are prefixed with a backslash (`\`).

-   `\?` or `\help`
    -   Shows help information for the available commands.
-   `\q` or `\quit`
    -   Exits the CLI application.

## Examples

```commandline
admin> list users;
+-------------------------------+------------------------+-----------+-------------+
| create_date                   | email                  | is_active | nickname    |
+-------------------------------+------------------------+-----------+-------------+
| Fri, 22 Nov 2024 16:03:41 GMT | jeffery@infiniflow.org | 1         | Jeffery     |
| Fri, 22 Nov 2024 16:10:55 GMT | aya@infiniflow.org     | 1         | Waterdancer |
+-------------------------------+------------------------+-----------+-------------+

admin> list services;
+-------------------------------------------------------------------------------------------+-----------+----+---------------+-------+----------------+
| extra                                                                                     | host      | id | name          | port  | service_type   |
+-------------------------------------------------------------------------------------------+-----------+----+---------------+-------+----------------+
| {}                                                                                        | 0.0.0.0   | 0  | ragflow_0     | 9380  | ragflow_server |
| {'meta_type': 'mysql', 'password': 'infini_rag_flow', 'username': 'root'}                 | localhost | 1  | mysql         | 5455  | meta_data      |
| {'password': 'infini_rag_flow', 'store_type': 'minio', 'user': 'rag_flow'}                | localhost | 2  | minio         | 9000  | file_store     |
| {'password': 'infini_rag_flow', 'retrieval_type': 'elasticsearch', 'username': 'elastic'} | localhost | 3  | elasticsearch | 1200  | retrieval      |
| {'db_name': 'default_db', 'retrieval_type': 'infinity'}                                   | localhost | 4  | infinity      | 23817 | retrieval      |
| {'database': 1, 'mq_type': 'redis', 'password': 'infini_rag_flow'}                        | localhost | 5  | redis         | 6379  | message_queue  |
+-------------------------------------------------------------------------------------------+-----------+----+---------------+-------+----------------+
```
