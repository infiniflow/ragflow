---
sidebar_position: 2
title: RAGFlow CLI
sidebar_label: RAGFlow CLI
slug: /admin_cli
sidebar_custom_props: {
  categoryIcon: LucideSquareTerminal
}
---
# RAGFlow CLI

RAGFlow CLI is the Go command-line client for administering RAGFlow. In Admin mode it connects to the Go Admin Service, manages users and system settings, and shows the health of dependencies and registered RAGFlow processes.

## Install and start

For regular use, install the prebuilt Go CLI from the latest RAGFlow GitHub Release. The installer detects the operating system and CPU architecture, downloads the matching binary, verifies it against `SHA256SUMS`, and then installs it.

默认安装（Linux/macOS）：

```bash
curl -fsSL https://raw.githubusercontent.com/infiniflow/ragflow/main/tools/scripts/install.sh | sh
```

不设置版本号时，安装脚本会解析最新 GitHub Release，并根据当前操作系统
和 CPU 架构安装匹配的 CLI。Linux 和 macOS 也可以通过 `VERSION` 指定版本：

```bash
curl -fsSL https://raw.githubusercontent.com/infiniflow/ragflow/main/tools/scripts/install.sh \
  | VERSION=v0.27.2 sh
```

不指定版本和指定版本都可以正常安装。生产环境或需要重复执行的安装流程，
建议指定版本，避免后续 Release 变化导致安装结果不同。

The default installation path is `/usr/local/bin/ragflow-cli`. If the current user cannot write to that directory, the installer requests `sudo` permission. To install into a user-writable directory instead, set `INSTALL_DIR` and ensure that directory is on `PATH`:

```bash
curl -fsSL https://raw.githubusercontent.com/infiniflow/ragflow/main/tools/scripts/install.sh | INSTALL_DIR="$HOME/.local/bin" sh
```

默认安装（Windows PowerShell）：

```powershell
irm https://raw.githubusercontent.com/infiniflow/ragflow/main/tools/scripts/install.ps1 | iex
```

Windows PowerShell 不指定版本时同样使用最新 GitHub Release。需要固定版本时，
下载脚本后传入 `-Version` 参数：

```powershell
irm https://raw.githubusercontent.com/infiniflow/ragflow/main/tools/scripts/install.ps1 -OutFile install.ps1
./install.ps1 -Version v0.27.2
```

The Windows installer installs `ragflow-cli.exe` under `%LOCALAPPDATA%\Programs\RAGFlow` by default and adds that directory to the user `PATH`. Restart the terminal if the installer reports that `PATH` was updated.

Verify the installation:

```bash
ragflow-cli --version
```

