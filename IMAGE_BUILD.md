# Сборка Docker-образа

Образ собирается из исходников репозитория и получает имя
`citysense/ragflow:<дата-время>` (например, `citysense/ragflow:20260814-193827`),
поэтому каждый собранный образ уникален и не перезатирает предыдущий.

## Скрипты сборки

| Окружение | Скрипт |
|---|---|
| Linux-сервер | `build-ragflow-image.sh` |
| Windows (локально) | `build-ragflow-image.ps1` |

Оба скрипта только **собирают** образ и никогда не перезапускают контейнер
приложения — запуск выполняется вручную с нужным тегом.

### Linux-сервер

```bash
bash build-ragflow-image.sh --data-dir $(pwd)
```

- По умолчанию скрипт клонирует/обновляет репозиторий в `~/ragflow`
  (переменная `RAGFLOW_DATA_DIR` или флаг `--data-dir`).
- Ветка по умолчанию — `main` (`RAGFLOW_BRANCH` или флаг `--branch`).
- Тег по умолчанию — `citysense/ragflow:<дата-время>` (`RAGFLOW_IMAGE_TAG`
  или флаг `--tag`).
- `--no-cache` — полная пересборка без кэша BuildKit.

### Локально (Windows)

```powershell
.\build-ragflow-image.ps1
```

Дополнительные флаги: `-Tag myrepo/ragflow:v1`, `-NoCache`, `-UseMirror`.

## Что сохраняется при сборке

Скрипты не дают потерять локальные правки при обновлении репозитория
(`git reset --hard`):

- `docker/.env` — локальные значения настроек (тег образа, порты и т.п.);
- `docker/service_conf.yaml.template` — конфигурация сервисов;
- правки под Yandex SSO: `api/apps/auth/yandex.py`,
  `api/apps/auth/__init__.py`, `api/apps/restful_apis/user_api.py`.

## Запуск контейнера с собранным образом

1. В `docker/.env` прописать собранный тег:

   ```
   RAGFLOW_IMAGE=citysense/ragflow:20260814-193827
   ```

2. Запустить приложение:

   ```bash
   docker compose -f docker/docker-compose.yml up -d ragflow-cpu
   ```

Узнать доступные образы: `docker images citysense/ragflow`.
