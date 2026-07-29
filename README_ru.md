<div align="center">
<a href="https://cloud.ragflow.io/">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow/main/web/src/assets/logo-with-text.svg" width="520" alt="ragflow logo">
</a>
</div>

<p align="center">
  <a href="./README.md"><img alt="README in English" src="https://img.shields.io/badge/English-DFE0E5"></a>
  <a href="./README_zh.md"><img alt="简体中文版自述文件" src="https://img.shields.io/badge/简体中文-DFE0E5"></a>
  <a href="./README_tzh.md"><img alt="繁體版中文自述文件" src="https://img.shields.io/badge/繁體中文-DFE0E5"></a>
  <a href="./README_ja.md"><img alt="日本語のREADME" src="https://img.shields.io/badge/日本語-DFE0E5"></a>
  <a href="./README_ko.md"><img alt="한국어" src="https://img.shields.io/badge/한국어-DFE0E5"></a>
  <a href="./README_fr.md"><img alt="README en Français" src="https://img.shields.io/badge/Français-DFE0E5"></a>
  <a href="./README_id.md"><img alt="Bahasa Indonesia" src="https://img.shields.io/badge/Bahasa Indonesia-DFE0E5"></a>
  <a href="./README_pt_br.md"><img alt="Português(Brasil)" src="https://img.shields.io/badge/Português(Brasil)-DFE0E5"></a>
  <a href="./README_ar.md"><img alt="README in Arabic" src="https://img.shields.io/badge/Arabic-DFE0E5"></a>
  <a href="./README_tr.md"><img alt="Türkçe README" src="https://img.shields.io/badge/Türkçe-DFE0E5"></a>
  <a href="./README_ru.md"><img alt="Русская версия README" src="https://img.shields.io/badge/Русский-DBEDFA"></a>
</p>

<p align="center">
    <a href="https://x.com/intent/follow?screen_name=infiniflowai" target="_blank">
        <img src="https://img.shields.io/twitter/follow/infiniflow?logo=X&color=%20%23f5f5f5" alt="подписаться на X(Twitter)">
    </a>
    <a href="https://cloud.ragflow.io" target="_blank">
        <img alt="Static Badge" src="https://img.shields.io/badge/Get-Started-4e6b99">
    </a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/infiniflow/ragflow-stats/main/badges/docker-pulls.json&style=flat-square&logo=docker&logoColor=white" alt="docker pull infiniflow/ragflow:v0.26.4">
    </a>
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=%D0%9F%D0%BE%D1%81%D0%BB%D0%B5%D0%B4%D0%BD%D0%B8%D0%B9%20%D1%80%D0%B5%D0%BB%D0%B8%D0%B7" alt="Последний релиз">
    </a>
    <a href="https://github.com/infiniflow/ragflow/blob/main/LICENSE">
        <img height="21" src="https://img.shields.io/badge/License-Apache--2.0-ffffff?labelColor=d4eaf7&color=2e6cc4" alt="лицензия">
    </a>
    <a href="https://deepwiki.com/infiniflow/ragflow">
        <img alt="Ask DeepWiki" src="https://deepwiki.com/badge.svg">
    </a>
</p>

<h4 align="center">
  <a href="https://cloud.ragflow.io">Облако</a> |
  <a href="https://ragflow.io/docs/dev/">Документация</a> |
  <a href="https://github.com/infiniflow/ragflow/issues/12241">Дорожная карта</a> |
  <a href="https://discord.gg/NjYzJD3GM3">Discord</a>
</h4>

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="RAGFlow in the GitHub Octoverse" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/ragflow-octoverse.png" width="1200"/>
</div>

<div align="center">
<a href="https://trendshift.io/repositories/9064" target="_blank"><img src="https://trendshift.io/api/badge/repositories/9064" alt="infiniflow%2Fragflow | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/></a>
</div>

<details open>
<summary><b>📕 Содержание</b></summary>

