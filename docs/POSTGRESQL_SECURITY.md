# PostgreSQL Security: Sandboxed User Setup

## Overview

By default, RAGFlow uses PostgreSQL superuser credentials (`postgres`) for database operations, mirroring the MySQL approach which uses `root`. This simplifies deployment but grants the application full access to the PostgreSQL server.

For production environments or shared database servers, you may want to create a **restricted user** with limited permissions to contain potential security breaches.

## Default Configuration (Superuser)

**Docker `.env` file:**
```shell
POSTGRES_USER=postgres
POSTGRES_PASSWORD=your_password
POSTGRES_DBNAME=ragflow_db
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
```

**`service_conf.yaml`:**
```yaml
postgres:
  user: 'postgres'           # Superuser (default)
  password: 'your_password'
  name: 'ragflow_db'
  host: 'localhost'
  port: 5432
```

**Pros:**
- ✅ Simple setup (no manual DB preparation)
- ✅ Automatic database creation
- ✅ Works with Docker and existing servers

**Cons:**
- ⚠️ If RAGFlow is compromised, attacker has full PostgreSQL access
- ⚠️ Can access/modify other databases on same server

## Sandboxed Configuration (Restricted User)

### Step 1: Create Restricted User (One-Time Setup)

Connect to PostgreSQL as the `postgres` superuser and run the following commands **in an interactive psql session**:

```bash
psql -U postgres
```

Then execute these SQL commands:

```sql
-- Create dedicated user for RAGFlow
CREATE USER ragflow_user WITH PASSWORD 'your_secure_password';

-- Create database owned by restricted user
CREATE DATABASE ragflow_db OWNER ragflow_user;

-- Grant connection and usage permissions
GRANT CONNECT ON DATABASE ragflow_db TO ragflow_user;
GRANT ALL PRIVILEGES ON DATABASE ragflow_db TO ragflow_user;

-- Connect to ragflow_db database
\c ragflow_db

-- Grant schema permissions (for table creation)
GRANT ALL PRIVILEGES ON SCHEMA public TO ragflow_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO ragflow_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO ragflow_user;

-- Set default permissions for future objects
ALTER DEFAULT PRIVILEGES FOR ROLE ragflow_user IN SCHEMA public GRANT ALL ON TABLES TO ragflow_user;
ALTER DEFAULT PRIVILEGES FOR ROLE ragflow_user IN SCHEMA public GRANT ALL ON SEQUENCES TO ragflow_user;
```

**Alternative: Using SQL Script**

If you prefer to run these commands as a SQL script, split them into two parts since `\c` (connect) is a psql meta-command and won't work in a script:

```bash
# Part 1: Run as postgres user
psql -U postgres -d postgres << EOF
CREATE USER ragflow_user WITH PASSWORD 'your_secure_password';
CREATE DATABASE ragflow_db OWNER ragflow_user;
GRANT CONNECT ON DATABASE ragflow_db TO ragflow_user;
GRANT ALL PRIVILEGES ON DATABASE ragflow_db TO ragflow_user;
EOF

# Part 2: Run against ragflow_db (as postgres user, but set defaults FOR ROLE ragflow_user)
psql -U postgres -d ragflow_db << EOF
GRANT ALL PRIVILEGES ON SCHEMA public TO ragflow_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO ragflow_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO ragflow_user;
-- Explicitly specify FOR ROLE so defaults apply to objects created by ragflow_user
ALTER DEFAULT PRIVILEGES FOR ROLE ragflow_user IN SCHEMA public GRANT ALL ON TABLES TO ragflow_user;
ALTER DEFAULT PRIVILEGES FOR ROLE ragflow_user IN SCHEMA public GRANT ALL ON SEQUENCES TO ragflow_user;
EOF
```

### Step 2: Configure RAGFlow with Restricted User

