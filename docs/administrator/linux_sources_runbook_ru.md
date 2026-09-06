---
sidebar_position: 5
slug: /linux-sources-runbook-ru
---

# Установка и обновление RAGFlow с PostgreSQL из архива

Этот runbook описывает установку текущего кастомного RAGFlow с нуля на Linux.
Источник поставки — проверенный `tar.gz` с отдельной SHA-256 суммой. Архив
передаётся через `pscp.exe`; Git на Linux-сервере не требуется.

Основная целевая платформа — Rocky Linux 9.8 x86_64 с корпоративным
DNF-репозиторием `cifra-docker`. Интернет-инсталлятор также поддерживает Ubuntu
24.04 и Debian 12, если доступная версия Docker Compose понимает теги
`!override` и `!reset`. Offline-пакет переносит frontend и Docker-образы без
обращения сервера к Docker Hub; системные RPM берутся из корпоративного DNF.

## 1. Что будет установлено

Один Compose-проект `ragflow-pg` содержит:

| Сервис | Назначение | Публикация на host |
| --- | --- | --- |
| `ragflow-cpu` | UI, API и текущий кастомный backend | `0.0.0.0:9380` |
| `postgres` | метаданные RAGFlow | не публикуется |
| `es01` | документы, полнотекстовый и векторный индекс | не публикуется |
| `redis` | Valkey, очередь и кэш | не публикуется |
| `minio` | внутреннее объектное хранилище | не публикуется |
| `plantuml-server` | рендеринг диаграмм | не публикуется |

MySQL отсутствует в итоговом Compose. PostgreSQL хранит метаданные приложения,
а Elasticsearch — документы и векторы, поэтому `pgvector` этому профилю не
нужен.

Первый пользователь создаётся через штатный `init_superuser()` с
`is_superuser=true` и становится владельцем своего tenant.

Регистрация новых пользователей по паролю выключена:
`REGISTER_ENABLED=0`.

## 2. Как гарантируется установка актуального кода

Архив содержит полное проверенное дерево исходников. Frontend собирается на
целевом сервере из этого же распакованного дерева в контейнере
`node:20-bookworm-slim` либо включается в offline/registry-пакет готовым.

Runtime RAGFlow состоит из закреплённого upstream-образа и локального overlay:

1. `docker/docker-compose.yml`;
2. `docker/docker-compose.local.yml`;
3. `docker/docker-compose.linux.local.yml`;
4. `deployment/linux-pg/docker-compose.release.yml`.

Второй файл монтирует текущие backend-модули и собранный `web/dist`, третий
отключает Docker Desktop bootstrap на Linux, четвёртый убирает MySQL и
ограничивает публикацию RAGFlow loopback-адресом. Запуск только первого
Compose-файла означает потерю PostgreSQL-профиля и кастомного кода.

Установщик принимает дерево с `DEPLOYMENT-SOURCE.env`, проверяет формат пакета,
копирует manifest в `/etc/ragflow-pg/deployed-source.env` и отказывается
устанавливать поверх уже существующего `/opt/ragflow-pg`.

## 3. Что ещё нужно решить до production

В базовую поставку не входят LLM, embedding-модели и ASR. Без chat-модели
RAGFlow не отвечает, а без embedding-модели не может полноценно индексировать и
искать знания.

До production-приёмки зафиксировать:

1. Chat/Vision LLM, embedding и, при необходимости, reranker: локальный Ollama,
   Ollama Proxy или внешний API, точные имена моделей и ключи.
2. CodeExec sandbox входит в release-профиль; поставка содержит зафиксированный
   gVisor/runsc, а приёмка требует заполненный Python/Node.js pool и реальный запуск кода.
3. DNS, reverse proxy, TLS и список административных IP.
4. Backup и проверяемое восстановление PostgreSQL, MinIO, конфигурации и
   секретов; политика Elasticsearch snapshots или полного переиндексирования.
5. Мониторинг диска, RAM/OOM, restart count, health, очередей и срока TLS.
6. Политика пользователей: локальные учётные записи или SSO/OIDC, SMTP,
   второй аварийный суперадмин и порядок отзыва доступа.
7. Производственная ёмкость, GPU/CPU-профиль для моделей, место под
   индексы и объекты, нагрузочный тест.
