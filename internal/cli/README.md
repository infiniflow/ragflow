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
├── cli.go           # Main CLI loop and interaction
├── parser/          # Command parser package
│   ├── types.go     # Token and Command types
│   ├── lexer.go     # Lexical analyzer
│   └── parser.go    # Recursive descent parser
```

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