Update your `.env` file:
```shell
POSTGRES_USER=ragflow_user
POSTGRES_PASSWORD=your_secure_password
POSTGRES_DBNAME=ragflow_db
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
```

Or update your `service_conf.yaml`:
```yaml
postgres:
  user: 'ragflow_user'       # Restricted user
  password: 'your_secure_password'
  name: 'ragflow_db'
  host: 'localhost'
  port: 5432
```

### Step 3: Verify Permissions

Test the restricted user can access only its database:

```bash
# Should succeed - ragflow_user can access ragflow_db
psql -h localhost -U ragflow_user -d ragflow_db -c "SELECT 1;"

# Should fail - verify restricted permissions by attempting privileged operation
psql -h localhost -U ragflow_user -d ragflow_db -c "CREATE DATABASE test_db;" 2>&1 | grep -i "permission denied"
```

**Note on postgres database access:** By default, PostgreSQL grants CONNECT on the `postgres` system database to PUBLIC, so verifying access denial requires testing specific operations. The above tests:
- ✅ Confirms `ragflow_user` can connect to and read from `ragflow_db`
- ✅ Confirms `ragflow_user` cannot execute administrative commands like CREATE DATABASE

## Security Benefits

**Sandboxed Setup:**
- ✅ Limited blast radius: Attacker can only access `ragflow_db`
- ✅ Cannot drop other databases or system tables
- ✅ Cannot create new users or modify permissions
- ✅ Suitable for shared PostgreSQL servers

**Comparison:**

| Aspect | Superuser (`postgres`) | Restricted User (`ragflow_user`) |
|--------|------------------------|----------------------------------|
| Setup complexity | Simple | Requires manual SQL setup |
| Database creation | Automatic | Manual pre-creation required |
| Security isolation | None | Sandboxed to own database |
| Multi-app server | ⚠️ Risk | ✅ Recommended |

## Troubleshooting

### "Permission denied to create database"

If you see this error, ensure:
1. Database is pre-created (Step 1 above)
2. User owns the database (`OWNER ragflow_user`)
3. RAGFlow is configured with correct username/password

### "Permission denied for schema public"

Run the schema permission grants from Step 1:
```sql
\c ragflow_db
GRANT ALL PRIVILEGES ON SCHEMA public TO ragflow_user;
```

### Database Already Exists

If `ragflow_db` already exists from previous setup:
```sql
-- Re-assign ownership to restricted user
ALTER DATABASE ragflow_db OWNER TO ragflow_user;
```

## Docker Compose Example

For Docker deployments with PostgreSQL container, the **recommended approach** is to use a shell wrapper script that explicitly targets the correct database for each set of commands:

```yaml
services:
  postgres:
    image: postgres:14
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: superuser_password
    volumes:
      - ./init-postgres.sh:/docker-entrypoint-initdb.d/init-postgres.sh
      - postgres_data:/var/lib/postgresql/data

  ragflow:
    environment:
      POSTGRES_HOST: postgres
      POSTGRES_USER: ragflow_user
      POSTGRES_PASSWORD: app_password
      POSTGRES_DBNAME: ragflow_db
```

**Important:** The default PostgreSQL Docker entrypoint runs `/docker-entrypoint-initdb.d/*.sql` scripts against the database specified by `POSTGRES_DB` (defaults to `postgres`). This means `02-init-permissions.sql` would run against `postgres`, not `ragflow_db`.

**Alternative: Two-Script Approach with `POSTGRES_DB`**

If you prefer separate SQL files, set `POSTGRES_DB: ragflow_db` and adjust accordingly:

```yaml
services:
  postgres:
    image: postgres:14
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: superuser_password
      POSTGRES_DB: ragflow_db  # Scripts run against this DB (auto-created)
    volumes:
      - ./01-init-user.sql:/docker-entrypoint-initdb.d/01-init-user.sql
      - ./02-init-permissions.sql:/docker-entrypoint-initdb.d/02-init-permissions.sql
      - postgres_data:/var/lib/postgresql/data
```