8. Регламент обновления и отката. `install.sh` предназначен только для первой
   установки и не является upgrade-скриптом.
9. Какие внешние источники и connectors действительно нужны, их ACL и
   отдельные read-only credentials.

Без пунктов 1, 3, 4 и 5 контур можно считать установленным, но не готовым к
production.

## 4. Требования

### 4.1. Машина подготовки файла поставки

Нужны:

- текущее рабочее дерево `S:\ragflow`;
- PowerShell 7 и `tar`;
- достаточно места для временной контрольной распаковки;
- PuTTY/PSCP для передачи архива и checksum.

На Linux Git не устанавливается и не вызывается. Комплекта PuTTY с `pscp.exe`
достаточно для передачи; на этой машине он находится в `C:\Program Files\PuTTY`.

### 4.2. Linux-сервер

Базовый профиль:

- Rocky Linux 9.x, Ubuntu 24.04 или Debian 12, `x86_64`, `systemd`;
- доступ `sudo`;
- 16 GB RAM для smoke; текущий Rocky-контур с 6 CPU допускается к установке;
- ориентир 16 CPU, 32 GB RAM и 300+ GB SSD для рабочего контура;
- исходящий HTTPS к DNF/APT и registry образов;
- свободный host-порт `9380` или другой выбранный порт.

Итоговые требования определяются объёмом данных и моделями. До загрузки
production-данных подтвердить их нагрузочным тестом и ростом индекса.
Установщик не блокирует запуск по числу CPU или размеру диска.

Проверка сервера:

```bash
uname -m
cat /etc/os-release
lscpu
free -h
df -hT
timedatectl status
sudo ss -ltnp | grep ':80 ' || true
```

Ожидаются `x86_64`, поддерживаемая ОС и свободный порт.

## 5. Собрать архив поставки

### 5.1. Рекомендуемый вариант без Docker Hub на сервере

Для Rocky Linux 9.x x86_64 можно собрать один архив, включающий готовый frontend,
15 Docker-образов и зафиксированный комплект gVisor/runsc:

```powershell
./deployment/linux-pg/build_offline_archive.ps1 -ReleaseVersion v1.12.0
```

Передать `ragflow-linux-pg-v1.12.0-offline.tar.gz` и соседний `.sha256` через
`pscp.exe`. На сервере:

```bash
cd /tmp
sha256sum -c ragflow-linux-pg-v1.12.0-offline.tar.gz.sha256
package_dir=$(mktemp -d /tmp/ragflow-offline-v1.12.0.XXXXXX)
sudo tar --no-same-owner --no-same-permissions \
  -xzf ragflow-linux-pg-v1.12.0-offline.tar.gz -C "$package_dir"
cd "$package_dir"
sudo env ADMIN_EMAIL=admin@example.org \
  ADMIN_NICKNAME='RAGFlow Administrator' \
  bash install_offline.sh
```

Сценарий проверяет внутренний `SHA256SUMS`, устанавливает Docker и утилиты через
корпоративный `cifra-docker`, выполняет `docker load` и запускает Compose с
`--pull never --no-build`. Docker Hub на сервере не используется. Chat LLM,
embedding-модели и ASR поставляются отдельно после выбора провайдера.

### 5.2. Компактный вариант с интернетом на сервере

На Windows открыть PowerShell в `S:\ragflow` и выполнить:

```powershell
./deployment/linux-pg/build_archive.ps1 -ReleaseVersion v1.12.0
```

Скрипт создаёт `ragflow-linux-pg-v1.12.0.tar.gz` и
`ragflow-linux-pg-v1.12.0.tar.gz.sha256`, выполняет контрольную распаковку и
проверяет обязательные файлы. В архив не входят `.git`, локальные `.env`, кэши,
данные контейнеров, `node_modules`, `web/dist`, локальные build/test-артефакты,
скачанные `ragflow_deps`, локальные model caches, `output`, старые артефакты и
каталоги предыдущих release-сборок. Frontend будет собран на Linux из исходников
поставки. Скрипт Git не вызывает: он упаковывает текущее состояние файлов,
поэтому перед сборкой нужно завершить проверку всех изменений поставки.

