# RAGFlow Linux PostgreSQL release archive

Этот каталог собирает RAGFlow в один `tar.gz`, который передаётся через
`pscp.exe` и устанавливается на Rocky Linux 9.x, Ubuntu 24.04 или Debian 12
`x86_64` без Git на сервере. Целостность поставки подтверждается отдельным
SHA-256 файлом, а версия и происхождение записаны внутри
`DEPLOYMENT-SOURCE.env`.

Полный пошаговый документ:
[`docs/administrator/linux_sources_runbook_ru.md`](../../docs/administrator/linux_sources_runbook_ru.md).

## Компактная поставка через registry Цифры

Этот вариант не включает `docker-images.tar` и не собирает образы на сервере.
Пятнадцать образов заранее публикуются в доступный серверу OCI registry,
а в архив входят только исходники, готовый `web/dist`, список образов и
контрольные суммы. В поставку входят T-One ASR, стек наблюдаемости и sandbox.

Сначала авторизоваться в registry через `docker login`, подготовить и
опубликовать образы:

```powershell
./deployment/linux-pg/publish_registry_images.ps1 `
  -RegistryPrefix registry.example.org/cifra `
  -ReleaseVersion v1.12.0 `
  -Push
```

Затем собрать компактный архив с точными ссылками на опубликованные образы:

```powershell
./deployment/linux-pg/build_registry_archive.ps1 `
  -ReleaseVersion v1.12.0 `
  -ImagesEnvPath ./deployment/linux-pg/registry-images-v1.12.0.env
```

Повторную упаковку уже проверенного `web/dist` можно ускорить явным параметром
`-UseExistingFrontend`. Без него production frontend всегда собирается заново.

Если `ImagesEnvPath` не указан, сборщик положит в архив шаблон. Тогда при
установке нужно передать `REGISTRY_PREFIX`, а registry должен сохранять
стандартную структуру путей из шаблона.

Все адреса `*.example` ниже являются только примерами и намеренно не работают.
Нужно получить реальный DNS-адрес OCI registry у администратора инфраструктуры.

Загрузка через PuTTY с PPK-ключом:

```powershell
./deployment/linux-pg/upload_registry_release.ps1 `
  -PackagePath ./deployment/linux-pg/ragflow-linux-pg-v1.12.0-registry.tar.gz `
  -HostName ragflow.example.org `
  -UserName deploy `
  -PrivateKeyPath C:\Keys\ragflow.ppk `
  -HostKey 'ssh-ed25519 255 SHA256:REPLACE_WITH_PIN'
```

На сервере:

```bash
cd /tmp/ragflow-registry-release
sha256sum -c ragflow-linux-pg-v1.12.0-registry.tar.gz.sha256
tar -xzf ragflow-linux-pg-v1.12.0-registry.tar.gz

sudo env \
  ADMIN_EMAIL=admin@example.org \
  ADMIN_NICKNAME='RAGFlow Administrator' \
  REGISTRY_PREFIX=registry.example.org/cifra \
  bash install_registry.sh
```

До установки комплект можно проверить без изменений системы. Проверка требует,
чтобы реальный registry разрешался через DNS, а его HTTPS `/v2/` отвечал `200`
или `401`:

```bash
REGISTRY_PREFIX=registry.example.org/cifra bash install_registry.sh --check
```

Для закрытого registry добавить `REGISTRY_USERNAME` и
`REGISTRY_PASSWORD_FILE`. Пароль передаётся Docker через stdin и не должен
входить в архив поставки. Если архив собран с `ImagesEnvPath`,
`REGISTRY_PREFIX` при установке не требуется.

## Офлайн-поставка образов для Rocky Linux 9.x x86_64

Рекомендуемый вариант для текущего Rocky-сервера без доступа к Docker Hub. На
Windows из `S:\ragflow` выполнить:

```powershell
./deployment/linux-pg/build_offline_archive.ps1 -ReleaseVersion v1.12.0
```

