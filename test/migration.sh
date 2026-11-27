#!/usr/bin/env bash
# Run: bash test/migration.sh
set -euo pipefail

repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
tmp_dir=$(mktemp -d)
trap 'rm -rf -- "$tmp_dir"' EXIT
cd "$tmp_dir"
compose_dir="$(pwd -P)/docker"
mkdir -p bin "$compose_dir/oceanbase/data" "$compose_dir/oceanbase/conf" "$compose_dir/seekdb"
cp "$repo_dir/docker/"{migration.sh,docker-compose-base.yml} "$compose_dir/"
export PATH="$PWD/bin:$PATH"
export archive_calls="$PWD/archive-calls"

# Compose reports the active project's storage. Record each Docker tar operation
# at the CLI boundary so the assertions can compare mount and archive pairs.
cat > bin/docker <<'STUB'
#!/usr/bin/env bash
set -eu
case "$1" in
    info|ps) exit 0 ;;
    compose)
        case "${*: -2}" in
            'config --profiles') cat >/dev/null; echo "$engine" ;;
            'config --volumes')  echo "$active_volumes" ;;
            'config --services') echo "$engine" ;;
            *) echo "Unexpected Compose arguments: $*" >&2; exit 1 ;;
        esac ;;
    volume) [[ $2 == inspect ]] ;;
    run)
        shift # run
        [[ $1 != --rm ]] || shift
        while [[ $1 == -v ]]; do
            case "${2#*:}" in
                /source:ro|/target) source=${2%%:*} ;;
                /backup|/backup:ro) backup=${2%%:*} ;;
                *) echo "Unexpected mount: $2" >&2; exit 1 ;;
            esac
            shift 2
        done
        archive=${4#/backup/}
        printf '%s %s %s\n' "$3" "$source" "$archive" >> "$archive_calls"
        case "$2 $3" in
            'tar czf') touch "$backup/$archive" ;;
            'tar xzf') test -f "$backup/$archive" ;;
            *) echo "Unexpected container command: $*" >&2; exit 1 ;;
        esac ;;
    *) echo "Unexpected Docker arguments: $*" >&2; exit 1 ;;
esac
STUB
chmod +x bin/docker

migrate() {
    : > "$archive_calls"
    bash "$compose_dir/migration.sh" -p fixture "$@" <<< y > migration.log 2>&1
}

assert_calls() {
    local mode=$1 pair
    shift
    for pair in "$@"; do
        printf '%s %s\n' "$mode" "$pair"
    done | sort > expected-calls
    diff -u expected-calls <(sort "$archive_calls")
}

common_volumes=$'mysql_data\nminio_data\nkvrocks_data\nnats_data'
common_archives=(
    'fixture_mysql_data mysql_backup.tar.gz'
    'fixture_minio_data minio_backup.tar.gz'
    'fixture_kvrocks_data kvrocks_data.tar.gz'
    'fixture_nats_data nats_data.tar.gz'
)
check_engine() {
    export engine=$1 active_volumes="$common_volumes"
    if [[ -n $2 ]]; then
        active_volumes+=$'\n'"$2"
    fi
    shift 2
    local backup="$PWD/$engine"
    migrate backup "$backup" || { cat migration.log >&2; return 1; }
    assert_calls czf "${common_archives[@]}" "$@"
    diff -u <(echo "$engine") "$backup/doc_engine"
    migrate restore "$backup" || { cat migration.log >&2; return 1; }
    assert_calls xzf "${common_archives[@]}" "$@"
    echo "PASS: $engine backup and restore"
}

check_engine elasticsearch esdata01 'fixture_esdata01 es_backup.tar.gz'
check_engine infinity infinity_data 'fixture_infinity_data infinity_data.tar.gz'
check_engine opensearch osdata39 'fixture_osdata39 osdata39.tar.gz'
check_engine serenedb serenedb_data 'fixture_serenedb_data serenedb_data.tar.gz'
check_engine vastbase vastbase_data 'fixture_vastbase_data vastbase_data.tar.gz'
check_engine oceanbase '' "$compose_dir/oceanbase/data oceanbase-data.tar.gz" \
    "$compose_dir/oceanbase/conf oceanbase-conf.tar.gz"
check_engine seekdb '' "$compose_dir/seekdb seekdb.tar.gz"

# The current Compose profile also has Kvrocks and NATS. A markerless backup
# must still restore exactly the four historical archives, including Redis.
engine=elasticsearch
active_volumes="$common_volumes"$'\nesdata01'
mkdir legacy
touch legacy/{mysql,minio,redis,es}_backup.tar.gz
migrate restore legacy || { cat migration.log >&2; exit 1; }
assert_calls xzf 'fixture_mysql_data mysql_backup.tar.gz' 'fixture_minio_data minio_backup.tar.gz' \
    'fixture_redis_data redis_backup.tar.gz' 'fixture_esdata01 es_backup.tar.gz'
echo 'PASS: legacy Elasticsearch restore'

assert_rejected() {
    local folder=$1 message=$2
    if migrate restore "$folder"; then
        echo "Unexpectedly restored $folder with engine $engine" >&2
        exit 1
    fi
    grep -Fq -- "$message" migration.log || { cat migration.log >&2; exit 1; }
    diff -u /dev/null "$archive_calls"
    echo "PASS: rejected restore ($message)"
}

# An interrupted overwrite leaves the old completion marker and archives.
echo elasticsearch > elasticsearch/.doc_engine.pending
assert_rejected elasticsearch 'Backup is incomplete'
assert_rejected infinity 'Backup folder uses DOC_ENGINE=infinity, but the deployment uses elasticsearch'