Для обновления существующего сервера compact source-архив обязан содержать
проверенный frontend из этой же рабочей копии:

```powershell
corepack pnpm --dir web install --frozen-lockfile --ignore-scripts
corepack pnpm --dir web run build
./deployment/linux-pg/build_archive.ps1 `
  -ReleaseVersion v1.12.0 `
  -UseExistingFrontend
```

Builder проверит `web/dist/index.html`, включит `web/dist` и запишет
`FRONTEND_MODE=prebuilt` в manifest. Обычный вызов без ключа оставляет frontend
за пределами архива (`FRONTEND_MODE=excluded`) и предназначен для первой
source-установки с доступом к build-зависимостям.

`DEPLOYMENT-SOURCE.env` содержит `RELEASE_VERSION`, `PACKAGE_FORMAT`, режим
сборки и время упаковки. Источником доверия служат утверждённая SHA-256 сумма и
этот manifest.

## 6. Передать архив через PuTTY

Передать оба файла программой `pscp.exe`; пользователя и адрес заменить:

```powershell
& 'C:\Program Files\PuTTY\pscp.exe' -P 22 `
  'S:\ragflow\deployment\linux-pg\ragflow-linux-pg-v1.12.0.tar.gz' `
  'admin@ragflow-server:/tmp/'

& 'C:\Program Files\PuTTY\pscp.exe' -P 22 `
  'S:\ragflow\deployment\linux-pg\ragflow-linux-pg-v1.12.0.tar.gz.sha256' `
  'admin@ragflow-server:/tmp/'
```

Пароль не указывать в командной строке: `pscp.exe` запросит его сам. Для PPK-ключа
добавить `-i 'C:\path\to\key.ppk'`.

## 7. Проверить и распаковать на Linux

В сеансе PuTTY:

```bash
cd /tmp
sha256sum -c ragflow-linux-pg-v1.12.0.tar.gz.sha256

sudo install -d -m 0755 /srv/ragflow-linux-pg
sudo tar -xzf ragflow-linux-pg-v1.12.0.tar.gz -C /srv/ragflow-linux-pg
cd /srv/ragflow-linux-pg

cat DEPLOYMENT-SOURCE.env
test -s deployment/linux-pg/install.sh
test -s deployment/linux-pg/docker-compose.release.yml
test -s deployment/linux-pg/seed_admin.py
test -s deployment/linux-pg/seed_asr.py
test -s services/asr-online-service/Dockerfile
```

`sha256sum` должен вывести `OK`, manifest — ожидаемую версию и
`PACKAGE_FORMAT=tar.gz`. На сервере не нужны `git clone`, `git checkout` или
установка Git.

## 8. Запустить установку

Рекомендуется не передавать пароль в командной строке: если
`ADMIN_PASSWORD` отсутствует, installer создаст случайный 64-символьный hex
пароль и сохранит его в root-only файле.

```bash
cd /srv/ragflow-linux-pg

sudo env \
  ADMIN_EMAIL=admin@example.org \
  ADMIN_NICKNAME='RAGFlow Administrator' \
  INSTALL_DIR=/opt/ragflow-pg \
  PROJECT_NAME=ragflow-pg \
  RAGFLOW_PORT=9380 \
  DOCKER_DNF_REPO=cifra-docker \
  bash deployment/linux-pg/install.sh
```

Установщик:

1. Проверяет ОС, архитектуру, manifest архива и новый `INSTALL_DIR`.
2. На Ubuntu/Debian устанавливает Docker Engine/Compose через APT; на Rocky
   Linux 9.x — Docker CE, containerd, Buildx и Compose plugin через включённый
   `cifra-docker` (или repo id из `DOCKER_DNF_REPO`).
3. Требует Docker >= 24 и Compose >= 2.26.1, включает Docker и сохраняет
   `vm.max_map_count=262144` и `net.ipv4.ip_forward=1`.
4. Копирует source tree в `/opt/ragflow-pg`, не копируя `.git`, кэши и данные.
5. Собирает frontend командой `npm ci && npm run build` в одноразовом
   `node:20-bookworm-slim`; `node_modules` остаётся во временном Docker volume,
   затем volume удаляется.
