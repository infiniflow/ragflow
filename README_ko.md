<div align="center">
<a href="https://demo.ragflow.io/">
<img src="web/src/assets/logo-with-text.png" width="520" alt="ragflow logo">
</a>
</div>

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README_zh.md">简体中文</a> |
  <a href="./README_ja.md">日本語</a> |
  <a href="./README_ko.md">한국어</a> |
</p>

<p align="center">
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=Latest%20Release" alt="Latest Release">
    </a>
    <a href="https://demo.ragflow.io" target="_blank">
        <img alt="Static Badge" src="https://img.shields.io/badge/Online-Demo-4e6b99"></a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/badge/docker_pull-ragflow:v0.12.0-brightgreen" alt="docker pull infiniflow/ragflow:v0.12.0"></a>
    <a href="https://github.com/infiniflow/ragflow/blob/main/LICENSE">
    <img height="21" src="https://img.shields.io/badge/License-Apache--2.0-ffffff?labelColor=d4eaf7&color=2e6cc4" alt="license">
  </a>
</p>

<h4 align="center">
  <a href="https://ragflow.io/docs/dev/">Document</a> |
  <a href="https://github.com/infiniflow/ragflow/issues/162">Roadmap</a> |
  <a href="https://twitter.com/infiniflowai">Twitter</a> |
  <a href="https://discord.gg/4XxujFgUN7">Discord</a> |
  <a href="https://demo.ragflow.io">Demo</a>
</h4>


## 💡 RAGFlow란?