**Note:** With `POSTGRES_DB: ragflow_db`, the database is auto-created by PostgreSQL, so both init scripts run against `ragflow_db`.

**`01-init-user.sql`:** (runs against `ragflow_db` database)
```sql
-- Create dedicated user for RAGFlow
CREATE USER ragflow_user WITH PASSWORD 'app_password';

-- Transfer database ownership to ragflow_user
ALTER DATABASE ragflow_db OWNER TO ragflow_user;

-- Grant explicit database-level permissions (OWNER already has these, but explicit for clarity)
GRANT CONNECT ON DATABASE ragflow_db TO ragflow_user;
GRANT ALL PRIVILEGES ON DATABASE ragflow_db TO ragflow_user;
```

**`02-init-permissions.sql`:** (runs against `ragflow_db` - set by `POSTGRES_DB`)
```sql
-- This script runs against ragflow_db (set by POSTGRES_DB) to set schema/table permissions
-- Grant schema permissions (required for table creation and access)
GRANT ALL PRIVILEGES ON SCHEMA public TO ragflow_user;

-- Grant permissions on existing tables (if any)
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO ragflow_user;

-- Grant permissions on existing sequences (if any)
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO ragflow_user;

-- Set default privileges for objects created in the future
-- This ensures ragflow_user has full access to tables/sequences created by ragflow_user in the future
ALTER DEFAULT PRIVILEGES FOR ROLE ragflow_user IN SCHEMA public GRANT ALL PRIVILEGES ON TABLES TO ragflow_user;
ALTER DEFAULT PRIVILEGES FOR ROLE ragflow_user IN SCHEMA public GRANT ALL PRIVILEGES ON SEQUENCES TO ragflow_user;
```

**Alternative: Shell Wrapper Approach (`init-postgres.sh`):**
```bash
#!/bin/bash
set -e

# Run against default postgres database to create user and database
psql -U postgres -d postgres -v ON_ERROR_STOP=1 <<-EOSQL
  CREATE USER ragflow_user WITH PASSWORD 'app_password';
  CREATE DATABASE ragflow_db OWNER ragflow_user;
  GRANT CONNECT ON DATABASE ragflow_db TO ragflow_user;
  GRANT ALL PRIVILEGES ON DATABASE ragflow_db TO ragflow_user;
EOSQL

# Run against ragflow_db to set schema/table/sequence permissions
psql -U postgres -d ragflow_db -v ON_ERROR_STOP=1 <<-EOSQL
  GRANT ALL PRIVILEGES ON SCHEMA public TO ragflow_user;
  GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO ragflow_user;
  GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO ragflow_user;
  ALTER DEFAULT PRIVILEGES FOR ROLE ragflow_user IN SCHEMA public GRANT ALL PRIVILEGES ON TABLES TO ragflow_user;
  ALTER DEFAULT PRIVILEGES FOR ROLE ragflow_user IN SCHEMA public GRANT ALL PRIVILEGES ON SEQUENCES TO ragflow_user;
EOSQL
```

Then mount in Docker Compose:
```yaml
volumes:
  - ./init-postgres.sh:/docker-entrypoint-initdb.d/init-postgres.sh
  - postgres_data:/var/lib/postgresql/data
```

## Recommendation

- **Development/Testing**: Use superuser (`postgres`) for simplicity
- **Production (single-app)**: Superuser acceptable if PostgreSQL is dedicated to RAGFlow
- **Production (multi-app)**: **Always use restricted user** to prevent cross-contamination
- **Shared servers**: **Always use restricted user** for security isolation

## References

- [PostgreSQL User Management](https://www.postgresql.org/docs/current/user-manag.html)
- [PostgreSQL Privileges](https://www.postgresql.org/docs/current/ddl-priv.html)
- [RAGFlow Configuration Guide](../README.md)