6. Создаёт `docker/.env` с независимыми случайными паролями PostgreSQL,
   Elasticsearch, MinIO и Valkey, а также постоянным ключом шифрования
   персональных EVA-токенов.
7. Валидирует объединённый Compose, требует `postgres`, `t-one-asr`, sandbox и
   observability-сервисы и аварийно завершает работу, если активен `mysql`.
8. Запускает все сервисы и ждёт полного RAGFlow health до 10 минут.
9. Создаёт superuser и проверяет его через фактические сервисы RAGFlow.

Получить сгенерированные credentials:

```bash
sudo stat -c '%U %G %a %n' /etc/ragflow-pg/admin.env
sudo cat /etc/ragflow-pg/admin.env
sudo cat /etc/ragflow-pg/deployed-source.env
```

Ожидаемый режим `admin.env` — `root root 600`. После помещения пароля в
утверждённый secret manager не создавать его копии с более широкими правами.

Если пароль должен быть задан заранее, считать его интерактивно:

```bash
read -rsp 'Initial admin password: ' ADMIN_PASSWORD
echo
sudo env \
  ADMIN_EMAIL=admin@example.org \
  ADMIN_PASSWORD="$ADMIN_PASSWORD" \
  INSTALL_DIR=/opt/ragflow-pg \
  PROJECT_NAME=ragflow-pg \
  RAGFLOW_PORT=9380 \
  bash deployment/linux-pg/install.sh
unset ADMIN_PASSWORD
```

## 9. Единая команда Compose

Для ручных операций использовать те же пять слоёв:

```bash
cd /opt/ragflow-pg/docker

rf_compose() {
  sudo docker compose \
    --env-file .env \
    -p ragflow-pg \
    -f docker-compose.yml \
    -f docker-compose.local.yml \
    -f docker-compose.linux.local.yml \
    -f docker-compose.observability.yml \
    -f ../deployment/linux-pg/docker-compose.release.yml \
    "$@"
}
```

Не заменять эту команду на `docker compose up` без overlays.

## 10. Приёмка инфраструктуры

### 10.1. Source и frontend

```bash
sudo cat /etc/ragflow-pg/deployed-source.env
test -s /opt/ragflow-pg/web/dist/index.html
grep -n '^RAGFLOW_IMAGE=' /opt/ragflow-pg/docker/.env
sudo grep -Eq '^RAGFLOW_CREDENTIALS_KEY=[0-9a-f]{64}$' \
  /opt/ragflow-pg/docker/.env && echo 'credential key: ok'
```

`RELEASE_VERSION` и `PACKAGE_FORMAT=tar.gz` должны совпадать с утверждённой
поставкой, а проверка внешнего `.sha256` перед распаковкой должна завершиться
со статусом `OK`. `RAGFLOW_IMAGE` — закреплённая upstream-основа; версию
кастомного релиза определяют архив, manifest и локальные монтирования.

### 10.2. Сервисы и отсутствие MySQL

```bash
rf_compose config --quiet
rf_compose config --services | sort
rf_compose ps

if rf_compose config --services | grep -qx mysql; then
  echo 'ERROR: MySQL is active' >&2
  exit 1
else
  echo 'OK: MySQL is absent'
fi

sudo docker ps -a \
  --filter label=com.docker.compose.project=ragflow-pg \
  --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}'
```

Ожидаются 13 постоянных сервисов: `es01`, `minio`, `plantuml-server`, `postgres`,
`ragflow-cpu`, `redis`, `t-one-asr`, `sandbox-executor-manager`, `otel-collector`,
`tempo`, `loki`, `prometheus`, `grafana`, а также шесть динамических sandbox
контейнеров пула. MySQL быть не должно.

### 10.3. Health RAGFlow и PostgreSQL

```bash
curl -fsS http://127.0.0.1:9380/api/v1/system/healthz | jq
curl -fsS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:9380/

ragflow_id=$(rf_compose ps -q ragflow-cpu)
postgres_id=$(rf_compose ps -q postgres)
test -n "$ragflow_id"
test -n "$postgres_id"

sudo docker exec "$ragflow_id" printenv DB_TYPE
sudo docker exec "$postgres_id" sh -lc \
  'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc \
  "select current_database(), count(*) from information_schema.tables where table_schema='"'"'public'"'"' group by current_database();"'
```