После отдельно успешно проверенной production-сборки того же clean release-tree
повторную упаковку можно выполнить с `-UseExistingFrontend`; без этого параметра
сборщик всегда заново выполняет `pnpm install --frozen-lockfile` и `pnpm run build`.

Сборщик не вызывает Git. Он создаёт один архив и checksum, содержащие:

- source snapshot и готовый `web/dist`;
- 15 Linux/amd64 Docker-образов: базовые сервисы, T-One ASR,
  Grafana/Prometheus/Loki/Tempo/OTel Collector, sandbox manager и Python/Node.js
  sandbox base images;
- зафиксированный gVisor/runsc `20260817.0` для изоляции CodeExec;
- внутренний `SHA256SUMS` для каждого payload-файла.

Результат:

- `ragflow-linux-pg-v1.12.0-offline.tar.gz`;
- `ragflow-linux-pg-v1.12.0-offline.tar.gz.sha256`.

Передать оба файла через `pscp.exe`, затем в PuTTY:

```bash
cd /tmp
sha256sum -c ragflow-linux-pg-v1.12.0-offline.tar.gz.sha256
package_dir=$(mktemp -d /tmp/ragflow-offline-v1.12.0.XXXXXX)
sudo tar --no-same-owner --no-same-permissions \
  -xzf ragflow-linux-pg-v1.12.0-offline.tar.gz -C "$package_dir"
cd "$package_dir"

sudo env \
  ADMIN_EMAIL=admin@example.org \
  ADMIN_NICKNAME='RAGFlow Administrator' \
  bash install_offline.sh
```

Offline installer проверяет все суммы, устанавливает Docker Engine/Compose и
системные утилиты из включённого корпоративного `cifra-docker`, загружает образы
через `docker load` и запускает Compose с `--pull never --no-build`. Доступ к
Docker Hub на сервере не используется. Требуются Rocky Linux 9.x x86_64,
доступные корпоративные DNF-репозитории, `sudo`, `tar` и `sha256sum`.

Chat LLM и embedding-модели не входят в базовую поставку. T-One ASR входит и
доступен RAGFlow по внутреннему адресу `http://t-one-asr:9011/v1`.

## Компактная поставка с загрузкой зависимостей на сервере

На Rocky Linux 9.x установщик использует включённый корпоративный DNF-репозиторий
`cifra-docker` и пакеты `docker-ce`, `docker-ce-cli`, `containerd.io`,
`docker-buildx-plugin`, `docker-compose-plugin`. Другой repo id можно передать
через `DOCKER_DNF_REPO`.

Установка разворачивает изолированный Compose-проект:

- полный source tree RAGFlow из проверенного архива;
- frontend, собранный из этого же дерева в контейнере Node.js 20;
- PostgreSQL 16 вместо MySQL;
- Elasticsearch, Valkey, MinIO и PlantUML;
- T-One ASR;
- Grafana, Prometheus, Loki, Tempo и OpenTelemetry Collector;
- sandbox manager и изолированный пул CodeExec под gVisor/runsc;
- первого пользователя с `is_superuser=true`.

MySQL в этом профиле выключен. Интерфейс публикуется на
`0.0.0.0:80` и доступен по IP или DNS-имени сервера. Сетевой доступ должен быть
ограничен корпоративным firewall или TLS reverse proxy. Grafana (`3001`) и
Prometheus (`9090`) публикуются только на `127.0.0.1`.

## 1. Собрать файл поставки на Windows

Из `S:\ragflow` в PowerShell:

```powershell
./deployment/linux-pg/build_archive.ps1 -ReleaseVersion v1.12.0
```

Скрипт не вызывает Git и упаковывает текущее состояние файлов. В архив не
попадают `.git`, локальные `.env`, кэши, данные контейнеров, `node_modules`,
`web/dist`, локальные build/test-артефакты, скачанные `ragflow_deps`, `output` и
локальные model caches, старые архивы и каталоги предыдущих release-сборок.
Перед сборкой нужно завершить проверку всех изменений, которые должны войти в
поставку.

