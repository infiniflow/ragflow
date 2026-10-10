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


<details open>
<summary><b>📕 목차</b></summary>

- 💡 [RAGFlow란?](#-ragflow란)
- 🎮 [시작하기](#-시작하기)
- 🔥 [업데이트](#-업데이트)
- 🌟 [주요 기능](#-주요-기능)
- 🔎 [시스템 아키텍처](#-시스템-아키텍처)
- 🏠 [로컬 배포](#-로컬-배포)
- 📚 [문서](#-문서)
- 📜 [로드맵](#-로드맵)
- 🏄 [커뮤니티](#-커뮤니티)
- 🙌 [컨트리뷰션](#-컨트리뷰션)

</details>

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

자세한 내용은 [전체 릴리스 노트](https://ragflow.io/docs/dev/release_notes)를 참조하세요.


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

### 🌱 **할루시네이션을 줄인 신뢰할 수 있는 인용**

- 텍스트 청킹을 시각화하여 사용자가 개입할 수 있도록 합니다.
- 중요한 참고 자료와 추적 가능한 인용을 빠르게 확인하여 신뢰할 수 있는 답변을 지원합니다.

### 🍔 **다른 종류의 데이터 소스와의 호환성**

- Word 문서, PowerPoint 프레젠테이션, Excel 스프레드시트, TXT 파일, 이미지, PDF, 스캔본, 구조화된 데이터, 웹 페이지 등을 지원합니다.

### 🛀 **자동화되고 손쉬운 RAG 워크플로우**

- 개인 및 대규모 비즈니스에 맞춘 효율적인 RAG 오케스트레이션.
- 구성 가능한 LLM 및 임베딩 모델.
- 다중 검색과 결합된 re-ranking.
- 비즈니스와 원활하게 통합할 수 있는 직관적인 API.

## 🔎 시스템 아키텍처

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/structure.jpg" alt="RAGFlow system architecture" width="1000" />
</div>

## 🏠 로컬 배포

Docker는 빠른 평가, 통합 테스트 및 프로덕션에 적합하고 소스 실행은 개발과 디버깅에 적합합니다. Docker에는 Go가 필요하지 않습니다. 소스 실행에는 `go.mod`의 Go가, 프런트엔드 개발에는 Node.js와 npm이 필요합니다.

### 🐳 Docker 배포

#### 📝 사전 요구 사항

- 권장 초기 구성: CPU 코어 4개, RAM 16 GB, 사용 가능한 디스크 50 GB.
- Docker ≥ 24.0.0, Docker Compose ≥ v2.26.1.

#### 🚀 서버 시작

1. Elasticsearch: `vm.max_map_count` ≥ 262144.

2. `git clone https://github.com/infiniflow/ragflow.git`

3. Docker:

   ```bash
   cd ragflow/docker
   git checkout v1.0.0-rc1
   docker compose -f docker-compose.yml up -d
   ```

4. 서비스와 API 준비 상태를 확인합니다:

   ```bash
   curl -f http://localhost/api/v1/system/healthz
   docker logs --tail 50 ragflow-cpu
   ```

5. 브라우저에서 `http://IP_OF_YOUR_MACHINE`을 열고 로그인합니다.

6. 모델 공급자 페이지에서 LLM, Embedding, Reranker를 추가합니다. 자세한 내용은 [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup)를 참조하세요.

자세한 내용은 [빠른 시작](./docs/quickstart.mdx)을 참조하세요.

#### ⚙️ Docker 구성 및 조정

`docker/.env`와 `docker/docker-compose.yml`을 사용합니다. [Docker 구성](./docker/README.md)과 [이미지 빌드 및 플랫폼 지원](./docs/develop/build_docker_image.mdx)을 참조하세요. macOS는 현재 지원되지 않습니다.

### 🔨 소스 코드로 시작

#### 📝 사전 요구 사항

1. 저장소를 클론하고 `go.mod`에 지정된 Go, Clang 20, LLD 20, CMake ≥ 4.0, PCRE2 및 CGO에 필요한 네이티브 라이브러리를 설치합니다.

2. 의존성을 준비합니다:

   ```bash
   python3 -m venv /tmp/ragflow-go-download-venv
   /tmp/ragflow-go-download-venv/bin/python -m pip install requests huggingface-hub
   /tmp/ragflow-go-download-venv/bin/python ragflow_deps/download_deps.py
   ./build.sh --all
   ```

3. 서비스를 빌드합니다:

   ```bash
   ./build.sh --all
   ```

4. 로컬 의존 서비스를 시작합니다:

   ```bash
   sudo sysctl -w vm.max_map_count=262144
   docker compose --env-file docker/.env -f docker/docker-compose-base.yml up -d --wait es01 mysql minio nats kvrocks
   ```

5. 데이터베이스를 마이그레이션합니다:

   ```bash
   ./bin/ragflow_server --migrate
   ```

6. 네 서비스를 네 터미널에서 시작합니다:

   터미널 1: Admin (`9381`).

   ```bash
   ./bin/ragflow_server --admin
   ```

   터미널 2: Ingestor.

   ```bash
   ./bin/ragflow_server --ingestor
   ```

   터미널 3: Syncer.

   ```bash
   ./bin/ragflow_server --syncer
   ```

   터미널 4: API (`9380`).

   ```bash
   ./bin/ragflow_server --api
   ```

7. 프런트엔드 개발 시에만 Node.js와 npm을 설치하고 API를 확인합니다:

   ```bash
   cd web
   npm install
   curl -f http://127.0.0.1:9380/api/v1/system/healthz
   ```

자세한 내용은 [소스에서 서비스 시작](./docs/develop/launch_ragflow_from_source.md)을 참조하세요.


> **운영 참고:** Elasticsearch 설정을 영구 적용하려면 `/etc/sysctl.conf`에 `vm.max_map_count=262144`를 추가하세요. 기본 MySQL 구성에서는 Docker 엔트리포인트가 마이그레이션 후 Admin, Syncer, Ingestor, API를 시작합니다. 오픈 소스 1.0의 DeepDoc은 레이아웃, OCR, 표 인식에 CPU 추론을 사용합니다. Docker 설정 변경 후 재시작과 데이터 유지·삭제는 [Docker 가이드](./docker/README.md)를 따르세요. `download_deps.py`는 네이티브 라이브러리와 모델을 준비하며 `requests`와 `huggingface-hub`를 사용합니다. `--migrate`는 마이그레이션 후 종료하고 나머지 옵션은 해당 서비스를 시작합니다. HTTP 200은 API 준비 완료를 뜻합니다.

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
