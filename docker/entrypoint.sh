#!/usr/bin/env bash

set -e

echo "Start RAGFlow, version: "
cat /ragflow/VERSION

# -----------------------------------------------------------------------------
# Usage and command-line argument parsing
# -----------------------------------------------------------------------------
function usage() {
    echo "Usage: $0 [OPTIONS]"
    echo
    echo "  --disable-api-server                     Disables the web server (nginx + ragflow_server)."
    echo "  --disable-ingestor                       Disables ingestor."
    echo "  --disable-syncer                         Disables data source syncer."
    echo "  --chatter                                Runs chat channels in a separate process alongside enabled services."
    echo "  --disable-chat-channels                  Disables the chat channels service completely."
    echo "  --skip-migrations                        Starts the services without running database migrations first."
    echo "  --init-model-provider-tables             Runs all database migrations and exits."
    echo "  --init-superuser                         Initializes the superuser."
    echo "  --ingestors=<num>                        Number of ingestors to run."

    echo
    echo "Examples:"
    echo "  $0 --disable-ingestor"
    echo "  $0 --disable-api-server --ingestors=2"
    echo "  $0 --init-superuser"
    echo "  $0 --init-model-provider-tables"
    echo "  $0 --skip-migrations --disable-ingestor --disable-syncer --disable-chat-channels"
    exit 1
}

ENABLE_ADMIN_SERVER=0 # Default close admin server
ENABLE_API_SERVER=1 # Default to enable web server
ENABLE_INGESTOR=1  # Default to enable ingestor
ENABLE_DATASYNC=1
ENABLE_CHATTER=0
DISABLE_CHAT_CHANNELS=0
SKIP_MIGRATIONS=0

INIT_SUPERUSER_ARGS="" # Default to not initialize superuser
INIT_MODEL_PROVIDER_TABLES=0
INGESTOR_NUMBER=1

# Parse arguments
for arg in "$@"; do
  case $arg in
    --disable-api-server)
      ENABLE_API_SERVER=0
      shift
      ;;
    --disable-ingestor)
      ENABLE_INGESTOR=0
      shift
      ;;
    --disable-syncer)
      ENABLE_DATASYNC=0
      shift
      ;;
    --chatter)
      ENABLE_CHATTER=1
      shift
      ;;
    --disable-chat-channels)
      DISABLE_CHAT_CHANNELS=1
      shift
      ;;
    --skip-migrations)
      SKIP_MIGRATIONS=1
      shift
      ;;
    --enable-adminserver)
      ENABLE_ADMIN_SERVER=1
      shift
      ;;
    --init-model-provider-tables)
      INIT_MODEL_PROVIDER_TABLES=1
      shift
      ;;
    --init-superuser)
      INIT_SUPERUSER_ARGS="--init-superuser"
      shift
      ;;
    --ingestors=*)
      INGESTOR_NUMBER="${arg#*=}"
      shift
      ;;
    *)
      usage
      ;;
  esac
done

if [[ "${INIT_MODEL_PROVIDER_TABLES}" -eq 1 && "${SKIP_MIGRATIONS}" -eq 1 ]]; then
    echo "--init-model-provider-tables and --skip-migrations cannot be combined."
    usage
fi

# -----------------------------------------------------------------------------
# Replace env variables in the service_conf.yaml file
# -----------------------------------------------------------------------------
CONF_DIR="/ragflow/conf"
TEMPLATE_FILE="${CONF_DIR}/service_conf.yaml.template"
CONF_FILE="${CONF_DIR}/service_conf.yaml"

rm -f "${CONF_FILE}"
DEF_ENV_VALUE_PATTERN="\$\{([^:]+):-([^}]+)\}"
while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" =~ DEF_ENV_VALUE_PATTERN ]]; then
        varname="${BASH_REMATCH[1]}"
        default="${BASH_REMATCH[2]}"

        if [ -n "${!varname}" ]; then
            eval "echo \"$line"\" >> "${CONF_FILE}"
        else
            echo "$line" | sed -E "s/\\\$\{[^:]+:-([^}]+)\}/\1/g" >> "${CONF_FILE}"
        fi
    else
        eval "echo \"$line\"" >> "${CONF_FILE}"
    fi