[RAGFlow](https://ragflow.io/)는 심층 문서 이해에 기반한 오픈소스 RAG (Retrieval-Augmented Generation) 엔진입니다. 이 엔진은 대규모 언어 모델(LLM)과 결합하여 정확한 질문 응답 기능을 제공하며, 다양한 복잡한 형식의 데이터에서 신뢰할 수 있는 출처를 바탕으로 한 인용을 통해 이를 뒷받침합니다. RAGFlow는 규모에 상관없이 모든 기업에 최적화된 RAG 워크플로우를 제공합니다.



## 🎮 데모
데모를 [https://demo.ragflow.io](https://demo.ragflow.io)에서 실행해 보세요.
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/7248/2f6baa3e-1092-4f11-866d-36f6a9d075e5" width="1200"/>
<img src="https://github.com/infiniflow/ragflow/assets/12318111/b083d173-dadc-4ea9-bdeb-180d7df514eb" width="1200"/>
</div>


## 🔥 업데이트

- 2024-09-29 다단계 대화를 최적화합니다.
  
- 2024-09-13 지식베이스 Q&A 검색 모드를 추가합니다.
  
- 2024-09-09 Agent에 의료상담 템플릿을 추가하였습니다.
  
- 2024-08-22 RAG를 통해 SQL 문에 텍스트를 지원합니다.
  
- 2024-08-02: [graphrag](https://github.com/microsoft/graphrag)와 마인드맵에서 영감을 받은 GraphRAG를 지원합니다.

- 2024-07-23: 오디오 파일 분석을 지원합니다.

- 2024-07-08: [Graph](./agent/README.md)를 기반으로 한 워크플로우를 지원합니다.

- 2024-06-27 Q&A 구문 분석 방식에서 Markdown 및 Docx를 지원하고, Docx 파일에서 이미지 추출, Markdown 파일에서 테이블 추출을 지원합니다.

- 2024-05-23: 더 나은 텍스트 검색을 위해 [RAPTOR](https://arxiv.org/html/2401.18059v1)를 지원합니다.



## 🌟 주요 기능

### 🍭 **"Quality in, quality out"**
- [심층 문서 이해](./deepdoc/README.md)를 기반으로 복잡한 형식의 비정형 데이터에서 지식을 추출합니다.
- 문자 그대로 무한한 토큰에서 "데이터 속의 바늘"을 찾아냅니다.

### 🍱 **템플릿 기반의 chunking**
- 똑똑하고 설명 가능한 방식.
- 다양한 템플릿 옵션을 제공합니다.


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
<img src="https://github.com/infiniflow/ragflow/assets/12318111/d6ac5664-c237-4200-a7c2-a4a00691b485" width="1000"/>
</div>

## 🎬 시작하기
### 📝 사전 준비 사항
- CPU >= 4 cores
- RAM >= 16 GB
- Disk >= 50 GB
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
  > 로컬 머신(Windows, Mac, Linux)에 Docker가 설치되지 않은 경우, [Docker 엔진 설치]((https://docs.docker.com/engine/install/))를 참조하세요.


### 🚀 서버 시작하기

1. `vm.max_map_count`가 262144 이상인지 확인하세요:
   > `vm.max_map_count`의 값을 아래 명령어를 통해 확인하세요:
   >
   > ```bash
   > $ sysctl vm.max_map_count
   > ```
   >
   > 만약 `vm.max_map_count` 이 262144 보다 작다면 값을 쟈설정하세요.
   >
   > ```bash
   > # 이 경우에 262144로 설정했습니다.:
   > $ sudo sysctl -w vm.max_map_count=262144
   > ```
   >
   > 이 변경 사항은 시스템 재부팅 후에 초기화됩니다. 변경 사항을 영구적으로 적용하려면 /etc/sysctl.conf 파일에 vm.max_map_count 값을 추가하거나 업데이트하세요:
   >
   > ```bash
   > vm.max_map_count=262144
   > ```

2. 레포지토리를 클론하세요:

   ```bash
   $ git clone https://github.com/infiniflow/ragflow.git
   ```

3. 미리 빌드된 Docker 이미지를 생성하고 서버를 시작하세요:

   > 다음 명령어를 실행하면 *dev* 버전의 RAGFlow Docker 이미지가 자동으로 다운로드됩니다. 특정 Docker 버전을 다운로드하고 실행하려면, **docker/.env** 파일에서 `RAGFLOW_VERSION`을 원하는 버전으로 업데이트한 후, 예를 들어 `RAGFLOW_VERSION=v0.12.0`로 업데이트 한 뒤, 다음 명령어를 실행하세요.
   ```bash
   $ cd ragflow/docker
   $ chmod +x ./entrypoint.sh
   $ docker compose up -d
   ```
   
   > 기본 이미지는 약 9GB 크기이며 로드하는 데 시간이 걸릴 수 있습니다.


4. 서버가 시작된 후 서버 상태를 확인하세요:

   ```bash
   $ docker logs -f ragflow-server
   ```

   _다음 출력 결과로 시스템이 성공적으로 시작되었음을 확인합니다:_

   ```bash
        ____   ___    ______ ______ __               
       / __ \ /   |  / ____// ____// /____  _      __
      / /_/ // /| | / / __ / /_   / // __ \| | /| / /
     / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ / 
    /_/ |_|/_/  |_|\____//_/    /_/ \____/ |__/|__/  

    * Running on all addresses (0.0.0.0)
    * Running on http://127.0.0.1:9380
    * Running on http://x.x.x.x:9380
    INFO:werkzeug:Press CTRL+C to quit
   ```
   > 만약 확인 단계를 건너뛰고 바로 RAGFlow에 로그인하면, RAGFlow가 완전히 초기화되지 않았기 때문에 브라우저에서 `network abnormal` 오류가 발생할 수 있습니다.

5. 웹 브라우저에 서버의 IP 주소를 입력하고 RAGFlow에 로그인하세요.
   > 기본 설정을 사용할 경우, `http://IP_OF_YOUR_MACHINE`만 입력하면 됩니다 (포트 번호는 제외). 기본 HTTP 서비스 포트 `80`은 기본 구성으로 사용할 때 생략할 수 있습니다.
6. [service_conf.yaml](./docker/service_conf.yaml) 파일에서 원하는 LLM 팩토리를 `user_default_llm`에 선택하고, `API_KEY` 필드를 해당 API 키로 업데이트하세요.
   > 자세한 내용은 [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup)를 참조하세요.

   _이제 쇼가 시작됩니다!_

## 🔧 설정

시스템 설정과 관련하여 다음 파일들을 관리해야 합니다:

- [.env](./docker/.env): `SVR_HTTP_PORT`, `MYSQL_PASSWORD`, `MINIO_PASSWORD`와 같은 시스템의 기본 설정을 포함합니다.
- [service_conf.yaml](./docker/service_conf.yaml): 백엔드 서비스를 구성합니다.
- [docker-compose.yml](./docker/docker-compose.yml): 시스템은 [docker-compose.yml](./docker/docker-compose.yml)을 사용하여 시작됩니다.

[.env](./docker/.env) 파일의 변경 사항이 [service_conf.yaml](./docker/service_conf.yaml) 파일의 내용과 일치하도록 해야 합니다.

> [./docker/README](./docker/README.md) 파일에는 환경 설정과 서비스 구성에 대한 자세한 설명이 있으며, [./docker/README](./docker/README.md) 파일에 나열된 모든 환경 설정이 [service_conf.yaml](./docker/service_conf.yaml) 파일의 해당 구성과 일치하도록 해야 합니다.

기본 HTTP 서비스 포트(80)를 업데이트하려면 [docker-compose.yml](./docker/docker-compose.yml) 파일에서 `80:80`을 `<YOUR_SERVING_PORT>:80`으로 변경하세요.

> 모든 시스템 구성 업데이트는 적용되기 위해 시스템 재부팅이 필요합니다.
>
> ```bash
> $ docker-compose up -d
> ```

## 🪛 Build the Docker image without embedding models

This image is approximately 1 GB in size and relies on external LLM and embedding services.

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
pip3 install huggingface-hub
python3 download_deps.py
docker build -f Dockerfile.slim -t infiniflow/ragflow:dev-slim .
```

## 🪚 Build the Docker image including embedding models

This image includes embedding models and is approximately 9 GB in size, and so relies on external LLM services only.

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
pip3 install huggingface-hub
python3 download_deps.py
docker build -f Dockerfile -t infiniflow/ragflow:dev .
```

## 🔨 Launch service from source for development

1. Install Poetry, or skip this step if it is already installed:
   ```bash
   curl -sSL https://install.python-poetry.org | python3 -
   ```

2. Clone the source code and install Python dependencies:
   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   export POETRY_VIRTUALENVS_CREATE=true POETRY_VIRTUALENVS_IN_PROJECT=true
   ~/.local/bin/poetry install --sync --no-root # install RAGFlow dependent python modules
   ```

3. Launch the dependent services (MinIO, Elasticsearch, Redis, and MySQL) using Docker Compose:
   ```bash
   docker compose -f docker/docker-compose-base.yml up -d
   ```

   Add the following line to `/etc/hosts` to resolve all hosts specified in **docker/service_conf.yaml** to `127.0.0.1`:  
   ```
   127.0.0.1       es01 mysql minio redis
   ```  
   In **docker/service_conf.yaml**, update mysql port to `5455` and es port to `1200`, as specified in **docker/.env**.

4. If you cannot access HuggingFace, set the `HF_ENDPOINT` environment variable to use a mirror site:
 
   ```bash
   export HF_ENDPOINT=https://hf-mirror.com
   ```

5. Launch backend service:
   ```bash
   source .venv/bin/activate
   export PYTHONPATH=$(pwd)
   bash docker/launch_backend_service.sh
   ```

6. Install frontend dependencies:  
   ```bash
   cd web
   npm install --force
   ```  
7. Configure frontend to update `proxy.target` in **.umirc.ts** to `http://127.0.0.1:9380`:
8. Launch frontend service:  
   ```bash
   npm run dev 
   ```

   _The following output confirms a successful launch of the system:_  

   ![](https://github.com/user-attachments/assets/0daf462c-a24d-4496-a66f-92533534e187)

## 📚 문서

- [Quickstart](https://ragflow.io/docs/dev/)
- [User guide](https://ragflow.io/docs/dev/category/user-guides)
- [References](https://ragflow.io/docs/dev/category/references)
- [FAQ](https://ragflow.io/docs/dev/faq)

## 📜 로드맵

[RAGFlow 로드맵 2024](https://github.com/infiniflow/ragflow/issues/162)을 확인하세요.

## 🏄 커뮤니티

- [Discord](https://discord.gg/4XxujFgUN7)
- [Twitter](https://twitter.com/infiniflowai)
- [GitHub Discussions](https://github.com/orgs/infiniflow/discussions)

## 🙌 컨트리뷰션

RAGFlow는 오픈소스 협업을 통해 발전합니다. 이러한 정신을 바탕으로, 우리는 커뮤니티의 다양한 기여를 환영합니다. 참여하고 싶으시다면, 먼저 [가이드라인](./CONTRIBUTING.md)을 검토해 주세요.
