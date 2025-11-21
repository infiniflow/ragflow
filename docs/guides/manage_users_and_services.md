---
sidebar_position: 6
slug: /manage_users_and_services
---


# Admin CLI and Admin Service



The Admin CLI and Admin Service form a client-server architectural suite for RAGflow system administration. The Admin CLI serves as an interactive command-line interface that receives instructions and displays execution results from the Admin Service in real-time. This duo enables real-time monitoring of system operational status, supporting visibility into RAGflow Server services and dependent components including MySQL, Elasticsearch, Redis, and MinIO. In administrator mode, they provide user management capabilities that allow viewing users and performing critical operations—such as user creation, password updates, activation status changes, and comprehensive user data deletion—even when corresponding web interface functionalities are disabled.



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



## Using the Admin CLI

1. Ensure the Admin Service is running.

2. Install ragflow-cli.

   ```bash
   pip install ragflow-cli==0.22.1
   ```

3. Launch the CLI client:

   ```bash
   ragflow-cli -h 127.0.0.1 -p 9381
   ```

    You will be prompted to enter the superuser's password to log in.
    The default password is admin.

    **Parameters:**
    
    - -h: RAGFlow admin server host address
    
    - -p: RAGFlow admin server port

## Default administrative account

- Username: admin@ragflow.io
- Password: admin

## Supported Commands

Commands are case-insensitive and must be terminated with a semicolon(;).

### Service manage commands

`LIST SERVICES;`

- Lists all available services within the RAGFlow system.

