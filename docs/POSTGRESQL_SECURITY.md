# PostgreSQL Security: Sandboxed User Setup

## Overview

By default, RAGFlow uses PostgreSQL superuser credentials (`postgres`) for database operations, mirroring the MySQL approach which uses `root`. This simplifies deployment but grants the application full access to the PostgreSQL server.

For production environments or shared database servers, you may want to create a **restricted user** with limited permissions to contain potential security breaches.

## Default Configuration (Superuser)

**Docker `.env` or `service_conf.yaml`:**
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
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO ragflow_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO ragflow_user;
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

# Part 2: Run against ragflow_db
psql -U postgres -d ragflow_db << EOF
GRANT ALL PRIVILEGES ON SCHEMA public TO ragflow_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO ragflow_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO ragflow_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO ragflow_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO ragflow_user;
EOF
```

### Step 2: Configure RAGFlow with Restricted User

Update your `.env` or `service_conf.yaml`:

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
# Should succeed
psql -h localhost -U ragflow_user -d ragflow_db -c "SELECT 1;"

# Should fail (no access to other databases)
psql -h localhost -U ragflow_user -d postgres -c "SELECT 1;"
```

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

For Docker deployments with PostgreSQL container:

```yaml
services:
  postgres:
    image: postgres:14
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: superuser_password
      # Pre-create database with init script
    volumes:
      - ./init-postgres.sql:/docker-entrypoint-initdb.d/init.sql
      - postgres_data:/var/lib/postgresql/data

  ragflow:
    environment:
      POSTGRES_HOST: postgres
      POSTGRES_USER: ragflow_user
      POSTGRES_PASSWORD: app_password
      POSTGRES_DBNAME: ragflow_db
```

**`init-postgres.sql`:**
```sql
-- Create dedicated user for RAGFlow
CREATE USER ragflow_user WITH PASSWORD 'app_password';

-- Create database owned by ragflow_user
CREATE DATABASE ragflow_db OWNER ragflow_user;

-- Grant database-level permissions
GRANT CONNECT ON DATABASE ragflow_db TO ragflow_user;
GRANT ALL PRIVILEGES ON DATABASE ragflow_db TO ragflow_user;

-- Grant schema permissions (required for table creation and access)
\c ragflow_db
GRANT USAGE ON SCHEMA public TO ragflow_user;
GRANT CREATE ON SCHEMA public TO ragflow_user;

-- Grant permissions on existing tables (if any)
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO ragflow_user;

-- Grant permissions on existing sequences (if any)
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO ragflow_user;

-- Set default privileges for objects created by ragflow_user (the database owner)
-- This ensures RAGFlow can access tables/sequences it creates
ALTER DEFAULT PRIVILEGES FOR USER ragflow_user IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ragflow_user;
ALTER DEFAULT PRIVILEGES FOR USER ragflow_user IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO ragflow_user;
ALTER DEFAULT PRIVILEGES FOR USER ragflow_user IN SCHEMA public GRANT EXECUTE ON FUNCTIONS TO ragflow_user;
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