Для обновления через компактный source-архив frontend должен быть собран и
включён явно:

```powershell
corepack pnpm --dir web install --frozen-lockfile --ignore-scripts
corepack pnpm --dir web run build
./deployment/linux-pg/build_archive.ps1 `
  -ReleaseVersion v1.12.0 `
  -UseExistingFrontend
```

Параметр `-UseExistingFrontend` требует `web/dist/index.html`, включает только
готовый `web/dist` текущей рабочей копии и записывает
`FRONTEND_MODE=prebuilt` в manifest. Без параметра manifest содержит
`FRONTEND_MODE=excluded`, что подходит для первой source-установки: frontend
соберёт `install.sh` на сервере.

Результат:

- `deployment/linux-pg/ragflow-linux-pg-v1.12.0.tar.gz`;
- `deployment/linux-pg/ragflow-linux-pg-v1.12.0.tar.gz.sha256`.

Скрипт сам проверяет чтение архива, выполняет контрольную распаковку и убеждается,
что обязательные deployment-файлы присутствуют, а `.git` отсутствует.

## 2. Передать оба файла через PuTTY

На Windows заменить пользователя и адрес сервера:

```powershell
& 'C:\Program Files\PuTTY\pscp.exe' -P 22 `
  'S:\ragflow\deployment\linux-pg\ragflow-linux-pg-v1.12.0.tar.gz' `
  'admin@ragflow-server:/tmp/'

& 'C:\Program Files\PuTTY\pscp.exe' -P 22 `
  'S:\ragflow\deployment\linux-pg\ragflow-linux-pg-v1.12.0.tar.gz.sha256' `
  'admin@ragflow-server:/tmp/'
```

Пароль не указывать в командной строке: `pscp.exe` запросит его. Для PPK-ключа
добавить `-i 'C:\path\to\key.ppk'`.

## 3. Проверить и распаковать через PuTTY

```bash
cd /tmp
sha256sum -c ragflow-linux-pg-v1.12.0.tar.gz.sha256

sudo install -d -m 0755 /srv/ragflow-linux-pg
sudo tar -xzf ragflow-linux-pg-v1.12.0.tar.gz -C /srv/ragflow-linux-pg
cd /srv/ragflow-linux-pg

cat DEPLOYMENT-SOURCE.env
test -s deployment/linux-pg/install.sh
test -s deployment/linux-pg/docker-compose.release.yml
```

Ожидаемый результат `sha256sum` — `OK`, а в manifest должны быть ожидаемые
`RELEASE_VERSION` и `PACKAGE_FORMAT=tar.gz`. Git для этих операций не нужен.

## 4. Запустить первую установку

```bash
cd /srv/ragflow-linux-pg

sudo env \
  ADMIN_EMAIL=admin@example.org \
  ADMIN_NICKNAME='RAGFlow Administrator' \
  INSTALL_DIR=/opt/ragflow-pg \
  PROJECT_NAME=ragflow-pg \
  RAGFLOW_PORT=80 \
  DOCKER_DNF_REPO=cifra-docker \
  bash deployment/linux-pg/install.sh
```

Если `ADMIN_PASSWORD` не задан, установщик создаст случайный пароль и сохранит
его в `/etc/ragflow-pg/admin.env` с правами `0600`. Реальные инфраструктурные
секреты генерируются на сервере и не входят в архив.

`install.sh` предназначен только для первой установки в пустой `INSTALL_DIR`.
Он собирает frontend, запускает Compose и проверяет health RAGFlow, PostgreSQL и
superuser. Версия установленного архива сохраняется в
`/etc/ragflow-pg/deployed-source.env`. Размер диска и число CPU установщик
намеренно не используют как блокирующие проверки; ёмкость контролируется
эксплуатационным мониторингом.

