# RAGFlow CLI (Go Version)

This is the Go implementation of the RAGFlow command-line interface, compatible with the Python version's syntax.

## Features

- Interactive mode only
- Full compatibility with Python CLI syntax
- Recursive descent parser for SQL-like commands
- Support for all major commands:
  - User management: LOGIN, REGISTER, CREATE USER, DROP USER, LIST USERS, etc.
  - Service management: LIST SERVICES, SHOW SERVICE, STARTUP/SHUTDOWN/RESTART SERVICE
  - Role management: CREATE ROLE, DROP ROLE, LIST ROLES, GRANT/REVOKE PERMISSION
  - Dataset management: CREATE DATASET, DROP DATASET, LIST DATASETS
  - Model management: SET/RESET DEFAULT LLM/VLM/EMBEDDING/etc.
  - And more...

## Usage

Build and run:

```bash
go build -o ragflow_cli ./cmd/ragflow_cli.go
./ragflow_cli
```

## Architecture

```
internal/cli/
├── cli.go              # Main CLI loop and interaction
├── client.go           # RAGFlowClient with Context Engine integration
├── http_client.go      # HTTP client for API communication
├── parser/             # Command parser package
│   ├── types.go        # Token and Command types
│   ├── lexer.go        # Lexical analyzer
│   └── parser.go       # Recursive descent parser
└── contextengine/      # Context Engine (Virtual Filesystem)
    ├── engine.go       # Core engine: path resolution, command routing
    ├── types.go        # Node, Command, Result types
    ├── provider.go     # Provider interface definition
    ├── dataset_provider.go  # Dataset provider implementation
    └── utils.go        # Helper functions
```

## Context Engine

The Context Engine provides a unified virtual filesystem interface over RAGFlow's RESTful APIs.

### Design Principles

1. **No Server-Side Changes**: All logic implemented client-side using existing APIs
2. **Provider Pattern**: Modular providers for different resource types (datasets, files, etc.)
3. **Unified Interface**: Common `ls`, `search`, `mkdir` commands across all providers
4. **Path-Based Navigation**: Virtual paths like `/datasets`, `/datasets/{name}/files`

### Supported Paths

| Path | Description |
|------|-------------|
| `/datasets` | List all datasets |
| `/datasets/{name}` | Get dataset info |
| `/datasets/{name}/files` | List documents in dataset |
| `/datasets/{name}/files/{doc}` | Get document info |

### Commands

- `ls [path]` - List nodes at path
- `search [path] WHERE query='...'` - Search nodes
- `mkdir [path]` - Create new resource (dataset)

## Command Examples

```sql
-- Authentication
LOGIN USER 'admin@example.com';

-- User management
REGISTER USER 'john' AS 'John Doe' PASSWORD 'secret';
CREATE USER 'jane' 'password123';
DROP USER 'jane';
LIST USERS;
SHOW USER 'john';

-- Service management
LIST SERVICES;
SHOW SERVICE 1;
STARTUP SERVICE 1;
SHUTDOWN SERVICE 1;
RESTART SERVICE 1;
PING;

-- Role management
CREATE ROLE admin DESCRIPTION 'Administrator role';
LIST ROLES;
GRANT read,write ON datasets TO ROLE admin;

-- Dataset management
CREATE DATASET 'my_dataset' WITH EMBEDDING 'text-embedding-ada-002' PARSER 'naive';
LIST DATASETS;
DROP DATASET 'my_dataset';

-- Model configuration
SET DEFAULT LLM 'gpt-4';
SET DEFAULT EMBEDDING 'text-embedding-ada-002';
RESET DEFAULT LLM;

-- Context Engine (Virtual Filesystem)
ls datasets;                              -- List all datasets
ls datasets/my_dataset;                   -- Show dataset info
ls datasets/my_dataset/files;             -- List documents in dataset
search datasets WHERE query='test';       -- Search datasets
mkdir datasets/new_dataset;               -- Create new dataset

-- Meta commands
\?          -- Show help
\q          -- Quit
\c          -- Clear screen
```

## Parser Implementation

The parser uses a hand-written recursive descent approach instead of go-yacc for:
- Better control over error messages
- Easier to extend and maintain
- No code generation step required

The parser structure follows the grammar defined in the Python version, ensuring full syntax compatibility.
