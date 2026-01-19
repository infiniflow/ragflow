<div align="center">
<a href="https://demo.ragflow.io/">
<img src="web/src/assets/logo-with-text.svg" width="520" alt="ragflow logo">
</a>
</div>

<p align="center">
  <a href="./README.md"><img alt="README in English" src="https://img.shields.io/badge/English-DFE0E5"></a>
  <a href="./README_zh.md"><img alt="简体中文版自述文件" src="https://img.shields.io/badge/简体中文-DFE0E5"></a>
  <a href="./README_tzh.md"><img alt="繁體版中文自述文件" src="https://img.shields.io/badge/繁體中文-DFE0E5"></a>
  <a href="./README_ja.md"><img alt="日本語のREADME" src="https://img.shields.io/badge/日本語-DFE0E5"></a>
  <a href="./README_ko.md"><img alt="한국어" src="https://img.shields.io/badge/한국어-DFE0E5"></a>
  <a href="./README_id.md"><img alt="Bahasa Indonesia" src="https://img.shields.io/badge/Bahasa Indonesia-DFE0E5"></a>
  <a href="./README_pt_br.md"><img alt="Português(Brasil)" src="https://img.shields.io/badge/Português(Brasil)-DBEDFA"></a>
</p>

<p align="center">
    <a href="https://x.com/intent/follow?screen_name=infiniflowai" target="_blank">
        <img src="https://img.shields.io/twitter/follow/infiniflow?logo=X&color=%20%23f5f5f5" alt="seguir no X(Twitter)">
    </a>
    <a href="https://demo.ragflow.io" target="_blank">
        <img alt="Badge Estático" src="https://img.shields.io/badge/Online-Demo-4e6b99">
    </a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/docker/pulls/infiniflow/ragflow?label=Docker%20Pulls&color=0db7ed&logo=docker&logoColor=white&style=flat-square" alt="docker pull infiniflow/ragflow:v0.23.1">
    </a>
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=Última%20Relese" alt="Última Versão">
    </a>
    <a href="https://github.com/infiniflow/ragflow/blob/main/LICENSE">
        <img height="21" src="https://img.shields.io/badge/License-Apache--2.0-ffffff?labelColor=d4eaf7&color=2e6cc4" alt="licença">
    </a>
    <a href="https://deepwiki.com/infiniflow/ragflow">
        <img alt="Ask DeepWiki" src="https://deepwiki.com/badge.svg">
    </a>
</p>

<h4 align="center">
  <a href="https://ragflow.io/docs/dev/">Documentação</a> |
  <a href="https://github.com/infiniflow/ragflow/issues/12241">Roadmap</a> |
  <a href="https://twitter.com/infiniflowai">Twitter</a> |
  <a href="https://discord.gg/NjYzJD3GM3">Discord</a> |
  <a href="https://demo.ragflow.io">Demo</a>
</h4>

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/ragflow-octoverse.png" width="1200"/>
</div>

<div align="center">
<a href="https://trendshift.io/repositories/9064" target="_blank"><img src="https://trendshift.io/api/badge/repositories/9064" alt="infiniflow%2Fragflow | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/></a>
</div>

<details open>
<summary><b>📕 Índice</b></summary>