Ожидаются HTTP `200`, health `status=ok`, `DB_TYPE=postgres`, база `rag_flow` и
ненулевое число таблиц.

### 10.4. Закрытая регистрация

```bash
curl -fsS http://127.0.0.1:9380/api/v1/system/config | jq -e '
  (.data.registerEnabled // .registerEnabled) as $enabled |
  ($enabled == 0 or $enabled == false)
'
```

### 10.5. Superuser

```bash
sudo docker exec -i \
  -e CHECK_ADMIN_EMAIL=admin@example.org \
  "$ragflow_id" python - <<'PY'
import os
from common import settings

settings.init_settings()

from api.db.services import UserService
email = os.environ["CHECK_ADMIN_EMAIL"]
users = list(UserService.query(email=email))
assert len(users) == 1
assert users[0].is_superuser
print("superuser=ok")
PY
```

В `CHECK_ADMIN_EMAIL` указать тот же адрес, что использовался при установке.

## 11. Доступ к UI

Release-профиль публикует RAGFlow на всех интерфейсах Linux-сервера. Для первичной
приёмки открыть SSH-туннель с рабочей станции:

```bash
ssh -L 19380:127.0.0.1:9380 admin@ragflow-server
```

Открыть `http://127.0.0.1:19380/` и войти созданным superuser.

Для production ограничить порт корпоративным firewall и поставить reverse proxy с TLS перед
`127.0.0.1:9380`. Не публиковать PostgreSQL, Elasticsearch, Valkey и MinIO в
LAN/WAN. Docker published ports могут обходить UFW, поэтому проверять
политику также через `DOCKER-USER`, nftables или внешний firewall.

## 12. Настроить модели

После базовой установки выбрать реальный model provider. Минимальный набор:

| Назначение | Обязательно | Проверка |
| --- | --- | --- |
| Chat LLM | да | непустой ответ на контрольный prompt |
| Embedding | да | индексация и retrieval тестового документа |
| Vision | по сценариям | распознавание контрольного изображения |
| Reranker | рекомендуется | rerank возвращает оценки и улучшает golden set |
| ASR | по сценариям | непустой transcript через RAGFlow |

Имена моделей, endpoint и API keys должны храниться в утверждённом secret
контуре. Health провайдера не заменяет реальный inference. Если модели локальные,
их сервис, GPU runtime и model cache — отдельный deployment-контур, которого в
этой поставке пока нет.

## 13. Автозапуск и reboot test

Compose использует restart policy `unless-stopped`, отдельный systemd unit для
тех же контейнеров не нужен.

```bash
sudo systemctl enable docker
rf_compose ps
sudo reboot
```

После возврата сервера заново определить функцию `rf_compose` и проверить:

```bash
sudo systemctl is-active docker
rf_compose ps
curl -fsS http://127.0.0.1:9380/api/v1/system/healthz | jq -e \
  '.status == "ok"'
```

## 14. Backup

Резервировать независимо:

- PostgreSQL RAGFlow;
- MinIO volume с загруженными файлами;
- `/opt/ragflow-pg/docker/.env`;
- `/etc/ragflow-pg/admin.env` и `deployed-source.env`;
- исходный release-архив, его `.sha256` и `DEPLOYMENT-SOURCE.env`;
- Elasticsearch snapshots либо гарантированно воспроизводимый индекс;
- reverse proxy, TLS и firewall-конфигурацию.

Пример логического backup PostgreSQL:

```bash
backup_dir="/var/backups/ragflow/$(date +%F_%H%M%S)"
sudo install -d -m 0700 "$backup_dir"
postgres_id=$(rf_compose ps -q postgres)

sudo docker exec "$postgres_id" sh -lc \
  'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc -f /tmp/rag_flow.dump'
sudo docker cp "$postgres_id:/tmp/rag_flow.dump" \
  "$backup_dir/rag_flow.dump"
sudo docker exec "$postgres_id" rm -f /tmp/rag_flow.dump
sudo sha256sum "$backup_dir/rag_flow.dump" | \
  sudo tee "$backup_dir/SHA256SUMS"
```