The installation scripts are maintained in [`tools/scripts/install.sh`](https://github.com/infiniflow/ragflow/blob/main/tools/scripts/install.sh) and [`tools/scripts/install.ps1`](https://github.com/infiniflow/ragflow/blob/main/tools/scripts/install.ps1).

If you are developing or modifying the CLI, build the Go server and CLI binaries from the repository root instead:

```bash
bash build.sh --go
```

Before starting Admin, start the required dependencies and complete the standalone database migration as described in [Start supporting services](../../develop/launch_ragflow_from_source.md#2-start-supporting-services) and [Migrate and launch the Go backend](../../develop/launch_ragflow_from_source.md#3-migrate-and-launch-the-go-backend). The CLI does not start or migrate the server for you.

For a source-development checkout, start the Admin Service with the same `RAGFLOW_DEV_MODE=true` setting used for the migration. Do not set this variable in production. Start Admin before the API server, ingestors, and syncers:

```bash
RAGFLOW_DEV_MODE=true ./bin/ragflow_server --admin --init-superuser
```

If this creates the first superuser, its email is `admin@ragflow.io` and its initial password is `admin`. Change that password immediately after the first login. The option does not reset an existing superuser's password.

Then start the CLI in Admin mode. It connects to `127.0.0.1:9381` by default:

```bash
ragflow-cli --admin
```

When using a binary built from source, replace `ragflow-cli` with `./bin/ragflow-cli` in the following examples.

To connect to another Admin Service, pass a `host:port` value:

```bash
ragflow-cli --admin --host 192.0.2.10:9381
```

To log in when starting the CLI, provide the administrator email address and enter the password at the prompt:

```bash
ragflow-cli --admin \
  --host 127.0.0.1:9381 \
  --user admin@ragflow.io
```

Avoid passing a real password with `--password`: command-line arguments can be visible to other local processes and may be retained in shell history. If you used the initial password, change it after logging in with `ALTER USER PASSWORD 'admin@ragflow.io' '<new_password>';`.

| Option | Description |
| --- | --- |
| `--admin`, `-admin` | Start in Admin mode. |
| `-h`, `--host <host:port>` | Admin Service address. The default is `127.0.0.1:9381`. |
| `-u`, `--user <email>` | Administrator email address. |
| `-p`, `--password <password>` | Administrator password. Prefer the interactive prompt to avoid exposing it in command-line arguments. |
| `-k`, `--key <path>` | Key file used by the client. |
| `-o`, `--output <format>` | Output format: `table`, `plain`, or `json`. |
| `-v`, `--verbose` | Enable verbose output. |

## Commands

### Syntax conventions

- `<parameter>` is required and must be replaced with an actual value.
- `[OPTION '<value>']` is optional. Omit the entire segment when it is not needed.
- Command keywords are case-insensitive and are shown in uppercase.
- Keep the quotation marks around string values.
- End SQL-like commands with a semicolon (`;`).
- `RAGFlow(admin)>` is the interactive prompt. Enter only the command after the prompt.
- Commands that access protected Admin resources require an authenticated administrator session. `LOGIN ADMIN`, `PING`, `SHOW VERSION`, `SHOW CURRENT`, `SHOW ADMIN SERVER`, `LIST API SERVER`, `SHOW API SERVER`, and meta-commands do not require an existing login.

### 1. Session and server commands

#### 1.1 LOGIN ADMIN

Logs in to the Admin Service with an administrator account. If `PASSWORD` is omitted, the CLI prompts for the password.

**Syntax**

```sql
LOGIN ADMIN '<email>' [PASSWORD '<password>'];
```

| Parameter | Required | Description |
| --- | --- | --- |
| `<email>` | Yes | Administrator email address. |
| `[PASSWORD '<password>']` | No | Administrator password. Omit this segment to enter the password interactively. |

**Example**

```text
RAGFlow(admin)> LOGIN ADMIN 'admin@ragflow.io' PASSWORD '<password>';
```

#### 1.2 LOGOUT

Logs out of the current Admin session and clears the login token.

**Syntax**

```sql
LOGOUT;
```

**Example**

```text
RAGFlow(admin)> LOGOUT;
SUCCESS
```

#### 1.3 PING

Checks whether the Admin Service is reachable.

**Syntax**

```sql
PING;
```

**Example**

```text
RAGFlow(admin)> PING;
SUCCESS
```

#### 1.4 SHOW VERSION

Shows the RAGFlow version and edition reported by the Admin Service.

**Syntax**

```sql
SHOW VERSION;
```

**Example**

```text
RAGFlow(admin)> SHOW VERSION;
```

#### 1.5 SHOW CURRENT

Shows the current CLI mode, server connection, authentication state, and output format.

**Syntax**

```sql
SHOW CURRENT;
```

**Example**

```text
RAGFlow(admin)> SHOW CURRENT;
```

#### 1.6 SHOW ADMIN SERVER

Shows the Admin Service connection stored by the CLI.

**Syntax**

```sql
SHOW ADMIN SERVER;
```

**Example**

```text
RAGFlow(admin)> SHOW ADMIN SERVER;
```

### 2. Service commands

The Admin Service combines dependency health checks with heartbeat registrations from Go API servers, ingestors, and file syncers. The runtime service types are `api_server`, `ingestor`, and `file_syncer`. The former `task_executor` service type is not used.

#### 2.1 LIST SERVICES

Lists infrastructure dependencies and runtime services registered through heartbeats.

**Syntax**

```sql
LIST SERVICES;
```

**Example**

```text
RAGFlow(admin)> LIST SERVICES;
```

The result can include MySQL, Elasticsearch, MinIO, the Kvrocks cache, the NATS message queue, Go API servers, ingestors, and file syncers.

#### 2.2 SHOW SERVICE

Shows the current status of one service. Use the service name returned by `LIST SERVICES`, not a numeric ID.

**Syntax**

```sql
SHOW SERVICE '<service_name>';
```

| Parameter | Required | Description |
| --- | --- | --- |
| `<service_name>` | Yes | Service name returned by `LIST SERVICES`, such as `mysql`. |

**Example**

```text
RAGFlow(admin)> SHOW SERVICE 'mysql';
```

### 3. User commands

#### 3.1 LIST USERS

Lists RAGFlow users.

**Syntax**

```sql
LIST USERS;
```

**Example**

```text
RAGFlow(admin)> LIST USERS;
```

#### 3.2 SHOW USER

Shows details for one user.

**Syntax**

```sql
SHOW USER '<email>';
```

| Parameter | Required | Description |
| --- | --- | --- |
| `<email>` | Yes | User email address. |

**Example**

```text
RAGFlow(admin)> SHOW USER 'alice@example.com';
```

#### 3.3 CREATE USER

Creates a user with the standard `user` role.

**Syntax**

```sql
CREATE USER '<email>' '<password>';
```

| Parameter | Required | Description |
| --- | --- | --- |
| `<email>` | Yes | Email address for the new user. |
| `<password>` | Yes | Initial password for the new user. |

**Example**

```text
RAGFlow(admin)> CREATE USER 'alice@example.com' 'Alice@123456';
SUCCESS
```

#### 3.4 ALTER USER ACTIVE

Activates or deactivates a user.

**Syntax**

```sql
ALTER USER ACTIVE '<email>' <on|off>;
```

| Parameter | Required | Description |
| --- | --- | --- |
| `<email>` | Yes | User email address. |
| `<on\|off>` | Yes | `on` activates the user; `off` deactivates the user. |

**Example**

```text
RAGFlow(admin)> ALTER USER ACTIVE 'alice@example.com' off;
SUCCESS
```

#### 3.5 ALTER USER PASSWORD

Changes a user's password.

**Syntax**

```sql
ALTER USER PASSWORD '<email>' '<new_password>';
```

| Parameter | Required | Description |
| --- | --- | --- |
| `<email>` | Yes | User email address. |
| `<new_password>` | Yes | New password. |

**Example**

```text
RAGFlow(admin)> ALTER USER PASSWORD 'alice@example.com' 'NewPassword@123';
SUCCESS
```

#### 3.6 DROP USER

Deletes a user and associated data.

An active user cannot be deleted. Run `ALTER USER ACTIVE '<email>' off;` before `DROP USER`. Otherwise, the Admin Service returns `user is active and can't be deleted. Please deactivate the user first`.

**Syntax**

```sql
DROP USER '<email>';
```

| Parameter | Required | Description |
| --- | --- | --- |
| `<email>` | Yes | Email address of a deactivated user. |

**Example**

```text
RAGFlow(admin)> ALTER USER ACTIVE 'alice@example.com' off;
SUCCESS
RAGFlow(admin)> DROP USER 'alice@example.com';
SUCCESS
```

### 4. Configuration commands

#### 4.1 SHOW VAR

Shows a runtime setting by its exact name or name prefix.

**Syntax**

```sql
SHOW VAR '<name>';
```

| Parameter | Required | Description |
| --- | --- | --- |
| `<name>` | Yes | Setting name or prefix, such as `mail.timeout`. |

**Example**

```text
RAGFlow(admin)> SHOW VAR 'mail.timeout';
```

#### 4.2 LIST VARS

Lists runtime settings.

**Syntax**

```sql
LIST VARS;
```

**Example**

```text
RAGFlow(admin)> LIST VARS;
```

#### 4.3 LIST CONFIGS

Lists the effective Admin Service configuration. This command does not list service health; use `LIST SERVICES` for that purpose.

**Syntax**

```sql
LIST CONFIGS;
```

**Example**

```text
RAGFlow(admin)> LIST CONFIGS;
```

#### 4.4 LIST ENVS

Lists the environment summary visible to the Admin Service.

**Syntax**

```sql
LIST ENVS;
```

**Example**

```text
RAGFlow(admin)> LIST ENVS;
```

### 5. Ingestion commands

#### 5.1 LIST INGESTORS

Lists ingestors that have registered with the Admin Service through heartbeats.

**Syntax**

```sql
LIST INGESTORS;
```

**Example**

```text
RAGFlow(admin)> LIST INGESTORS;
```

#### 5.2 LIST INGESTION TASKS

Lists ingestion tasks known to the Admin Service.

**Syntax**

```sql
LIST INGESTION TASKS;
```

**Example**

```text
RAGFlow(admin)> LIST INGESTION TASKS;
```

### 6. API server commands

`LIST API SERVER` and `SHOW API SERVER` inspect API server connections saved in the CLI configuration. They do not query the Admin Service heartbeat registry. To find running Go API servers registered by heartbeat, use `LIST SERVICES` and look for `type=api_server`.

#### 6.1 LIST API SERVER

Lists API server connections saved in the local CLI configuration.

**Syntax**

```sql
LIST API SERVER;
```

**Example**

```text
RAGFlow(admin)> LIST API SERVER;
```

An empty local configuration produces `No data to print` even when a Go API server is running and registered with the Admin Service.

#### 6.2 SHOW API SERVER

Shows one API server connection from the local CLI configuration.

**Syntax**

```sql
SHOW API SERVER '<server_name>';
```

| Parameter | Required | Description |
| --- | --- | --- |
| `<server_name>` | Yes | Local API server configuration name, such as `default`. |

**Example**

```text
RAGFlow(admin)> SHOW API SERVER 'default';
```

If the name does not exist in the local configuration, the command returns `api_server=N/A`.

### 7. Message queue commands

The MQ commands operate on the NATS JetStream task stream used by ingestors. When an ingestor is running, it can consume a published test message before a subsequent `MQ LIST` or `MQ PULL` command observes it.

#### 7.1 MQ SHOW

Shows message queue statistics, including consumer, message, pending, waiting, and acknowledgement counts.

**Syntax**

```sql
MQ SHOW;
```

**Example**

```text
RAGFlow(admin)> MQ SHOW;
```

#### 7.2 MQ LIST

Lists messages currently retained in the task stream. The optional `PENDING` keyword is accepted by the CLI.

**Syntax**

```sql
MQ LIST [PENDING];
```

| Parameter | Required | Description |
| --- | --- | --- |
| `[PENDING]` | No | Requests the pending-message form of the command. |

**Example**

```text
RAGFlow(admin)> MQ LIST;
```

#### 7.3 MQ PUBLISH

Publishes a test message to the ingestion task subject.

**Syntax**

```sql
MQ PUBLISH '<message>';
```

| Parameter | Required | Description |
| --- | --- | --- |
| `<message>` | Yes | String stored as the test task identifier. |

**Example**

```text
RAGFlow(admin)> MQ PUBLISH 'manual-ingestion-test';
SUCCESS
```

A successful response confirms that NATS JetStream accepted the message. If an ingestor is waiting for work, it can consume and acknowledge the message immediately.

#### 7.4 MQ PULL

Manually pulls messages from the ingestion task consumer. The default count is `1`. By default, pulled messages are acknowledged; `NOACK` negatively acknowledges them so that they can be redelivered.

**Syntax**

```sql
MQ PULL [<count>] [NOACK];
```

| Parameter | Required | Description |
| --- | --- | --- |
| `[<count>]` | No | Number of messages to pull. The default is `1` and the value must be within the CLI's allowed range. |
| `[NOACK]` | No | Negatively acknowledges pulled messages instead of acknowledging them. |

**Example**

```text
RAGFlow(admin)> MQ PULL 1 NOACK;
```

### 8. Meta-commands

#### 8.1 HELP

Shows CLI help.

**Syntax**

```text
\?
\h
\help
```

**Example**

```text
RAGFlow(admin)> \help
```

#### 8.2 PWD

Shows the current working directory.

**Syntax**

```text
\pwd
```

**Example**

```text
RAGFlow(admin)> \pwd
```

#### 8.3 QUIT

Exits the CLI.

**Syntax**

```text
\q
\quit
\exit
```

**Example**

```text
RAGFlow(admin)> \q
Goodbye!
```
