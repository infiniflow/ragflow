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
  <a href="./README_ko.md"><img alt="한국어" src="https://img.shields.io/badge/한국어-DBEDFA"></a>
  <a href="./README_fr.md"><img alt="README en Français" src="https://img.shields.io/badge/Français-DFE0E5"></a>
  <a href="./README_id.md"><img alt="Bahasa Indonesia" src="https://img.shields.io/badge/Bahasa Indonesia-DFE0E5"></a>
  <a href="./README_pt_br.md"><img alt="Português(Brasil)" src="https://img.shields.io/badge/Português(Brasil)-DFE0E5"></a>
  <a href="./README_ar.md"><img alt="README in Arabic" src="https://img.shields.io/badge/Arabic-DFE0E5"></a>
  <a href="./README_tr.md"><img alt="Türkçe README" src="https://img.shields.io/badge/Türkçe-DFE0E5"></a>
  <a href="./README_ru.md"><img alt="Русская версия README" src="https://img.shields.io/badge/Русский-DFE0E5"></a>
</p>

<p align="center">
    <a href="https://x.com/intent/follow?screen_name=infiniflowai" target="_blank">
        <img src="https://img.shields.io/twitter/follow/infiniflow?logo=X&color=%20%23f5f5f5" alt="follow on X(Twitter)">
    </a>
    <a href="https://cloud.ragflow.io" target="_blank">
        <img alt="Static Badge" src="https://img.shields.io/badge/Get-Started-4e6b99">
    </a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/infiniflow/ragflow-stats/main/badges/docker-pulls.json&style=flat-square&logo=docker&logoColor=white" alt="RAGFlow Docker image downloads">
    </a>
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=Latest%20Release" alt="Latest Release">
    </a>
    <a href="https://github.com/infiniflow/ragflow/blob/main/LICENSE">
        <img height="21" src="https://img.shields.io/badge/License-Apache--2.0-ffffff?labelColor=d4eaf7&color=2e6cc4" alt="license">
    </a>
</p>

<h4 align="center">
  <a href="https://cloud.ragflow.io">Cloud</a> |
  <a href="https://ragflow.io/docs/dev/">Document</a> |
  <a href="https://github.com/infiniflow/ragflow/issues/12241">Roadmap</a> |
  <a href="https://discord.gg/NjYzJD3GM3">Discord</a>
</h4>

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="RAGFlow in the GitHub Octoverse" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/ragflow-octoverse.png" width="1200"/>
</div>

<div align="center">
<a href="https://trendshift.io/repositories/9064" target="_blank"><img src="https://trendshift.io/api/badge/repositories/9064" alt="infiniflow%2Fragflow | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/></a>
</div>


## 💡 RAGFlow란?

