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
- 🔥 [Последние обновления](#-последние-обновления)
- 🌟 [Ключевые возможности](#-ключевые-возможности)
- 🔎 [Архитектура системы](#-архитектура-системы)
- 🏠 [Локальное развёртывание](#-локальное-развёртывание)
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

- 2026-09-29 Выпущен RAGFlow 1.0.0-rc1.

- 2026-09-10 Добавлен сбор веб-контента через sitemap.
- 2026-08-19 Представлена Knowledge Compilation для создания Wiki, Graph, Tree, PageIndex, Mind Map, Timeline и Skills на уровне документов и наборов данных.
- 2026-08-19 Представлен Agentic RAG с режимами рассуждения Low, Medium, High и Ultra.
- 2026-07-02 Добавлены источник данных Google BigQuery и инкрементальная синхронизация.

Другие обновления см. в [полных примечаниях к выпускам](https://ragflow.io/docs/dev/release_notes).

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

### 🌱 **Обоснованные цитаты с минимальными галлюцинациями**

- Визуализация чанкинга с возможностью ручной корректировки.
- Быстрый просмотр ключевых источников и отслеживаемых цитат.

### 🍔 **Работа с разнородными источниками данных**

- Поддержка документов Word, презентаций PowerPoint, таблиц Excel, файлов TXT, изображений, PDF, сканов, структурированных данных, веб-страниц и многого другого.

### 🛀 **Автоматизированный и простой RAG-пайплайн**

- Удобная оркестрация RAG как для личных проектов, так и для крупных компаний.
- Настраиваемые LLM и модели эмбеддингов.
- Множественный поиск + fused re-ranking.
- Понятные API для простой интеграции.

## 🔎 Архитектура системы

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/structure.jpg" alt="RAGFlow system architecture" width="1000" />
</div>

## 🏠 Локальное развёртывание

Docker подходит для быстрой оценки, интеграционных тестов и production, а запуск из исходников — для разработки и отладки. Docker не требует Go. Для исходников нужна версия Go из `go.mod`, а для frontend — Node.js и npm.

### 🐳 Развёртывание Docker

#### 📝 Требования

- Рекомендуемая начальная конфигурация: 4 ядра CPU, 16 ГБ RAM и 50 ГБ свободного места.
- Docker ≥ 24.0.0, Docker Compose ≥ v2.26.1.

#### 🚀 Запуск сервера

1. Elasticsearch: `vm.max_map_count` ≥ 262144.

2. `git clone https://github.com/infiniflow/ragflow.git`

3. Docker:

   ```bash
   cd ragflow/docker
   git checkout v1.0.0-rc1
   docker compose -f docker-compose.yml up -d
   ```

4. Проверьте состояние сервисов и готовность API:

   ```bash
   curl -f http://localhost/api/v1/system/healthz
   docker logs --tail 50 ragflow-cpu
   ```

5. Откройте `http://IP_OF_YOUR_MACHINE` в браузере и войдите в систему.

6. Добавьте LLM, Embedding и Reranker на странице поставщиков моделей. Подробнее: [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup).

Подробнее см. в [кратком руководстве](./docs/quickstart.mdx).

#### ⚙️ Настройка Docker

Используйте `docker/.env` и `docker/docker-compose.yml`. См. [настройку Docker](./docker/README.md) и [сборку образа и поддержку платформ](./docs/develop/build_docker_image.mdx). macOS сейчас не поддерживается.

### 🔨 Запуск из исходников

#### 📝 Требования

1. Клонируйте репозиторий и установите Go из `go.mod`, Clang 20, LLD 20, CMake ≥ 4.0, PCRE2 и нативные библиотеки для CGO.

2. Подготовьте зависимости:

   ```bash
   python3 -m venv /tmp/ragflow-go-download-venv
   /tmp/ragflow-go-download-venv/bin/python -m pip install requests huggingface-hub
   /tmp/ragflow-go-download-venv/bin/python ragflow_deps/download_deps.py
   ./build.sh --all
   ```

3. Соберите сервисы:

   ```bash
   ./build.sh --all
   ```

4. Запустите локальные зависимости:

   ```bash
   sudo sysctl -w vm.max_map_count=262144
   docker compose --env-file docker/.env -f docker/docker-compose-base.yml up -d --wait es01 mysql minio nats kvrocks
   ```

5. Выполните миграцию базы данных:

   ```bash
   ./bin/ragflow_server --migrate
   ```

6. Запустите четыре сервиса в четырёх терминалах:

   Терминал 1: Admin (`9381`).

   ```bash
   ./bin/ragflow_server --admin
   ```

   Терминал 2: Ingestor.

   ```bash
   ./bin/ragflow_server --ingestor
   ```

   Терминал 3: Syncer.

   ```bash
   ./bin/ragflow_server --syncer
   ```

   Терминал 4: API (`9380`).

   ```bash
   ./bin/ragflow_server --api
   ```

7. Только для frontend установите Node.js и npm и проверьте API:

   ```bash
   cd web
   npm install
   curl -f http://127.0.0.1:9380/api/v1/system/healthz
   ```

Подробнее см. [Запуск из исходников](./docs/develop/launch_ragflow_from_source.md).

> **Эксплуатационные примечания:** для постоянной настройки Elasticsearch добавьте `vm.max_map_count=262144` в `/etc/sysctl.conf`. При стандартном MySQL точка входа Docker выполняет миграцию перед запуском Admin, Syncer, Ingestor и API. В open-source 1.0 DeepDoc использует CPU для анализа макета, OCR и таблиц. После изменения Docker перезапустите сервисы и следуйте [руководству Docker](./docker/README.md) для сохранения или удаления данных. `download_deps.py` подготавливает нативные библиотеки и модели и требует `requests` и `huggingface-hub`. `--migrate` выполняет миграцию и завершается; остальные параметры запускают соответствующие сервисы. HTTP 200 означает готовность API.

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
