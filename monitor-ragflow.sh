#!/usr/bin/env bash
#
# Мониторинг деградации контейнера RAGFlow.
# Каждый запуск пишет одну строку в лог: время, число процессов/потоков,
# результат резолва DNS (es01/mysql/minio), соединения к MySQL/ES и память.
#
# Использование (запускать каждые 5-10 минут, например через cron):
#   */5 * * * * /home/apps/ragflow-ecosystem/ragflow/monitor-ragflow.sh
#
# Лог: /var/log/ragflow-monitor.log (ротация через logrotate при желании).
#
set -u

CONTAINER="${RAGFLOW_CONTAINER:-docker-ragflow-cpu-1}"
LOG="${RAGFLOW_MONITOR_LOG:-/var/log/ragflow-monitor.log}"
HOSTS="es01 mysql minio redis"

now() { date '+%Y-%m-%d %H:%M:%S'; }

# Контейнер может быть временно недоступен (перезапуск).
docker inspect "$CONTAINER" >/dev/null 2>&1 || { echo "$(now) container $CONTAINER not running" >> "$LOG"; exit 0; }

pids=$(docker exec "$CONTAINER" sh -c 'ps -e | wc -l' 2>/dev/null)
threads=$(docker exec "$CONTAINER" sh -c 'ps -eL | wc -l' 2>/dev/null)
python_procs=$(docker exec "$CONTAINER" sh -c 'ps -e | grep -c python3' 2>/dev/null)
mem=$(docker stats --no-stream --format '{{.MemUsage}} {{.PIDs}}' "$CONTAINER" 2>/dev/null)

# Резолв DNS: для каждого имени — ok/fail
dns=""
for h in $HOSTS; do
  if docker exec "$CONTAINER" getent hosts "$h" >/dev/null 2>&1; then
    dns="$dns $h=ok"
  else
    dns="$dns $h=FAIL"
  fi
done

# Число соединений со стороны RAGFlow к MySQL и ES изнутри контейнера
mysql_conn=$(docker exec "$CONTAINER" sh -c 'ss -tn 2>/dev/null | grep -c ":3306"' 2>/dev/null)
es_conn=$(docker exec "$CONTAINER" sh -c 'ss -tn 2>/dev/null | grep -c ":9200"' 2>/dev/null)

echo "$(now) pids=$pids threads=$threads python=$python_procs mysql_conn=${mysql_conn:-0} es_conn=${es_conn:-0} mem=${mem:-n/a} dns:$dns" >> "$LOG"