[RAGFlow](https://ragflow.io/) 는 최첨단 [RAG](https://ragflow.io/basics/what-is-rag)(Retrieval-Augmented Generation)와 Agent 기능을 융합하여 대규모 언어 모델(LLM)을 위한 우수한 컨텍스트 계층을 생성하는 선도적인 오픈소스 RAG 엔진입니다. 모든 규모의 기업에 적용 가능한 효율적인 RAG 워크플로를 제공하며, 통합 [컨텍스트 엔진](https://ragflow.io/basics/what-is-agent-context-engine)과 사전 구축된 Agent 템플릿을 통해 개발자들이 복잡한 데이터를 예외적인 효율성과 정밀도로 고급 구현도의 프로덕션 준비 완료 AI 시스템으로 변환할 수 있도록 지원합니다.

## 🎮 시작하기

[https://cloud.ragflow.io](https://cloud.ragflow.io)에서 저희 클라우드 서비스를 이용해 보세요.

로컬 배포는 [자체 호스팅](#-자체-호스팅)을 참조하세요.

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="Chunking demonstration" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/chunking.gif" width="1200"/>
<img alt="Agentic workflow demonstration" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/agentic-dark.gif" width="1200"/>
</div>

## 🔥 업데이트

- 2026-09-29 RAGFlow 1.0.0-rc1을 출시했습니다.

- 2026-09-10 사이트맵을 통한 웹 콘텐츠 수집을 추가했습니다.
- 2026-08-19 Knowledge Compilation을 도입했습니다. 문서 및 데이터셋 수준에서 Wiki, Graph, Tree, PageIndex, Mind Map, Timeline, Skills를 생성할 수 있습니다.
- 2026-08-19 Low, Medium, High, Ultra 사고 모드를 지원하는 Agentic RAG를 도입했습니다.
- 2026-07-02 Google BigQuery 데이터 소스 수집 및 증분 동기화를 추가했습니다.
- 2026-06-29 WhatsApp, DingTalk, WeCom 채팅 채널을 추가했습니다.
- 2026-05-26 Agent가 웹 페이지를 탐색하고 조작할 수 있는 Browser 구성 요소를 추가했습니다.
- 2026-04-21 기본 제공 데이터 수집 파이프라인 템플릿 7종을 추가했습니다.
- 2026-04-21 Agent 애플리케이션 게시, Sandbox 코드 실행, 차트 생성을 추가했습니다.
- 2026-04-21 사용자 수준 메모리 저장 및 검색을 추가했습니다.

자세한 내용은 [전체 릴리스 노트](./docs/release_notes.md)를 참조하세요.


## 🎉 계속 지켜봐 주세요

⭐️우리의 저장소를 즐겨찾기에 등록하여 흥미로운 새로운 기능과 업데이트를 최신 상태로 유지하세요! 모든 새로운 릴리스에 대한 즉시 알림을 받으세요! 🌟

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="RAGFlow feature updates" src="https://github.com/user-attachments/assets/18c9707e-b8aa-4caf-a154-037089c105ba" width="1200"/>
</div>

## 🌟 주요 기능

### 🍭 **"Quality in, quality out"**

- 심층 문서 이해를 기반으로 복잡한 형식의 비정형 데이터에서 지식을 추출합니다.
- 문자 그대로 무한한 토큰에서 "데이터 속의 바늘"을 찾아냅니다.

### 🍱 **템플릿 기반의 chunking**

- 똑똑하고 설명 가능한 방식.
- 다양한 템플릿 옵션을 제공합니다.

### 🧩 **지식 컴파일(Knowledge Compilation)**

- 문서와 데이터셋의 콘텐츠를 Wiki, Graph, Tree, PageIndex, Mind Map, Timeline, Skills 등의 구조화된 지식 산출물로 정리합니다.
- 컴파일 모델과 처리 규칙을 설정하고 산출물을 확인, 업데이트, 재생성할 수 있습니다.

### 🧠 **Agentic Retrieval**

- 복잡한 질문을 분석하고 필요하면 여러 단계로 질문을 분해하고 지식을 검색하며 근거를 검증합니다.
- Low, Medium, High, Ultra 사고 모드로 질문 복잡도에 따라 검색 및 추론 깊이를 조절할 수 있습니다.

### ⚙️ **Go 네이티브 서비스 아키텍처**

- 통합 Go 서비스가 API, Admin, Ingestor, Syncer를 제공합니다. DeepDoc은 Go 프로세스 안에서 레이아웃 분석, OCR, 표 인식을 수행합니다.
- Go 서비스는 CGO를 통해 네이티브 문서 파싱 라이브러리와 ONNX Runtime을 호출합니다. MCP와 Sandbox Executor는 필요할 때 활성화할 수 있습니다.

### 🌱 **할루시네이션을 줄인 신뢰할 수 있는 인용**

- 텍스트 청킹을 시각화하여 사용자가 개입할 수 있도록 합니다.
- 중요한 참고 자료와 추적 가능한 인용을 빠르게 확인하여 신뢰할 수 있는 답변을 지원합니다.

### 🍔 **다른 종류의 데이터 소스와의 호환성**

- 워드, 슬라이드, 엑셀, 텍스트 파일, 이미지, 스캔본, 구조화된 데이터, 웹 페이지 등을 지원합니다.

### 🛀 **자동화되고 손쉬운 RAG 워크플로우**

- 개인 및 대규모 비즈니스에 맞춘 효율적인 RAG 오케스트레이션.
- 구성 가능한 LLM 및 임베딩 모델.
- 다중 검색과 결합된 re-ranking.
- 비즈니스와 원활하게 통합할 수 있는 직관적인 API.

## 🔎 시스템 아키텍처

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/structure.jpg" alt="RAGFlow 시스템 아키텍처" width="1000" />
</div>

## 🎬 자체 호스팅

### 🐳 Docker 배포

#### 📝 Docker 배포 사전 요구 사항

- 권장 시작 구성: CPU 4코어, RAM 16 GB, 사용 가능한 디스크 공간 50 GB. 실제 요구 사항은 문서 엔진, 데이터 양, 파싱 작업, 동시 실행 수에 따라 달라집니다. 로컬 모델과 기타 선택적 구성 요소에는 추가 리소스가 필요할 수 있습니다.
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
- [gVisor](https://gvisor.dev/docs/user_guide/install/): Self-Managed 컨테이너 Sandbox를 사용하는 경우에만 필요합니다.

Docker 배포에는 호스트에 Go를 설치할 필요가 없습니다. Self-Managed 컨테이너 Sandbox는 gVisor 설치와 설정이 필요하지만, 다른 Sandbox 공급자는 RAGFlow 호스트에 gVisor를 설치할 필요가 없습니다.

> [!TIP]
> 로컬 머신(Windows, Mac, Linux)에 Docker가 설치되지 않은 경우, [Docker 엔진 설치](https://docs.docker.com/engine/install/)를 참조하세요.

#### 🚀 서버 시작하기

1. `vm.max_map_count`가 262144 이상인지 확인하세요:

   > `vm.max_map_count`의 값을 아래 명령어를 통해 확인하세요:
   >
   > ```bash
   > sysctl vm.max_map_count
   > ```
   >
   > 만약 `vm.max_map_count` 이 262144 보다 작다면 값을 쟈설정하세요.
   >
   > ```bash
   > # 이 경우에 262144로 설정했습니다.:
   > sudo sysctl -w vm.max_map_count=262144
   > ```
   >
   > 이 변경 사항은 시스템 재부팅 후에 초기화됩니다. 변경 사항을 영구적으로 적용하려면 /etc/sysctl.conf 파일에 vm.max_map_count 값을 추가하거나 업데이트하세요:
   >
   > ```bash
   > vm.max_map_count=262144
   > ```

2. 레포지토리를 클론하세요:

   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   ```

3. Go 릴리스 태그로 전환하고 사전 빌드된 Go 이미지를 Docker Compose로 시작하세요:

> [!CAUTION]
> 모든 Docker 이미지는 x86 플랫폼을 위해 빌드되었습니다. 우리는 현재 ARM64 플랫폼을 위한 Docker 이미지를 제공하지 않습니다.
> ARM64 플랫폼을 사용 중이라면, [시스템과 호환되는 Docker 이미지를 빌드하려면 이 가이드를 사용해 주세요](https://ragflow.io/docs/dev/build_docker_image).



   Docker 배포 디렉터리로 이동합니다.

   ```bash
   cd ragflow/docker
   ```

   Go v1.0.0-rc1 릴리스 태그로 전환합니다.

   ```bash
   git checkout v1.0.0-rc1
   ```

   Go 서비스와 종속 서비스를 백그라운드에서 시작합니다.

   ```bash
   docker compose -f docker-compose.yml up -d
   ```

   기본 MySQL 구성에서는 Go 이미지 진입점이 먼저 데이터베이스 마이그레이션을 실행한 다음 `bin/ragflow_server`를 통해 Syncer, Admin, API 및 Ingestor를 시작합니다.

> RAGFlow 오픈 소스 1.0의 DeepDoc은 레이아웃 분석, OCR, 표 인식에 CPU 추론을 사용합니다.

4. 시작 후 서비스 상태와 API 준비 상태를 확인하세요:

   ```bash
   docker ps
   ```

   위 명령은 종속 서비스 상태를 표시합니다. RAGFlow 자체에는 Compose healthcheck가 정의되어 있지 않으므로 API를 통해 준비 상태를 확인하세요:

   ```bash
   curl -f http://localhost/api/v1/system/healthz
   ```

   HTTP 200 응답은 준비가 완료되었음을 의미합니다. `SVR_WEB_HTTP_PORT`를 변경했다면 상태 확인 URL에 해당 포트를 사용하세요. 시작에 실패하면 `docker logs --tail 50 <service>`로 관련 서비스 로그를 확인하세요.

5. 웹 브라우저에 서버의 IP 주소를 입력하고 RAGFlow에 로그인하세요.
   > 기본 설정을 사용할 경우, `http://IP_OF_YOUR_MACHINE`만 입력하면 됩니다 (포트 번호는 제외). 기본 HTTP 서비스 포트 `80`은 기본 구성으로 사용할 때 생략할 수 있습니다.
6. RAGFlow에 로그인한 뒤 모델 공급자 페이지에서 LLM, 임베딩 모델, 리랭커를 추가하고 모델 이름, 서비스 주소, API 키를 입력하세요.

   > 자세한 내용은 [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup)를 참조하세요.

   _이제 쇼가 시작됩니다!_

#### ⚙️ Docker 구성 및 조정

Go Docker 배포는 `docker/.env`와 `docker/docker-compose.yml`을 사용하며, 캐시와 Checkpoint 저장에는 Kvrocks를, 메시지 큐에는 NATS JetStream을 사용합니다. 이미지, 포트, 비밀번호, 문서 엔진, 모델 이미지 소스를 변경하려면 [Docker 구성 가이드](./docker/README.md)를 따르세요. 플랫폼 제한과 macOS 요구 사항은 [Go Docker 이미지 빌드 및 플랫폼 지원 가이드](./docs/develop/build_docker_image.mdx)를 참조하세요.

문서 엔진 전환, 구성 변경 후 서비스 재시작, 기존 데이터 보존 또는 삭제 작업도 위 Docker 구성 가이드를 따르세요.

### 🔨 소스 코드로 서비스를 시작합니다.

#### 📝 소스 빌드 사전 요구 사항

1. `go.mod`에 지정된 Go 버전(현재 Go 1.27), Clang 20, LLD 20, CMake 4.0 이상, PCRE2 개발 파일을 설치합니다. Go 서비스는 CGO와 네이티브 라이브러리에 의존하며 [build.sh](./build.sh)가 필요한 빌드 매개변수를 설정합니다.

2. 저장소를 클론하고 빌드에 필요한 네이티브 라이브러리와 모델 파일을 준비한 뒤 Go 서비스를 빌드합니다:

   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   ```

   ```bash
   python3 -m venv /tmp/ragflow-go-download-venv
   /tmp/ragflow-go-download-venv/bin/python -m pip install requests huggingface-hub
   /tmp/ragflow-go-download-venv/bin/python ragflow_deps/download_go_deps.py
   bash build.sh --all
   ```

   이 스크립트는 Go 빌드에 필요한 네이티브 라이브러리와 모델 리소스를 준비하며 `requests`와 `huggingface-hub`가 필요합니다. 같은 리소스를 다른 방법으로 준비했다면 이 단계를 건너뛸 수 있습니다. 저장소 루트에서 시작하면 Go 서비스가 `rag/res/deepdoc`을 자동으로 찾습니다. 다른 디렉터리에서 시작하려면 `DEEPDOC_MODEL_DIR`을 해당 디렉터리의 절대 경로로 설정하세요.

3. Docker Compose를 사용하여 필요한 의존 서비스(Elasticsearch, MySQL, MinIO, NATS, Kvrocks, ClickHouse)를 시작합니다:

   ```bash
   sudo sysctl -w vm.max_map_count=262144
   docker compose --env-file docker/.env -f docker/docker-compose-base.yml \
     up -d --wait es01 mysql minio nats kvrocks clickhouse
   ```

   소스에서 실행하는 Go 서비스는 `localhost:6379`로 Kvrocks에 연결하므로 제공된 구성에서는 `/etc/hosts`를 수정할 필요가 없습니다.


4. 데이터베이스 마이그레이션 후 서비스를 순서대로 시작합니다. 각 명령은 저장소 루트에서 별도의 터미널로 실행하고, 네 개의 서비스 터미널을 계속 열어 둡니다:

   ```bash
   ./bin/ragflow_server --migrate
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --admin
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --ingestor
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --syncer
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --api
   ```

   시작 모드의 역할은 다음과 같습니다:

   - `--migrate`: 데이터베이스 마이그레이션을 실행한 뒤 종료합니다.
   - `--admin`: 관리 및 초기화 작업을 위한 Admin 서비스를 시작합니다.
   - `--ingestor`: 데이터 수집 및 파싱 작업을 위한 Ingestor 서비스를 시작합니다.
   - `--syncer`: 데이터 동기화 작업을 위한 Syncer 서비스를 시작합니다.
   - `--api`: Web UI, SDK 및 외부 클라이언트를 위한 API 서비스를 시작합니다.

   `RAGFLOW_DEV_MODE=true`는 개발 전용입니다. 코드 버전과 데이터베이스 마이그레이션 버전 사이의 다운그레이드 검사를 비활성화하지만 마이그레이션을 실행하거나 스키마를 변경하지 않습니다. 프로덕션에서는 설정하지 마세요. Admin을 다른 서비스보다 먼저 시작하세요. 마이그레이션 후 `RAGFLOW_DEV_MODE=true bash build.sh --run`은 Admin, Ingestor, API를 시작하지만 Syncer는 시작하지 않습니다. 전체 서비스 체인에는 `RAGFLOW_DEV_MODE=true ./bin/ragflow_server --syncer`를 별도로 실행하세요.
5. 프론트엔드를 개발할 때만 Node.js와 npm을 설치한 다음 React 프론트엔드를 시작합니다:

   ```bash
   cd web
   npm install
   API_PROXY_SCHEME=go npm run dev
   ```

   다른 터미널에서 Go API가 준비되었는지 확인합니다:

   ```bash
   curl -f http://127.0.0.1:9380/api/v1/system/healthz
   ```

   HTTP 200 응답은 API가 정상적으로 응답함을 의미합니다. 개발이 끝나면 각 서비스 터미널에서 `Ctrl+C`를 누릅니다. 컨테이너를 유지한 채 의존 서비스를 중지하려면 `docker compose --env-file docker/.env -f docker/docker-compose-base.yml stop es01 mysql minio nats kvrocks clickhouse`를 실행합니다. 명명된 볼륨을 유지하면서 의존 컨테이너와 Compose 네트워크를 삭제하려면 `docker compose --env-file docker/.env -f docker/docker-compose-base.yml down`을 실행합니다.

자세한 내용은 [소스에서 서비스 시작](./docs/develop/launch_ragflow_from_source.md)을 참조하세요.


## 📚 문서

- [Quickstart](https://ragflow.io/docs/dev/)
- [Configuration](https://ragflow.io/docs/dev/configurations)
- [Release notes](https://ragflow.io/docs/dev/release_notes)
- [User guides](https://ragflow.io/docs/category/user-guides)
- [Developer guides](https://ragflow.io/docs/category/developer-guides)
- [References](https://ragflow.io/docs/dev/category/references)
- [FAQs](https://ragflow.io/docs/dev/faq)

## 📜 로드맵

[RAGFlow 로드맵 2026](https://github.com/infiniflow/ragflow/issues/12241)을 확인하세요.

## 🏄 커뮤니티

- [Discord](https://discord.gg/NjYzJD3GM3)
- [X](https://x.com/infiniflowai)
- [GitHub Discussions](https://github.com/orgs/infiniflow/discussions)

## 🙌 컨트리뷰션

RAGFlow는 오픈소스 협업을 통해 발전합니다. 이러한 정신을 바탕으로, 우리는 커뮤니티의 다양한 기여를 환영합니다. 참여하고 싶으시다면, 먼저 [가이드라인](https://ragflow.io/docs/dev/contributing)을 검토해 주세요.