Backup не принят, пока не выполнено тестовое восстановление в изолированный
Compose-проект.

## 15. Обновление и откат

Не запускать `install.sh` поверх существующего контура: он генерирует новые
инфраструктурные секреты и предназначен только для чистой установки.

Утверждённая offline-поставка обновляется так:

```bash
cd /srv/ragflow-linux-pg-v1.12.0-offline

# Change gate: только проверки, без остановки приложения и миграций.
sudo env \
  SOURCE_DIR="$(mktemp -d)" \
  INSTALL_DIR=/opt/ragflow-pg \
  PROJECT_NAME=ragflow-pg \
  RAGFLOW_PORT=80 \
  BACKUP_ROOT=/var/backups/ragflow-pg \
  bash ./upgrade_offline.sh --check

# После подтверждения окна обслуживания.
sudo env \
  SOURCE_DIR="$(mktemp -d)" \
  INSTALL_DIR=/opt/ragflow-pg \
  PROJECT_NAME=ragflow-pg \
  RAGFLOW_PORT=80 \
  BACKUP_ROOT=/var/backups/ragflow-pg \
  BACKUP_MINIO=1 \
  bash ./upgrade_offline.sh
```

Повторный запуск уже установленной версии является проверенным `no-op`: wrapper
проверяет полный health и таблицу `system_audit_event`, но не загружает образы,
не создаёт backup и не запускает миграции. Архив следует распаковывать только в
новый каталог из `mktemp`, с `--no-same-owner --no-same-permissions`.
Внутренний `SOURCE_DIR` также должен быть пустым для каждого вызова, включая
`--check`: повторное использование распакованного source запрещено даже для
той же версии. Предыдущий каталог сохраняется; указанные команды создают новый.

Для registry-поставки заменить точку входа на `upgrade_registry.sh` и передать
те же проверенные параметры registry, что и при первой установке. Скрипт
проверяет checksum/manifests, сохраняет существующие секреты, делает и проверяет
PostgreSQL dump и MinIO archive, атомарно меняет каталог приложения, запускает
штатные идемпотентные schema migrations и принимает контур по полному health.
Файлы операции находятся в `/var/backups/ragflow-pg/<timestamp>-<from>-to-<to>`;
предыдущий код — в `/opt/ragflow-pg.previous-<timestamp>`.

Компактный source-архив, собранный с `-UseExistingFrontend`, можно применить без
обращения к registry, если все образы итогового Compose уже загружены:

```bash
cd /srv/ragflow-linux-pg-v1.12.0

sudo env OFFLINE_INSTALL=1 \
  INSTALL_DIR=/opt/ragflow-pg \
  PROJECT_NAME=ragflow-pg \
  RAGFLOW_PORT=80 \
  BACKUP_ROOT=/var/backups/ragflow-pg \
  bash deployment/linux-pg/upgrade.sh --check

sudo env OFFLINE_INSTALL=1 \
  INSTALL_DIR=/opt/ragflow-pg \
  PROJECT_NAME=ragflow-pg \
  RAGFLOW_PORT=80 \
  BACKUP_ROOT=/var/backups/ragflow-pg \
  BACKUP_MINIO=1 \
  bash deployment/linux-pg/upgrade.sh
```

Preflight проверяет Compose, зарегистрированный `runsc` и наличие каждого
образа до остановки приложения, backup и миграций. Отсутствующий образ является
блокирующей ошибкой. Без `OFFLINE_INSTALL=1` source-upgrade выполняет локальную
сборку и поэтому требует доступных build-зависимостей.

### Автоматический возврат кода

Если новый API не прошёл health за 10 минут или post-migration checks, скрипт
останавливает новый `ragflow-cpu`, возвращает прежний release-каталог и запускает
старый Compose. Это не является откатом данных: миграции схемы назад намеренно
не выполняются автоматически.

### Полное восстановление данных

Применять только если прежний код несовместим с уже изменённой схемой. Сначала
остановить `ragflow-cpu`, сверить `SHA256SUMS` в backup и сохранить неуспешный
контур для разбора. Затем в отдельном PostgreSQL-контейнере той же major-версии
выполнить тестовый `pg_restore --list` и пробное восстановление. После одобрения:

1. восстановить прежний release-каталог из пути `PREVIOUS_DIR` в `UPGRADE.env`;
2. пересоздать БД `rag_flow` и выполнить `pg_restore --clean --if-exists` из
   `postgres.dump`;
3. при наличии `minio-data.tar.gz` очистить только volume `minio_data` данного
   Compose-проекта и распаковать архив в `/data`;
4. запустить PostgreSQL, MinIO, Redis и RAGFlow;
5. пересоздать Elasticsearch-индексы штатной переиндексацией документов из
   PostgreSQL/MinIO; живой data directory Elasticsearch не копируется;
6. повторить health, вход администратора, поиск по контрольной базе и проверку
   цепочки audit/correlation событий.

Откат кода без restore данных допустим только после документированной проверки
совместимости схемы. `BACKUP_MINIO=0` разрешён лишь для отдельного согласованного
окна без изменений объектов и создаёт явный маркер `MINIO-BACKUP-SKIPPED.env`.

## 16. Приёмочный лист

- [ ] SHA-256 архива проверен до распаковки и сохранён в change-записи.
- [ ] В архиве нет `.git`, локальных `.env`, ключей, dump и model cache.
- [ ] `deployed-source.env` совпадает с manifest утверждённого архива.
- [ ] Frontend собран из того же release-архива.
- [ ] Постоянный `RAGFLOW_CREDENTIALS_KEY` сгенерирован и входит в защищённый backup.
- [ ] Compose config валиден и содержит 13 ожидаемых постоянных сервисов.
- [ ] Сервис и контейнер MySQL отсутствуют.
- [ ] RAGFlow health возвращает `db/doc_engine/redis/storage=ok`.
- [ ] PostgreSQL содержит схему RAGFlow; `DB_TYPE=postgres`.
- [ ] UI доступен только через loopback/tunnel либо TLS reverse proxy.
- [ ] Первый пользователь активен и `is_superuser=true`.
- [ ] `REGISTER_ENABLED=0` подтверждён runtime API.
- [ ] Выбраны и проверены chat LLM и embedding; reranker/vision — по сценарию.
- [ ] После reboot все сервисы восстанавливаются.
- [ ] Настроены мониторинг и алерты.
- [ ] Backup имеет checksum и тестово восстановлен.
- [ ] gVisor/runsc зарегистрирован, sandbox pool заполнен и CodeExec прошёл реальную проверку.
- [ ] Есть второй аварийный администратор и порядок отзыва доступа.

## 17. Типовые неисправности

### Installer сообщает ошибку `DEPLOYMENT-SOURCE.env`

```bash
cd /srv/ragflow-linux-pg
cat DEPLOYMENT-SOURCE.env
grep -Fx 'PACKAGE_FORMAT=tar.gz' DEPLOYMENT-SOURCE.env
test -n "$(sed -n 's/^RELEASE_VERSION=//p' DEPLOYMENT-SOURCE.env)"
```

Не создавать manifest вручную на сервере. Повторно проверить checksum,
распаковать утверждённый архив в пустой `/srv/ragflow-linux-pg` и запустить
installer из его корня.

### Compose не понимает `!override` или `!reset`

```bash
sudo docker compose version
```

Установить актуальный Docker Compose plugin по утверждённой процедуре ОС и
повторить `rf_compose config --quiet`. Не удалять теги из overlays: они
гарантируют замену upstream depends/ports/profiles.

### Frontend build не проходит

Проверить доступ к `node:20-bookworm-slim`, свободное место, `web/package-lock.json`
и полный вывод `npm ci`/`npm run build`. Не подменять сборку старым `web/dist`.

### RAGFlow не стал healthy за 10 минут

```bash
rf_compose ps
rf_compose logs --tail=300 ragflow-cpu postgres es01 redis minio
df -hT
free -h
sysctl vm.max_map_count
```

### Появился MySQL

Убедиться, что используется `deployment/linux-pg/docker-compose.release.yml`,
а `COMPOSE_PROFILES` не переопределён вручную. Installer обязан остановиться
ещё на `config --services`.

### Порт 9380 занят

Выбрать другой host port при первой установке, например
`RAGFLOW_PORT=19380`. Внутренние адреса контейнеров при этом не меняются.