## Обновление существующего сервера

Не запускайте `install.sh`, `install_offline.sh` или `install_registry.sh` поверх
существующего `/opt/ragflow-pg`: установщики первой инсталляции создают новые
секреты. Пакеты обновления содержат отдельные точки входа:

```bash
# Сначала только проверка пакета, сервера, версии и Compose-конфигурации.
sudo env SOURCE_DIR="$(mktemp -d)" INSTALL_DIR=/opt/ragflow-pg PROJECT_NAME=ragflow-pg \
  bash ./upgrade_offline.sh --check

# Затем плановое обновление с обязательным backup PostgreSQL и MinIO.
sudo env SOURCE_DIR="$(mktemp -d)" INSTALL_DIR=/opt/ragflow-pg PROJECT_NAME=ragflow-pg \
  BACKUP_ROOT=/var/backups/ragflow-pg BACKUP_MINIO=1 \
  bash ./upgrade_offline.sh
```

Повторный запуск той же версии выполняет полный health-check и проверяет
`system_audit_event`, после чего завершается успешно без backup, миграций и
повторной загрузки образов. Для распаковки всегда используйте новый каталог из
`mktemp`; это исключает конфликт владельца и метаданных с предыдущей попыткой.
`--check` тоже распаковывает payload: для последующего запуска нужен отдельный
пустой `SOURCE_DIR`. Непустой каталог отклоняется независимо от версии; скрипт
не удаляет предыдущую распаковку и не принимает её за содержимое нового пакета.

Для registry-поставки используются те же параметры и `upgrade_registry.sh`;
при шаблонном `images.env` дополнительно задаются `REGISTRY_PREFIX` и
`REGISTRY_HOST`. Обновление:

1. повторно проверяет `SHA256SUMS` и manifests пакета;
2. сохраняет без изменений `docker/.env`, `.env.local` и `/etc/ragflow-pg`;
3. останавливает только `ragflow-cpu`, делает проверяемый PostgreSQL dump и
   согласованную копию MinIO;
4. фиксирует список Elasticsearch-индексов и считает их пересоздаваемыми из
   PostgreSQL/MinIO вместо небезопасного файлового копирования живого индекса;
5. атомарно меняет release-каталог, после чего штатный старт выполняет
   идемпотентные schema migrations;
6. проверяет полный health, `DB_TYPE=postgres` и таблицу
   `system_audit_event`, затем записывает новую версию.

Backup и предыдущий release-каталог сохраняются до закрытия приёмочного окна.
При ошибке до прохождения health скрипт возвращает прежний код и контейнеры.
Миграции БД автоматически назад не откатываются: если старая версия несовместима
с новой схемой, выполняется полное восстановление по процедуре ниже.

Компактный source-архив для обновления должен быть собран с
`-UseExistingFrontend`. Если все образы уже загружены и сервер не имеет доступа
к внешним registry, сначала выполнить preflight без остановки приложения,
backup и миграций, а затем обновление в режиме запрета pull/build:

```bash
sudo env OFFLINE_INSTALL=1 INSTALL_DIR=/opt/ragflow-pg PROJECT_NAME=ragflow-pg \
  BACKUP_ROOT=/var/backups/ragflow-pg \
  bash deployment/linux-pg/upgrade.sh --check

sudo env OFFLINE_INSTALL=1 INSTALL_DIR=/opt/ragflow-pg PROJECT_NAME=ragflow-pg \
  BACKUP_ROOT=/var/backups/ragflow-pg BACKUP_MINIO=1 \
  bash deployment/linux-pg/upgrade.sh
```

В `--check` входят проверка Compose, `runsc` и наличия каждого образа итоговой
конфигурации. При отсутствии любого образа проверка завершается до остановки
приложения и создания backup. Без `OFFLINE_INSTALL=1` source-upgrade имеет право
собирать локальные образы и требует доступных build-зависимостей.