- 💡 [O que é o RAGFlow?](#-o-que-é-o-ragflow)
- 🎮 [Demo](#-demo)
- 📌 [Últimas Atualizações](#-últimas-atualizações)
- 🌟 [Principais Funcionalidades](#-principais-funcionalidades)
- 🔎 [Arquitetura do Sistema](#-arquitetura-do-sistema)
- 🎬 [Primeiros Passos](#-primeiros-passos)
- 🔧 [Configurações](#-configurações)
- 🔧 [Construir uma imagem docker sem incorporar modelos](#-construir-uma-imagem-docker-sem-incorporar-modelos)
- 🔧 [Construir uma imagem docker incluindo modelos](#-construir-uma-imagem-docker-incluindo-modelos)
- 🔨 [Lançar serviço a partir do código-fonte para desenvolvimento](#-lançar-serviço-a-partir-do-código-fonte-para-desenvolvimento)
- 📚 [Documentação](#-documentação)
- 📜 [Roadmap](#-roadmap)
- 🏄 [Comunidade](#-comunidade)
- 🙌 [Contribuindo](#-contribuindo)

</details>

## 💡 O que é o RAGFlow?

[RAGFlow](https://ragflow.io/) é um mecanismo de [RAG](https://ragflow.io/basics/what-is-rag) (Retrieval-Augmented Generation) open-source líder que fusiona tecnologias RAG de ponta com funcionalidades Agent para criar uma camada contextual superior para LLMs. Oferece um fluxo de trabalho RAG otimizado adaptável a empresas de qualquer escala. Alimentado por [um motor de contexto](https://ragflow.io/basics/what-is-agent-context-engine) convergente e modelos Agent pré-construídos, o RAGFlow permite que desenvolvedores transformem dados complexos em sistemas de IA de alta fidelidade e pronto para produção com excepcional eficiência e precisão.

## 🎮 Demo

Experimente nossa demo em [https://demo.ragflow.io](https://demo.ragflow.io).

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/chunking.gif" width="1200"/>
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/agentic-dark.gif" width="1200"/>
</div>

## 🔥 Últimas Atualizações

- 26-12-2025 Suporte à função 'Memória' para agentes de IA.
- 19-11-2025 Suporta Gemini 3 Pro.
- 12-11-2025 Suporta a sincronização de dados do Confluence, S3, Notion, Discord e Google Drive.
- 23-10-2025 Suporta MinerU e Docling como métodos de análise de documentos.
- 15-10-2025 Suporte para pipelines de dados orquestrados.
- 08-08-2025 Suporta a mais recente série GPT-5 da OpenAI.
- 01-08-2025 Suporta fluxo de trabalho agente e MCP.
- 23-05-2025 Adicione o componente executor de código Python/JS ao Agente.
- 05-05-2025 Suporte a consultas entre idiomas.
- 19-03-2025 Suporta o uso de um modelo multi-modal para entender imagens dentro de arquivos PDF ou DOCX.
- 18-12-2024 Atualiza o modelo de Análise de Layout de Documentos no DeepDoc.
- 22-08-2024 Suporta conversão de texto para comandos SQL via RAG.

## 🎉 Fique Ligado

⭐️ Dê uma estrela no nosso repositório para se manter atualizado com novas funcionalidades e melhorias empolgantes! Receba notificações instantâneas sobre novos lançamentos! 🌟

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/user-attachments/assets/18c9707e-b8aa-4caf-a154-037089c105ba" width="1200"/>
</div>

## 🌟 Principais Funcionalidades

### 🍭 **"Qualidade entra, qualidade sai"**

- Extração de conhecimento baseada em [entendimento profundo de documentos](./deepdoc/README.md) a partir de dados não estruturados com formatos complicados.
- Encontra a "agulha no palheiro de dados" de literalmente tokens ilimitados.

### 🍱 **Fragmentação baseada em templates**

- Inteligente e explicável.
- Muitas opções de templates para escolher.

### 🌱 **Citações fundamentadas com menos alucinações**

- Visualização da fragmentação de texto para permitir intervenção humana.
- Visualização rápida das referências chave e citações rastreáveis para apoiar respostas fundamentadas.

### 🍔 **Compatibilidade com fontes de dados heterogêneas**

- Suporta Word, apresentações, excel, txt, imagens, cópias digitalizadas, dados estruturados, páginas da web e mais.

### 🛀 **Fluxo de trabalho RAG automatizado e sem esforço**

- Orquestração RAG simplificada voltada tanto para negócios pessoais quanto grandes empresas.
- Modelos LLM e de incorporação configuráveis.
- Múltiplas recuperações emparelhadas com reclassificação fundida.
- APIs intuitivas para integração sem problemas com os negócios.

## 🔎 Arquitetura do Sistema

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/user-attachments/assets/31b0dd6f-ca4f-445a-9457-70cb44a381b2" width="1000"/>
</div>

## 🎬 Primeiros Passos

### 📝 Pré-requisitos

- CPU >= 4 núcleos
- RAM >= 16 GB
- Disco >= 50 GB
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
- [gVisor](https://gvisor.dev/docs/user_guide/install/): Necessário apenas se você pretende usar o recurso de executor de código (sandbox) do RAGFlow.

> [!TIP]
> Se você não instalou o Docker na sua máquina local (Windows, Mac ou Linux), veja [Instalar Docker Engine](https://docs.docker.com/engine/install/).

### 🚀 Iniciar o servidor

1. Certifique-se de que `vm.max_map_count` >= 262144:

   > Para verificar o valor de `vm.max_map_count`:
   >
   > ```bash
   > $ sysctl vm.max_map_count
   > ```
   >
   > Se necessário, redefina `vm.max_map_count` para um valor de pelo menos 262144:
   >
   > ```bash
   > # Neste caso, defina para 262144:
   > $ sudo sysctl -w vm.max_map_count=262144
   > ```
   >
   > Essa mudança será resetada após a reinicialização do sistema. Para garantir que a alteração permaneça permanente, adicione ou atualize o valor de `vm.max_map_count` em **/etc/sysctl.conf**:
   >
   > ```bash
   > vm.max_map_count=262144
   > ```
   >
2. Clone o repositório:

   ```bash
   $ git clone https://github.com/infiniflow/ragflow.git
   ```
3. Inicie o servidor usando as imagens Docker pré-compiladas:

> [!CAUTION]
> Todas as imagens Docker são construídas para plataformas x86. Atualmente, não oferecemos imagens Docker para ARM64.
> Se você estiver usando uma plataforma ARM64, por favor, utilize [este guia](https://ragflow.io/docs/dev/build_docker_image) para construir uma imagem Docker compatível com o seu sistema.

    > O comando abaixo baixa a edição`v0.23.1` da imagem Docker do RAGFlow. Consulte a tabela a seguir para descrições de diferentes edições do RAGFlow. Para baixar uma edição do RAGFlow diferente da `v0.23.1`, atualize a variável `RAGFLOW_IMAGE` conforme necessário no **docker/.env** antes de usar `docker compose` para iniciar o servidor.

```bash
   $ cd ragflow/docker

   # git checkout v0.23.1
   # Opcional: use uma tag estável (veja releases: https://github.com/infiniflow/ragflow/releases)
   # Esta etapa garante que o arquivo entrypoint.sh no código corresponda à versão da imagem do Docker.

   # Use CPU for DeepDoc tasks:
   $ docker compose -f docker-compose.yml up -d

   # To use GPU to accelerate DeepDoc tasks:
   # sed -i '1i DEVICE=gpu' .env
   # docker compose -f docker-compose.yml up -d
```

> Nota: Antes da `v0.22.0`, fornecíamos imagens com modelos de embedding e imagens slim sem modelos de embedding. Detalhes a seguir:

| RAGFlow image tag | Image size (GB) | Has embedding models? | Stable?        |
|-------------------|-----------------|-----------------------|----------------|
| v0.21.1           | &approx;9       | ✔️                    | Stable release |
| v0.21.1-slim      | &approx;2       | ❌                     | Stable release |

> A partir da `v0.22.0`, distribuímos apenas a edição slim e não adicionamos mais o sufixo **-slim** às tags das imagens.

4. Verifique o status do servidor após tê-lo iniciado:

   ```bash
   $ docker logs -f docker-ragflow-cpu-1
   ```

   _O seguinte resultado confirma o lançamento bem-sucedido do sistema:_

   ```bash
        ____   ___    ______ ______ __
       / __ \ /   |  / ____// ____// /____  _      __
      / /_/ // /| | / / __ / /_   / // __ \| | /| / /
     / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ /
    /_/ |_|/_/  |_|\____//_/    /_/ \____/ |__/|__/

    * Rodando em todos os endereços (0.0.0.0)
   ```

   > Se você pular essa etapa de confirmação e acessar diretamente o RAGFlow, seu navegador pode exibir um erro `network abnormal`, pois, nesse momento, seu RAGFlow pode não estar totalmente inicializado.
   >
5. No seu navegador, insira o endereço IP do seu servidor e faça login no RAGFlow.

   > Com as configurações padrão, você só precisa digitar `http://IP_DO_SEU_MÁQUINA` (**sem** o número da porta), pois a porta HTTP padrão `80` pode ser omitida ao usar as configurações padrão.
   >
6. Em [service_conf.yaml.template](./docker/service_conf.yaml.template), selecione a fábrica LLM desejada em `user_default_llm` e atualize o campo `API_KEY` com a chave de API correspondente.

   > Consulte [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup) para mais informações.
   >

_O show está no ar!_

## 🔧 Configurações

Quando se trata de configurações do sistema, você precisará gerenciar os seguintes arquivos:

- [.env](./docker/.env): Contém as configurações fundamentais para o sistema, como `SVR_HTTP_PORT`, `MYSQL_PASSWORD` e `MINIO_PASSWORD`.
- [service_conf.yaml.template](./docker/service_conf.yaml.template): Configura os serviços de back-end. As variáveis de ambiente neste arquivo serão automaticamente preenchidas quando o contêiner Docker for iniciado. Quaisquer variáveis de ambiente definidas dentro do contêiner Docker estarão disponíveis para uso, permitindo personalizar o comportamento do serviço com base no ambiente de implantação.
- [docker-compose.yml](./docker/docker-compose.yml): O sistema depende do [docker-compose.yml](./docker/docker-compose.yml) para iniciar.

> O arquivo [./docker/README](./docker/README.md) fornece uma descrição detalhada das configurações do ambiente e dos serviços, que podem ser usadas como `${ENV_VARS}` no arquivo [service_conf.yaml.template](./docker/service_conf.yaml.template).

Para atualizar a porta HTTP de serviço padrão (80), vá até [docker-compose.yml](./docker/docker-compose.yml) e altere `80:80` para `<SUA_PORTA_DE_SERVIÇO>:80`.

Atualizações nas configurações acima exigem um reinício de todos os contêineres para que tenham efeito:

> ```bash
> $ docker compose -f docker-compose.yml up -d
> ```

### Mudar o mecanismo de documentos de Elasticsearch para Infinity

O RAGFlow usa o Elasticsearch por padrão para armazenar texto completo e vetores. Para mudar para o [Infinity](https://github.com/infiniflow/infinity/), siga estas etapas:

1. Pare todos os contêineres em execução:

   ```bash
   $ docker compose -f docker/docker-compose.yml down -v
   ```

   Note: `-v` irá deletar os volumes do contêiner, e os dados existentes serão apagados.
2. Defina `DOC_ENGINE` no **docker/.env** para `infinity`.
3. Inicie os contêineres:

   ```bash
   $ docker compose -f docker-compose.yml up -d
   ```

> [!ATENÇÃO]
 > A mudança para o Infinity em uma máquina Linux/arm64 ainda não é oficialmente suportada.

## 🔧 Criar uma imagem Docker

Esta imagem tem cerca de 2 GB de tamanho e depende de serviços externos de LLM e incorporação.

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
docker build --platform linux/amd64 -f Dockerfile -t infiniflow/ragflow:nightly .
```

Se você estiver atrás de um proxy, pode passar argumentos de proxy:

```bash
docker build --platform linux/amd64 \
  --build-arg http_proxy=http://YOUR_PROXY:PORT \
  --build-arg https_proxy=http://YOUR_PROXY:PORT \
  -f Dockerfile -t infiniflow/ragflow:nightly .
```

## 🔨 Lançar o serviço a partir do código-fonte para desenvolvimento

1. Instale o `uv` e o `pre-commit`, ou pule esta etapa se eles já estiverem instalados:

   ```bash
   pipx install uv pre-commit
   ```
2. Clone o código-fonte e instale as dependências Python:

   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   uv sync --python 3.12 # instala os módulos Python dependentes do RAGFlow
   uv run download_deps.py
   pre-commit install
   ```
3. Inicie os serviços dependentes (MinIO, Elasticsearch, Redis e MySQL) usando Docker Compose:

   ```bash
   docker compose -f docker/docker-compose-base.yml up -d
   ```

   Adicione a seguinte linha ao arquivo `/etc/hosts` para resolver todos os hosts especificados em **docker/.env** para `127.0.0.1`:

   ```
   127.0.0.1       es01 infinity mysql minio redis sandbox-executor-manager
   ```
4. Se não conseguir acessar o HuggingFace, defina a variável de ambiente `HF_ENDPOINT` para usar um site espelho:

   ```bash
   export HF_ENDPOINT=https://hf-mirror.com
   ```
5. Se o seu sistema operacional não tiver jemalloc, instale-o da seguinte maneira:

   ```bash
   # ubuntu
   sudo apt-get install libjemalloc-dev
   # centos
   sudo yum instalar jemalloc
   # mac
   sudo brew install jemalloc
   ```
6. Lance o serviço de back-end:

   ```bash
   source .venv/bin/activate
   export PYTHONPATH=$(pwd)
   bash docker/launch_backend_service.sh
   ```
7. Instale as dependências do front-end:

   ```bash
   cd web
   npm install
   ```
8. Lance o serviço de front-end:

   ```bash
   npm run dev
   ```

   _O seguinte resultado confirma o lançamento bem-sucedido do sistema:_

   ![](https://github.com/user-attachments/assets/0daf462c-a24d-4496-a66f-92533534e187)
9. Pare os serviços de front-end e back-end do RAGFlow após a conclusão do desenvolvimento:

   ```bash
   pkill -f "ragflow_server.py|task_executor.py"
   ```

## 📚 Documentação

- [Quickstart](https://ragflow.io/docs/dev/)
- [Configuration](https://ragflow.io/docs/dev/configurations)
- [Release notes](https://ragflow.io/docs/dev/release_notes)
- [User guides](https://ragflow.io/docs/dev/category/guides)
- [Developer guides](https://ragflow.io/docs/dev/category/developers)
- [References](https://ragflow.io/docs/dev/category/references)
- [FAQs](https://ragflow.io/docs/dev/faq)

## 📜 Roadmap

Veja o [RAGFlow Roadmap 2026](https://github.com/infiniflow/ragflow/issues/12241)

## 🏄 Comunidade

- [Discord](https://discord.gg/NjYzJD3GM3)
- [Twitter](https://twitter.com/infiniflowai)
- [GitHub Discussions](https://github.com/orgs/infiniflow/discussions)

## 🙌 Contribuindo

O RAGFlow prospera por meio da colaboração de código aberto. Com esse espírito, abraçamos contribuições diversas da comunidade.
Se você deseja fazer parte, primeiro revise nossas [Diretrizes de Contribuição](https://ragflow.io/docs/dev/contributing).
