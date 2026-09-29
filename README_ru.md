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
        <img src="https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/infiniflow/ragflow-stats/main/badges/docker-pulls.json&style=flat-square&logo=docker&logoColor=white" alt="RAGFlow Docker image downloads">
    </a>
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=%D0%9F%D0%BE%D1%81%D0%BB%D0%B5%D0%B4%D0%BD%D0%B8%D0%B9%20%D1%80%D0%B5%D0%BB%D0%B8%D0%B7" alt="Последний релиз">
    </a>
    <a href="https://github.com/infiniflow/ragflow/blob/main/LICENSE">
        <img height="21" src="https://img.shields.io/badge/License-Apache--2.0-ffffff?labelColor=d4eaf7&color=2e6cc4" alt="лицензия">
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

Для локального развёртывания см. раздел [Локальное развёртывание](#-самостоятельное-развёртывание).

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="Chunking demonstration" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/chunking.gif" width="1200"/>
<img alt="Agentic workflow demonstration" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/agentic-dark.gif" width="1200"/>
</div>

## 🔥 Последние обновления

- 2026-09-10 Добавлен сбор веб-контента через sitemap.
- 2026-08-19 Представлена Knowledge Compilation для создания Wiki, Graph, Tree, PageIndex, Mind Map, Timeline и Skills на уровне документов и наборов данных.
- 2026-08-19 Представлен Agentic RAG с режимами рассуждения Low, Medium, High и Ultra.
- 2026-07-02 Добавлены источник данных Google BigQuery и инкрементальная синхронизация.
- 2026-06-29 Добавлены каналы чата WhatsApp, DingTalk и WeCom.
- 2026-05-26 Добавлен компонент Browser для просмотра и взаимодействия агентов с веб-страницами.
- 2026-04-21 Добавлены семь встроенных шаблонов конвейеров ingest данных.
- 2026-04-21 Добавлены публикация приложений Agent, выполнение кода в sandbox и генерация диаграмм.
- 2026-04-21 Добавлены хранение и поиск пользовательской памяти.

Другие обновления см. в [полных примечаниях к выпускам](./docs/release_notes.md).

## 🎉 Следите за обновлениями

⭐️ Поставьте звезду репозиторию, чтобы не пропускать новые возможности и улучшения. Получайте уведомления о релизах!

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="RAGFlow feature updates" src="https://github.com/user-attachments/assets/18c9707e-b8aa-4caf-a154-037089c105ba" width="1200"/>
</div>

## 🌟 Ключевые возможности

### 🍭 **«Качество на входе — качество на выходе»**

- Извлечение знаний на основе глубокого понимания документов из неструктурированных данных со сложным форматированием.
- Поиск «иголки в стоге данных» при практически неограниченном количестве токенов.

### 🍱 **Шаблонный чанкинг**

- Интеллектуальный и объяснимый.
- Большой выбор готовых шаблонов.

### 🧩 **Компиляция знаний (Knowledge Compilation)**

- Преобразуйте содержимое документов и наборов данных в структурированные объекты знаний: Wiki, Graph, Tree, PageIndex, Mind Map, Timeline и Skills.
- Настраивайте модели компиляции и правила обработки, просматривайте, обновляйте и создавайте объекты знаний заново.

### 🧠 **Агентный поиск (Agentic Retrieval)**

- Модель анализирует сложные вопросы и при необходимости разбивает их на части, ищет знания и проверяет доказательства в несколько этапов.
- Режимы Low, Medium, High и Ultra позволяют регулировать глубину поиска и рассуждений в зависимости от сложности вопроса.

### ⚙️ **Нативная Go-архитектура сервисов**

- Единый сервис Go предоставляет API, Admin, Ingestor и Syncer. DeepDoc работает внутри процесса Go и выполняет анализ макета, OCR и распознавание таблиц.
- Сервисы Go вызывают нативные библиотеки обработки документов и ONNX Runtime через CGO. MCP и Sandbox Executor можно включать по мере необходимости.

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
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/structure.jpg" alt="Архитектура RAGFlow" width="1000" />
</div>

## 🎬 Самостоятельное развёртывание

### 🐳 Развёртывание с Docker

#### 📝 Требования к развёртыванию с Docker

- Рекомендуемая начальная конфигурация: 4 ядра CPU, 16 ГБ RAM и 50 ГБ свободного места на диске. Фактические требования зависят от движка документов, объёма данных, задач анализа и параллельной нагрузки. Для локальных моделей и других дополнительных компонентов могут потребоваться дополнительные ресурсы.
- Docker ≥ 24.0.0 и Docker Compose ≥ v2.26.1
- [gVisor](https://gvisor.dev/docs/user_guide/install/) — требуется только для Self-Managed контейнерной Sandbox.

Для Docker-развёртывания не требуется устанавливать Go на хост. Для Self-Managed контейнерной Sandbox необходимо установить и настроить gVisor; другим провайдерам Sandbox gVisor на хосте RAGFlow не требуется.

> [!TIP]
> Если Docker ещё не установлен (Windows, Mac или Linux), см. [Install Docker Engine](https://docs.docker.com/engine/install/).

#### 🚀 Запуск сервера

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

3. Переключитесь на тег релиза Go и запустите готовый образ Go с помощью Docker Compose:

> [!CAUTION]
> Все образы собраны под x86. Образов для ARM64 пока нет.
> Если вы на ARM64, следуйте [этому руководству](https://ragflow.io/docs/dev/build_docker_image), чтобы собрать образ самостоятельно.

   ```bash
   # Перейдите в каталог развертывания Docker.
   cd ragflow/docker
   # Переключитесь на тег релиза Go v1.0.0-rc1.
   git checkout v1.0.0-rc1
   # Запустите службы Go и их зависимости в фоновом режиме.
   docker compose -f docker-compose.yml up -d
   ```

> В открытой версии RAGFlow 1.0 DeepDoc использует CPU для анализа макета, OCR и распознавания таблиц.

4. Проверьте статус после запуска:

   ```bash
   docker compose -f docker-compose.yml ps
   curl -f http://localhost/api/v1/system/healthz
   ```

5. Откройте в браузере IP-адрес сервера и войдите в RAGFlow.

   > При стандартных настройках достаточно `http://IP_ВАШЕЙ_МАШИНЫ` (порт 80 можно не указывать).

6. После входа добавьте LLM, модель эмбеддингов и reranker на странице поставщиков моделей, затем укажите имя модели, адрес сервиса и API-ключ.

   > Подробнее: [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup).

   _Готово!_

#### ⚙️ Настройка Docker

Развёртывание Go в Docker использует `docker/.env` и `docker/docker-compose.yml`, Kvrocks для кэша и хранения Checkpoint, а NATS JetStream — в качестве очереди сообщений. Изменяйте образ, порты, пароли, движок документов и источник образов моделей в соответствии с [руководством по настройке Docker](./docker/README.md). Ограничения платформ и требования macOS описаны в [руководстве по сборке Go-образа и поддержке платформ](./docs/develop/build_docker_image.mdx).

При смене движка документов, изменении конфигурации и перезапуске сервисов, а также при сохранении или удалении существующих данных следуйте этому же руководству по настройке Docker.

### 🔨 Запуск из исходников для разработки

#### 📝 Требования для сборки из исходников

1. Установите версию Go из `go.mod` (сейчас Go 1.27), Clang 20, LLD 20, CMake 4.0 или новее и файлы разработки PCRE2. Сервисам Go нужны CGO и нативные библиотеки; [build.sh](./build.sh) задаёт необходимые параметры сборки.

2. Клонируйте репозиторий, подготовьте нативные библиотеки и файлы моделей, затем соберите сервисы Go:

   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   python3 -m venv /tmp/ragflow-go-download-venv
   /tmp/ragflow-go-download-venv/bin/python -m pip install requests huggingface-hub
   /tmp/ragflow-go-download-venv/bin/python ragflow_deps/download_go_deps.py
   bash build.sh --all
   ```

   Скрипт подготавливает нативные библиотеки и ресурсы моделей для сборки Go и требует `requests` и `huggingface-hub`. Пропустите этот шаг, если те же ресурсы подготовлены другим способом. При запуске из корня репозитория сервисы Go автоматически находят `rag/res/deepdoc`; для запуска из другого каталога задайте `DEEPDOC_MODEL_DIR` как абсолютный путь к нему.

3. Запустите необходимые зависимости (Elasticsearch, MySQL, MinIO, NATS, Kvrocks и ClickHouse):

   ```bash
   sudo sysctl -w vm.max_map_count=262144
   docker compose --env-file docker/.env -f docker/docker-compose-base.yml \
     up -d --wait es01 mysql minio nats kvrocks clickhouse
   ```

   Сервисы Go, запущенные из исходного кода, подключаются к Kvrocks через `localhost:6379`; предоставленная конфигурация не требует изменения `/etc/hosts`.


4. После миграции базы данных запустите сервисы по порядку. Выполняйте каждую команду в отдельном терминале из корня репозитория и оставьте открытыми четыре терминала сервисов:

   ```bash
   ./bin/ragflow_server --migrate
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --admin
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --ingestor
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --syncer
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --api
   ```

   Режимы запуска работают следующим образом:

   - `--migrate`: выполняет миграции базы данных и завершает работу.
   - `--admin`: запускает сервис Admin для управления и инициализации.
   - `--ingestor`: запускает сервис Ingestor для приёма и анализа данных.
   - `--syncer`: запускает сервис Syncer для синхронизации данных.
   - `--api`: запускает API для веб-интерфейса, SDK и внешних клиентов.

   `RAGFLOW_DEV_MODE=true` предназначен только для разработки. Он отключает проверку отката между версиями кода и миграций базы данных, но не запускает миграции и не изменяет схему. Не используйте его в production. Запускайте Admin раньше остальных сервисов. После миграции `RAGFLOW_DEV_MODE=true bash build.sh --run` запускает Admin, Ingestor и API, но не Syncer; для полной цепочки отдельно выполните `RAGFLOW_DEV_MODE=true ./bin/ragflow_server --syncer`.
5. Только для разработки фронтенда установите Node.js и npm, затем запустите React-фронтенд:

   ```bash
   cd web
   npm install
   API_PROXY_SCHEME=go npm run dev
   ```

   В другом терминале проверьте готовность Go API:

   ```bash
   curl -f http://127.0.0.1:9380/api/v1/system/healthz
   ```

   Ответ HTTP 200 означает, что API отвечает. После разработки нажмите `Ctrl+C` в каждом терминале сервиса. Чтобы остановить зависимости, сохранив контейнеры, выполните `docker compose --env-file docker/.env -f docker/docker-compose-base.yml stop es01 mysql minio nats kvrocks clickhouse`. Чтобы удалить контейнеры зависимостей и сеть Compose, сохранив именованные тома, выполните `docker compose --env-file docker/.env -f docker/docker-compose-base.yml down`.

Подробнее см. [Запуск сервиса из исходников](./docs/develop/launch_ragflow_from_source.md).

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