- [Example](#example-list-services)

`SHOW SERVICE <id>;`

- Shows detailed status information for the service identified by **id**.
- [Example](#example-show-service)

`SHOW VERSION;`

- Shows RAGFlow version.
- [Example](#example-show-version)

### User Management Commands

`LIST USERS;`

- Lists all users known to the system.
- [Example](#example-list-users)

`SHOW USER <username>;`

- Shows details and permissions for the user specified by **email**. The username must be enclosed in single or double quotes.
- [Example](#example-show-user)

`CREATE USER <username> <password>;`

- Create user by username and password. The username and password must be enclosed in single or double quotes.
- [Example](#example-create-user)

`DROP USER <username>;`

- Removes the specified user from the system. Use with caution.
- [Example](#example-drop-user)

`ALTER USER PASSWORD <username> <new_password>;`

- Changes the password for the specified user.
- [Example](#example-alter-user-password)

`ALTER USER ACTIVE <username> <on/off>;`

- Changes the user to active or inactive.
- [Example](#example-alter-user-active)

### Data and Agent Commands

`LIST DATASETS OF <username>;`

- Lists the datasets associated with the specified user.
- [Example](#example-list-datasets-of-user)

`LIST AGENTS OF <username>;`

- Lists the agents associated with the specified user.
- [Example](#example-list-agents-of-user)

### Meta-Commands

- \? or \help
  Shows help information for the available commands.
- \q or \quit
  Exits the CLI application.
- [Example](#example-meta-commands)

### Examples

<span id="example-list-services"></span>

- List all available services.

```
admin> list services;
command: list services;
Listing all services
+-------------------------------------------------------------------------------------------+-----------+----+---------------+-------+----------------+---------+
| extra                                                                                     | host      | id | name          | port  | service_type   | status  |
+-------------------------------------------------------------------------------------------+-----------+----+---------------+-------+----------------+---------+
| {}                                                                                        | 0.0.0.0   | 0  | ragflow_0     | 9380  | ragflow_server | Timeout |
| {'meta_type': 'mysql', 'password': 'infini_rag_flow', 'username': 'root'}                 | localhost | 1  | mysql         | 5455  | meta_data      | Alive   |
| {'password': 'infini_rag_flow', 'store_type': 'minio', 'user': 'rag_flow'}                | localhost | 2  | minio         | 9000  | file_store     | Alive   |
| {'password': 'infini_rag_flow', 'retrieval_type': 'elasticsearch', 'username': 'elastic'} | localhost | 3  | elasticsearch | 1200  | retrieval      | Alive   |
| {'db_name': 'default_db', 'retrieval_type': 'infinity'}                                   | localhost | 4  | infinity      | 23817 | retrieval      | Timeout |
| {'database': 1, 'mq_type': 'redis', 'password': 'infini_rag_flow'}                        | localhost | 5  | redis         | 6379  | message_queue  | Alive   |
+-------------------------------------------------------------------------------------------+-----------+----+---------------+-------+----------------+---------+

```

<span id="example-show-service"></span>

- Show ragflow_server.

```
admin> show service 0;
command: show service 0;
Showing service: 0
Service ragflow_0 is alive. Detail:
Confirm elapsed: 26.0 ms.
```

- Show mysql.

```
admin> show service 1;
command: show service 1;
Showing service: 1
Service mysql is alive. Detail:
+---------+----------+------------------+------+------------------+------------------------+-------+-----------------+
| command | db       | host             | id   | info             | state                  | time  | user            |
+---------+----------+------------------+------+------------------+------------------------+-------+-----------------+
| Daemon  | None     | localhost        | 5    | None             | Waiting on empty queue | 16111 | event_scheduler |
| Sleep   | rag_flow | 172.18.0.1:40046 | 1610 | None             |                        | 2     | root            |
| Query   | rag_flow | 172.18.0.1:35882 | 1629 | SHOW PROCESSLIST | init                   | 0     | root            |
+---------+----------+------------------+------+------------------+------------------------+-------+-----------------+
```

- Show minio.

```
admin> show service 2;
command: show service 2;
Showing service: 2
Service minio is alive. Detail:
Confirm elapsed: 2.1 ms.
```

- Show elasticsearch.

```
admin> show service 3;
command: show service 3;
Showing service: 3
Service elasticsearch is alive. Detail:
+----------------+------+--------------+---------+----------------+--------------+---------------+--------------+------------------------------+----------------------------+-----------------+-------+---------------+---------+-------------+---------------------+--------+------------+--------------------+
| cluster_name   | docs | docs_deleted | indices | indices_shards | jvm_heap_max | jvm_heap_used | jvm_versions | mappings_deduplicated_fields | mappings_deduplicated_size | mappings_fields | nodes | nodes_version | os_mem  | os_mem_used | os_mem_used_percent | status | store_size | total_dataset_size |
+----------------+------+--------------+---------+----------------+--------------+---------------+--------------+------------------------------+----------------------------+-----------------+-------+---------------+---------+-------------+---------------------+--------+------------+--------------------+
| docker-cluster | 717  | 86           | 37      | 42             | 3.76 GB      | 1.74 GB       | 21.0.1+12-29 | 6575                         | 48.0 KB                    | 8521            | 1     | ['8.11.3']    | 7.52 GB | 4.55 GB     | 61                  | green  | 4.60 MB    | 4.60 MB            |
+----------------+------+--------------+---------+----------------+--------------+---------------+--------------+------------------------------+----------------------------+-----------------+-------+---------------+---------+-------------+---------------------+--------+------------+--------------------+
```

- Show infinity.

```
admin> show service 4;
command: show service 4;
Showing service: 4
Fail to show service, code: 500, message: Infinity is not in use.
```

- Show redis.

```
admin> show service 5;
command: show service 5;
Showing service: 5
Service redis is alive. Detail:
+-----------------+-------------------+---------------------------+-------------------------+---------------+-------------+--------------------------+---------------------+-------------+
| blocked_clients | connected_clients | instantaneous_ops_per_sec | mem_fragmentation_ratio | redis_version | server_mode | total_commands_processed | total_system_memory | used_memory |
+-----------------+-------------------+---------------------------+-------------------------+---------------+-------------+--------------------------+---------------------+-------------+
| 0               | 2                 | 1                         | 10.41                   | 7.2.4         | standalone  | 10446                    | 30.84G              | 1.10M       |
+-----------------+-------------------+---------------------------+-------------------------+---------------+-------------+--------------------------+---------------------+-------------+
```
<span id="example-show-version"></span>

- Show RAGFlow version

```
admin> show version;
+-----------------------+
| version               |
+-----------------------+
| v0.21.0-241-gc6cf58d5 |
+-----------------------+
```

<span id="example-list-users"></span>

- List all user.

```
admin> list users;
command: list users;
Listing all users
+-------------------------------+----------------------+-----------+----------+
| create_date                   | email                | is_active | nickname |
+-------------------------------+----------------------+-----------+----------+
| Mon, 22 Sep 2025 10:59:04 GMT | admin@ragflow.io     | 1         | admin    |
| Sun, 14 Sep 2025 17:36:27 GMT | lynn_inf@hotmail.com | 1         | Lynn     |
+-------------------------------+----------------------+-----------+----------+
```

<span id="example-show-user"></span>

- Show specified user.

```
admin> show user "admin@ragflow.io";
command: show user "admin@ragflow.io";
Showing user: admin@ragflow.io
+-------------------------------+------------------+-----------+--------------+------------------+--------------+----------+-----------------+---------------+--------+-------------------------------+
| create_date                   | email            | is_active | is_anonymous | is_authenticated | is_superuser | language | last_login_time | login_channel | status | update_date                   |
+-------------------------------+------------------+-----------+--------------+------------------+--------------+----------+-----------------+---------------+--------+-------------------------------+
| Mon, 22 Sep 2025 10:59:04 GMT | admin@ragflow.io | 1         | 0            | 1                | True         | Chinese  | None            | None          | 1      | Mon, 22 Sep 2025 10:59:04 GMT |
+-------------------------------+------------------+-----------+--------------+------------------+--------------+----------+-----------------+---------------+--------+-------------------------------+
```

<span id="example-create-user"></span>

- Create new user.

```
admin> create user "example@ragflow.io" "psw";
command: create user "example@ragflow.io" "psw";
Create user: example@ragflow.io, password: psw, role: user
+----------------------------------+--------------------+----------------------------------+--------------+---------------+----------+
| access_token                     | email              | id                               | is_superuser | login_channel | nickname |
+----------------------------------+--------------------+----------------------------------+--------------+---------------+----------+
| 5cdc6d1e9df111f099b543aee592c6bf | example@ragflow.io | 5cdc6ca69df111f099b543aee592c6bf | False        | password      |          |
+----------------------------------+--------------------+----------------------------------+--------------+---------------+----------+
```

<span id="example-alter-user-password"></span>

- Alter user password.

```
admin> alter user password "example@ragflow.io" "newpsw";
command: alter user password "example@ragflow.io" "newpsw";
Alter user: example@ragflow.io, password: newpsw
Password updated successfully!
```

<span id="example-alter-user-active"></span>

- Alter user active, turn off.

```
admin> alter user active "example@ragflow.io" off;
command: alter user active "example@ragflow.io" off;
Alter user example@ragflow.io activate status, turn off.
Turn off user activate status successfully!
```

<span id="example-drop-user"></span>

- Drop user.

```
admin> Drop user "example@ragflow.io";
command: Drop user "example@ragflow.io";
Drop user: example@ragflow.io
Successfully deleted user. Details:
Start to delete owned tenant.
- Deleted 2 tenant-LLM records.
- Deleted 0 langfuse records.
- Deleted 1 tenant.
- Deleted 1 user-tenant records.
- Deleted 1 user.
Delete done!
```

Delete user's data at the same time.

<span id="example-list-datasets-of-user"></span>

- List the specified user's dataset.

```
admin> list datasets of "lynn_inf@hotmail.com";
command: list datasets of "lynn_inf@hotmail.com";
Listing all datasets of user: lynn_inf@hotmail.com
+-----------+-------------------------------+---------+----------+---------------+------------+--------+-----------+-------------------------------+
| chunk_num | create_date                   | doc_num | language | name          | permission | status | token_num | update_date                   |
+-----------+-------------------------------+---------+----------+---------------+------------+--------+-----------+-------------------------------+
| 29        | Mon, 15 Sep 2025 11:56:59 GMT | 12      | Chinese  | test_dataset  | me         | 1      | 12896     | Fri, 19 Sep 2025 17:50:58 GMT |
| 4         | Sun, 28 Sep 2025 11:49:31 GMT | 6       | Chinese  | dataset_share | team       | 1      | 1121      | Sun, 28 Sep 2025 14:41:03 GMT |
+-----------+-------------------------------+---------+----------+---------------+------------+--------+-----------+-------------------------------+
```

<span id="example-list-agents-of-user"></span>

- List the specified user's agents.

```
admin> list agents of "lynn_inf@hotmail.com";
command: list agents of "lynn_inf@hotmail.com";
Listing all agents of user: lynn_inf@hotmail.com
+-----------------+-------------+------------+-----------------+
| canvas_category | canvas_type | permission | title           |
+-----------------+-------------+------------+-----------------+
| agent           | None        | team       | research_helper |
+-----------------+-------------+------------+-----------------+
```

<span id="example-meta-commands"></span>

- Show help information.

```
admin> \help
command: \help

Commands:
  LIST SERVICES
  SHOW SERVICE <service>
  STARTUP SERVICE <service>
  SHUTDOWN SERVICE <service>
  RESTART SERVICE <service>
  LIST USERS
  SHOW USER <user>
  DROP USER <user>
  CREATE USER <user> <password>
  ALTER USER PASSWORD <user> <new_password>
  ALTER USER ACTIVE <user> <on/off>
  LIST DATASETS OF <user>
  LIST AGENTS OF <user>

Meta Commands:
  \?, \h, \help     Show this help
  \q, \quit, \exit   Quit the CLI
```

- Exit

```
admin> \q
command: \q
Goodbye!
```