- 💡 [Что такое RAGFlow?](#-что-такое-ragflow)
- 🎮 [Начало работы](#-начало-работы)
- 📌 [Последние обновления](#-последние-обновления)
- 🌟 [Ключевые возможности](#-ключевые-возможности)
- 🔎 [Архитектура системы](#-архитектура-системы)
- 🎬 [Самостоятельное развёртывание](#-самостоятельное-развёртывание)
- 🔧 [Конфигурация](#-конфигурация)
- 🔧 [Сборка Docker-образа](#-сборка-docker-образа)
- 🔨 [Запуск из исходников для разработки](#-запуск-из-исходников-для-разработки)
- 📚 [Документация](#-документация)
- 📜 [Дорожная карта](#-дорожная-карта)
- 🏄 [Сообщество](#-сообщество)
- 🙌 [Участие в разработке](#-участие-в-разработке)

</details>

## 💡 Что такое RAGFlow?

[RAGFlow](https://ragflow.io/) — ведущий open-source движок Retrieval-Augmented Generation ([RAG](https://ragflow.io/basics/what-is-rag)), который объединяет передовые возможности RAG с агентными технологиями и создаёт качественный контекстный слой для больших языковых моделей. Он предлагает понятный и масштабируемый RAG-пайплайн, подходящий как для небольших проектов, так и для крупных компаний. Благодаря объединённому [движку контекста](https://ragflow.io/basics/what-is-agent-context-engine) и готовым шаблонам агентов RAGFlow позволяет разработчикам быстро превращать сложные данные в высокоточные production-ready AI-системы.

## 🎮 Начало работы

Попробуйте облачную версию: [https://cloud.ragflow.io](https://cloud.ragflow.io).

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="Chunking demonstration" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/chunking.gif" width="1200"/>
<img alt="Agentic workflow demonstration" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/agentic-dark.gif" width="1200"/>
</div>

## 🔥 Последние обновления

- 2026-06-15 Поддержка нескольких каналов чата: Feishu, Discord, Telegram, Line и другие.
- 2026-04-24 Поддержка DeepSeek v4.
- 2026-03-24 [RAGFlow Skill на OpenClaw](https://clawhub.ai/yingfeng/ragflow-skill) — официальный навык для работы с датасетами RAGFlow через OpenClaw.
- 2025-12-26 Поддержка «Памяти» для AI-агентов.
- 2025-11-19 Поддержка Gemini 3 Pro.
- 2025-11-12 Синхронизация данных из Confluence, S3, Notion, Discord и Google Drive.
- 2025-10-23 Поддержка MinerU и Docling в качестве методов парсинга документов.
- 2025-10-15 Поддержка настраиваемого пайплайна ingestion.
- 2025-08-08 Поддержка новейших моделей серии GPT-5 от OpenAI.
- 2025-08-01 Поддержка агентных workflow и MCP.
- 2025-05-23 Добавлен компонент выполнения кода на Python/JavaScript в Agent.
- 2025-03-19 Поддержка мультимодальных моделей для понимания изображений внутри PDF и DOCX.

## 🎉 Следите за обновлениями

⭐️ Поставьте звезду репозиторию, чтобы не пропускать новые возможности и улучшения. Получайте уведомления о релизах!

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="RAGFlow feature updates" src="https://github.com/user-attachments/assets/18c9707e-b8aa-4caf-a154-037089c105ba" width="1200"/>
</div>

## 🌟 Ключевые возможности

### 🍭 **«Качество на входе — качество на выходе»**

- Извлечение знаний на основе [глубокого понимания документов](./deepdoc/README.md) из неструктурированных данных со сложным форматированием.
- Поиск «иголки в стоге данных» при практически неограниченном количестве токенов.

### 🍱 **Шаблонный чанкинг**

- Интеллектуальный и объяснимый.
- Большой выбор готовых шаблонов.

### 🌱 **Обоснованные цитаты с минимальными галлюцинациями**

- Визуализация чанкинга с возможностью ручной корректировки.
- Быстрый просмотр ключевых источников и отслеживаемых цитат.

### 🍔 **Работа с разнородными источниками данных**

- Поддержка Word, презентаций, Excel, txt, изображений, сканов, структурированных данных, веб-страниц и многого другого.

### 🛀 **Автоматизированный и простой RAG-пайплайн**

- Удобная оркестрация RAG как для личных проектов, так и для крупных компаний.
- Настраиваемые LLM и модели эмбеддингов.
- Множественный поиск + fused re-ranking.
- Понятные API для простой интеграции.

## 🔎 Архитектура системы

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="RAGFlow system architecture" src="https://github.com/user-attachments/assets/31b0dd6f-ca4f-445a-9457-70cb44a381b2" width="1000"/>
</div>

## 🎬 Самостоятельное развёртывание

### 📝 Требования

- CPU ≥ 4 ядра
- RAM ≥ 16 ГБ
- Диск ≥ 50 ГБ
- Docker ≥ 24.0.0 и Docker Compose ≥ v2.26.1
- Python ≥ 3.13
- [gVisor](https://gvisor.dev/docs/user_guide/install/) — нужен только если вы планируете использовать песочницу (code executor).

> [!TIP]
> Если Docker ещё не установлен (Windows, Mac или Linux), см. [Install Docker Engine](https://docs.docker.com/engine/install/).

### 🚀 Запуск сервера

1. Убедитесь, что `vm.max_map_count` ≥ 262144:

   > Проверить текущее значение:
   >
   > ```bash
   > sysctl vm.max_map_count
   > ```
   >
   > Если значение меньше 262144, установите:
   >
   > ```bash
   > sudo sysctl -w vm.max_map_count=262144
   > ```
   >
   > Чтобы изменение сохранилось после перезагрузки, добавьте в **/etc/sysctl.conf**:
   >
   > ```bash
   > vm.max_map_count=262144
   > ```

2. Клонируйте репозиторий:

   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   ```

3. Запустите сервер с помощью готовых Docker-образов:

> [!CAUTION]
> Все образы собраны под x86. Образов для ARM64 пока нет.
> Если вы на ARM64, следуйте [этому руководству](https://ragflow.io/docs/dev/build_docker_image), чтобы собрать образ самостоятельно.

> Команда ниже скачивает образ `v0.26.4`. Чтобы использовать другую версию, измените `RAGFLOW_IMAGE` в файле **docker/.env** перед запуском.

```bash
   cd ragflow/docker

   git checkout v0.26.4
   # Опционально: используйте стабильный тег (см. релизы)
   # Это гарантирует, что entrypoint.sh соответствует версии образа.

   # CPU-режим для DeepDoc:
   docker compose -f docker-compose.yml up -d

   # GPU-ускорение DeepDoc:
   # sed -i '1i DEVICE=gpu' .env
   # docker compose -f docker-compose.yml up -d
```

> До версии `v0.22.0` существовали образы с моделями эмбеддингов и slim-образы без них:

| Тег образа RAGFlow | Размер (ГБ) | Есть модели эмбеддингов? | Стабильный?      |
|--------------------|-------------|--------------------------|------------------|
| v0.21.1            | ≈9          | ✔️                       | Стабильный релиз |
| v0.21.1-slim       | ≈2          | ❌                        | Стабильный релиз |

> Начиная с `v0.22.0` поставляется только slim-редакция, суффикс `-slim` больше не используется.

4. Проверьте статус после запуска:

   ```bash
   docker logs -f docker-ragflow-cpu-1
   ```

   _Успешный запуск выглядит так:_

   ```bash
         ____   ___    ______ ______ __
        / __ \ /   |  / ____// ____// /____  _      __
       / /_/ // /| | / / __ / /_   / // __ \| | /| / /
      / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ /
     /_/ |_|/_/  |_|\____//_/    /_/ \____/ |__/|__/

    * Running on all addresses (0.0.0.0)
   ```

   > Если пропустить этот шаг и сразу открыть RAGFlow в браузере, может появиться ошибка `network abnormal` — система ещё не полностью инициализировалась.

5. Откройте в браузере IP-адрес сервера и войдите в RAGFlow.

   > При стандартных настройках достаточно `http://IP_ВАШЕЙ_МАШИНЫ` (порт 80 можно не указывать).

6. В файле [service_conf.yaml.template](./docker/service_conf.yaml.template) выберите нужного провайдера LLM в `user_default_llm` и укажите `API_KEY`.

   > Подробнее: [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup).

   _Готово!_

## 🔧 Конфигурация

Основные файлы конфигурации:

- [.env](./docker/.env) — базовые настройки системы (`SVR_HTTP_PORT`, `MYSQL_PASSWORD`, `MINIO_PASSWORD` и др.).
- [service_conf.yaml.template](./docker/service_conf.yaml.template) — конфигурация бэкенд-сервисов. Переменные окружения подставляются автоматически при старте контейнера.
- [docker-compose.yml](./docker/docker-compose.yml) — оркестрация сервисов.

> Подробное описание переменных и настроек есть в [./docker/README](./docker/README.md). Их можно использовать как `${ENV_VARS}` в `service_conf.yaml.template`.

Чтобы изменить HTTP-порт по умолчанию (80), в [docker-compose.yml](./docker/docker-compose.yml) замените `80:80` на `<ВАШ_ПОРТ>:80`.

После изменений перезапустите контейнеры:

```bash
docker compose -f docker-compose.yml up -d
```

### Переключение с Elasticsearch на Infinity

По умолчанию RAGFlow использует Elasticsearch. Чтобы перейти на [Infinity](https://github.com/infiniflow/infinity/):

1. Остановите все контейнеры:

   ```bash
   docker compose -f docker/docker-compose.yml down -v
   ```

> [!WARNING]
> Флаг `-v` удалит volumes — существующие данные будут потеряны.

2. В файле **docker/.env** установите `DOC_ENGINE=infinity`.
3. Запустите контейнеры:

   ```bash
   docker compose -f docker/docker-compose.yml up -d
   ```

> [!WARNING]
> Переключение на Infinity на Linux/arm64 пока официально не поддерживается.

## 🔧 Сборка Docker-образа

Образ занимает около 2 ГБ и рассчитывает на внешние LLM и сервисы эмбеддингов.

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
docker build --platform linux/amd64 -f Dockerfile -t infiniflow/ragflow:nightly .
```

Если вы за прокси:

```bash
docker build --platform linux/amd64 \
  --build-arg http_proxy=http://YOUR_PROXY:PORT \
  --build-arg https_proxy=http://YOUR_PROXY:PORT \
  -f Dockerfile -t infiniflow/ragflow:nightly .
```

## 🔨 Запуск из исходников для разработки

> [!IMPORTANT]
> После первого клонирования один раз выполните из корня репозитория:
> `git config --local --unset core.hooksPath`, `uv tool install lefthook` и `lefthook install`.

1. Установите `uv` (если ещё не установлен):

   ```bash
   pipx install uv
   ```

2. Клонируйте репозиторий и установите зависимости:

   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   uv sync --python 3.13
   uv run python3 ragflow_deps/download_deps.py
   git config --local --unset core.hooksPath
   uv tool install lefthook
   lefthook install
   ```

3. Запустите зависимости (MinIO, Elasticsearch, Redis, MySQL):

   ```bash
   docker compose -f docker/docker-compose-base.yml up -d
   ```

   Добавьте в `/etc/hosts`:

   ```text
   127.0.0.1       es01 infinity mysql minio redis sandbox-executor-manager
   ```

4. Если нет доступа к HuggingFace, укажите зеркало:

   ```bash
   export HF_ENDPOINT=https://hf-mirror.com
   ```

5. Если в системе нет jemalloc:

   ```bash
   # Ubuntu
   sudo apt-get install libjemalloc-dev
   # CentOS
   sudo yum install jemalloc
   # OpenSUSE
   sudo zypper install jemalloc
   # macOS
   brew install jemalloc
   ```

6. Запустите бэкенд:

   ```bash
   source .venv/bin/activate
   export PYTHONPATH=$(pwd)
   bash docker/launch_backend_service.sh
   ```

7. Установите зависимости фронтенда:

   ```bash
   cd web
   npm install
   ```

8. Запустите фронтенд:

   ```bash
   npm run dev
   ```

9. После разработки остановите сервисы:

   ```bash
   pkill -f "ragflow_server.py|task_executor.py"
   ```

## 📚 Документация

- [Быстрый старт](https://ragflow.io/docs/dev/)
- [Конфигурация](https://ragflow.io/docs/dev/configurations)
- [Примечания к релизам](https://ragflow.io/docs/dev/release_notes)
- [Руководства пользователя](https://ragflow.io/docs/category/user-guides)
- [Руководства для разработчиков](https://ragflow.io/docs/category/developer-guides)
- [Справочные материалы](https://ragflow.io/docs/dev/category/references)
- [FAQ](https://ragflow.io/docs/dev/faq)

## 📜 Дорожная карта

См. [Дорожную карту RAGFlow на 2026 год](https://github.com/infiniflow/ragflow/issues/12241)

## 🏄 Сообщество

- [Discord](https://discord.gg/NjYzJD3GM3)
- [X](https://x.com/infiniflowai)
- [GitHub Discussions](https://github.com/orgs/infiniflow/discussions)

## 🙌 Участие в разработке

RAGFlow развивается как open-source проект. Мы рады любым вкладам сообщества.
Если хотите помочь — сначала ознакомьтесь с [Contribution Guidelines](https://ragflow.io/docs/dev/contributing).
