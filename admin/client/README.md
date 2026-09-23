# RAGFlow Admin Service and CLI

RAGFlow uses the Go Admin Service and Go CLI for administration. The Admin
Service manages users, permissions, configuration, dependency health, and
registered Go services. The CLI connects to the Admin Service on port `9381`.

## Start the Admin Service

Build the Go server and CLI from the repository root:

```bash
bash build.sh --go
```

Start the Admin Service after preparing the dependencies and completing the
database migration:

```bash
./bin/ragflow_server --admin --init-superuser
```

The first superuser is `admin@ragflow.io`. Change the initial password after
the first login.

## Install and start the CLI

For regular use, install the prebuilt Go CLI.

Linux or macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/infiniflow/ragflow/main/tools/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/infiniflow/ragflow/main/tools/scripts/install.ps1 | iex
```

Verify the installation and connect to Admin:

```bash
ragflow-cli --version
ragflow-cli --admin --host 127.0.0.1:9381
```

The CLI prompts for the administrator password. See [the complete Go CLI
reference](../../docs/administrator/admin/ragflow_cli.md) for supported
commands.