done < "${TEMPLATE_FILE}"

export LD_LIBRARY_PATH="/usr/lib/x86_64-linux-gnu/"

# -----------------------------------------------------------------------------
# Function(s)
# -----------------------------------------------------------------------------

# One-shot Go migration. bin/ragflow_server --migrate is a standalone action: it
# runs the migrations and exits, independent of any server mode.
function run_go_migrations() {
    local db_type="${DB_TYPE:-mysql}"
    db_type="${db_type,,}"
    if [[ "$db_type" == "gaussdb" || "$db_type" == "gauss" ]]; then
        # The Go migrations emit MySQL-only SQL and cannot run against a GaussDB
        # metadata database.
        echo "Skipping MySQL-specific database migrations for DB_TYPE=${DB_TYPE:-mysql}."
        return 0
    fi
    echo "Running database migrations..."
    bin/ragflow_server --migrate
}

# Whether a Go server mode other than the ingestor will run. The standalone
# migration runs before them; an ingestor-only container relies on another
# container to migrate.
function go_backend_enabled() {
    if [[ "${ENABLE_DATASYNC}" -eq 1 || "${ENABLE_CHATTER}" -eq 1 ]]; then
        return 0
    fi
    if [[ "${ENABLE_ADMIN_SERVER}" -eq 1 ]] || [[ "${ENABLE_API_SERVER}" -eq 1 ]]; then
        return 0
    fi
    return 1
}

# -----------------------------------------------------------------------------
# Start components based on flags
# -----------------------------------------------------------------------------
run_with_restart() {
  local process_name="$1"
  shift

  while true; do
    echo "Attempt to start ${process_name}..."
    set +e
    "$@"
    local exit_code=$?
    set -e
    echo "${process_name} exited with code ${exit_code}. Restarting in 1 second..."
    sleep 1
  done
}

# --init-model-provider-tables runs all migrations and exits without booting any
# server, e.g. as a Kubernetes init container or Job.
if [[ "${INIT_MODEL_PROVIDER_TABLES}" -eq 1 ]]; then
    run_go_migrations
    echo "Database migrations finished. Exiting."
    exit 0
fi

# Otherwise migrate once up front, before any Go server mode boots, unless the
# migrations run elsewhere (--skip-migrations).
if [[ "${SKIP_MIGRATIONS}" -eq 0 ]] && go_backend_enabled; then
    run_go_migrations
fi

if [[ "${ENABLE_ADMIN_SERVER}" -eq 1 ]]; then
    echo "Starting Admin go server..."
    run_with_restart "Admin go server" bin/ragflow_server --admin ${INIT_SUPERUSER_ARGS} &
fi

if [[ "${ENABLE_API_SERVER}" -eq 1 ]]; then
    echo "Starting nginx..."
    /usr/sbin/nginx -c /etc/nginx/nginx.conf

    echo "Starting RAGFlow go server..."
    API_ARGS=()
    if [[ "${ENABLE_CHATTER}" -eq 1 || "${DISABLE_CHAT_CHANNELS}" -eq 1 ]]; then
        API_ARGS+=(--disable-chat-channels)
    fi
    run_with_restart "RAGFlow go server" bin/ragflow_server --api "${API_ARGS[@]}" &
fi

if [[ "${ENABLE_CHATTER}" -eq 1 ]]; then
    echo "Starting chat channels..."
    run_with_restart "Chat channel runtime" bin/ragflow_server --chatter &
fi

if [[ "${ENABLE_DATASYNC}" -eq 1 ]]; then
    echo "Starting data sync..."
    run_with_restart "RAGFlow go server" bin/ragflow_server --syncer &
fi

if [[ "${ENABLE_INGESTOR}" -eq 1 ]]; then
    echo "Starting ${INGESTOR_NUMBER} ingestor worker(s)..."
    for (( i=0; i<INGESTOR_NUMBER; i++ ))
    do
        run_with_restart "ingestor" bin/ragflow_server --ingestor &
    done
fi

wait
