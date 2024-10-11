<div align="center">
<a href="https://demo.ragflow.io/">
<img src="web/src/assets/logo-with-text.png" width="350" alt="ragflow logo">
</a>
</div>

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README_zh.md">简体中文</a> |
  <a href="./README_ja.md">日本語</a> |
  <a href="./README_ko.md">한국어</a>
</p>

<p align="center">
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=Latest%20Release" alt="Latest Release">
    </a>
    <a href="https://demo.ragflow.io" target="_blank">
        <img alt="Static Badge" src="https://img.shields.io/badge/Online-Demo-4e6b99"></a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/badge/docker_pull-ragflow:v0.12.0-brightgreen"
            alt="docker pull infiniflow/ragflow:v0.12.0"></a>
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

## 💡 RAGFlow とは？

[RAGFlow](https://ragflow.io/) は、深い文書理解に基づいたオープンソースの RAG (Retrieval-Augmented Generation) エンジンである。LLM（大規模言語モデル）を組み合わせることで、様々な複雑なフォーマットのデータから根拠のある引用に裏打ちされた、信頼できる質問応答機能を実現し、あらゆる規模のビジネスに適した RAG ワークフローを提供します。

## 🎮 Demo

デモをお試しください：[https://demo.ragflow.io](https://demo.ragflow.io)。
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/7248/2f6baa3e-1092-4f11-866d-36f6a9d075e5" width="1200"/>
<img src="https://github.com/infiniflow/ragflow/assets/12318111/b083d173-dadc-4ea9-bdeb-180d7df514eb" width="1200"/>
</div>


## 🔥 最新情報

- 2024-09-29 マルチラウンドダイアログを最適化。
- 2024-09-13 ナレッジベース Q&A の検索モードを追加しました。
- 2024-09-09 エージェントに医療相談テンプレートを追加しました。
- 2024-08-22 RAG を介して SQL ステートメントへのテキストをサポートします。
- 2024-08-02 [graphrag](https://github.com/microsoft/graphrag) からインスピレーションを得た GraphRAG とマインド マップをサポートします。

## 🎉 続きを楽しみに
⭐️ リポジトリをスター登録して、エキサイティングな新機能やアップデートを最新の状態に保ちましょう！すべての新しいリリースに関する即時通知を受け取れます！ 🌟
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/user-attachments/assets/18c9707e-b8aa-4caf-a154-037089c105ba" width="1200"/>
</div>

## 🌟 主な特徴

### 🍭 **"Quality in, quality out"**

- 複雑な形式の非構造化データからの[深い文書理解](./deepdoc/README.md)ベースの知識抽出。
- 無限のトークンから"干し草の山の中の針"を見つける。

### 🍱 **テンプレートベースのチャンク化**

- 知的で解釈しやすい。
- テンプレートオプションが豊富。

### 🌱 **ハルシネーションが軽減された根拠のある引用**

- 可視化されたテキストチャンキング（text chunking）で人間の介入を可能にする。
- 重要な参考文献のクイックビューと、追跡可能な引用によって根拠ある答えをサポートする。

### 🍔 **多様なデータソースとの互換性**

- Word、スライド、Excel、txt、画像、スキャンコピー、構造化データ、Web ページなどをサポート。

### 🛀 **自動化された楽な RAG ワークフロー**

- 個人から大企業まで対応できる RAG オーケストレーション（orchestration）。
- カスタマイズ可能な LLM とエンベッディングモデル。
- 複数の想起と融合された再ランク付け。
- 直感的な API によってビジネスとの統合がシームレスに。

## 🔎 システム構成

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/d6ac5664-c237-4200-a7c2-a4a00691b485" width="1000"/>
</div>

## 🎬 初期設定

### 📝 必要条件

- CPU >= 4 cores
- RAM >= 16 GB
- Disk >= 50 GB
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
  > ローカルマシン（Windows、Mac、または Linux）に Docker をインストールしていない場合は、[Docker Engine のインストール](https://docs.docker.com/engine/install/) を参照してください。

### 🚀 サーバーを起動

1. `vm.max_map_count` >= 262144 であることを確認する:

   > `vm.max_map_count` の値をチェックするには:
   >
   > ```bash
   > $ sysctl vm.max_map_count
   > ```
   >
   > `vm.max_map_count` が 262144 より大きい値でなければリセットする。
   >
   > ```bash
   > # In this case, we set it to 262144:
   > $ sudo sysctl -w vm.max_map_count=262144
   > ```
   >
   > この変更はシステム再起動後にリセットされる。変更を恒久的なものにするには、**/etc/sysctl.conf** の `vm.max_map_count` 値を適宜追加または更新する:
   >
   > ```bash
   > vm.max_map_count=262144
   > ```

2. リポジトリをクローンする:

   ```bash
   $ git clone https://github.com/infiniflow/ragflow.git
   ```

3. ビルド済みの Docker イメージをビルドし、サーバーを起動する:

   > 以下のコマンドは、RAGFlow slim（`dev-slim`）の開発版Dockerイメージをダウンロードします。RAGFlow slimのDockerイメージには、埋め込みモデルやPythonライブラリが含まれていないため、サイズは約1GBです。

   ```bash
   $ cd ragflow/docker
   $ docker compose -f docker-compose.yml up -d
   ```

   > - 特定のバージョンのRAGFlow slim Dockerイメージをダウンロードするには、**docker/.env**内の`RAGFlow_IMAGE`変数を希望のバージョンに更新します。例えば、`RAGFLOW_IMAGE=infiniflow/ragflow:v0.12.0`とします。この変更を行った後、上記のコマンドを再実行してダウンロードを開始してください。
   > - RAGFlowの埋め込みモデルとPythonライブラリを含む開発版Dockerイメージをダウンロードするには、**docker/.env**内の`RAGFlow_IMAGE`変数を`RAGFLOW_IMAGE=infiniflow/ragflow:dev`に更新します。この変更を行った後、上記のコマンドを再実行してダウンロードを開始してください。
   > - 特定のバージョンのRAGFlow Dockerイメージ（埋め込みモデルとPythonライブラリを含む）をダウンロードするには、**docker/.env**内の`RAGFlow_IMAGE`変数を希望のバージョンに更新します。例えば、`RAGFLOW_IMAGE=infiniflow/ragflow:v0.12.0`とします。この変更を行った後、上記のコマンドを再実行してダウンロードを開始してください。  
   
   > **NOTE:** 埋め込みモデルとPythonライブラリを含むRAGFlow Dockerイメージのサイズは約9GBであり、読み込みにかなりの時間がかかる場合があります。

4. サーバーを立ち上げた後、サーバーの状態を確認する:

   ```bash
   $ docker logs -f ragflow-server
   ```

   _以下の出力は、システムが正常に起動したことを確認するものです:_

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
   > もし確認ステップをスキップして直接 RAGFlow にログインした場合、その時点で RAGFlow が完全に初期化されていない可能性があるため、ブラウザーがネットワーク異常エラーを表示するかもしれません。

5. ウェブブラウザで、プロンプトに従ってサーバーの IP アドレスを入力し、RAGFlow にログインします。
   > デフォルトの設定を使用する場合、デフォルトの HTTP サービングポート `80` は省略できるので、与えられたシナリオでは、`http://IP_OF_YOUR_MACHINE`（ポート番号は省略）だけを入力すればよい。
6. [service_conf.yaml](./docker/service_conf.yaml) で、`user_default_llm` で希望の LLM ファクトリを選択し、`API_KEY` フィールドを対応する API キーで更新する。

   > 詳しくは [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup) を参照してください。

   _これで初期設定完了！ショーの開幕です！_

## 🔧 コンフィグ

システムコンフィグに関しては、以下のファイルを管理する必要がある:

- [.env](./docker/.env): `SVR_HTTP_PORT`、`MYSQL_PASSWORD`、`MINIO_PASSWORD` などのシステムの基本設定を保持する。
- [service_conf.yaml](./docker/service_conf.yaml): バックエンドのサービスを設定します。
- [docker-compose.yml](./docker/docker-compose.yml): システムの起動は [docker-compose.yml](./docker/docker-compose.yml) に依存している。

[.env](./docker/.env) ファイルの変更が [service_conf.yaml](./docker/service_conf.yaml) ファイルの内容と一致していることを確認する必要があります。

> [./docker/README](./docker/README.md) ファイルは環境設定とサービスコンフィグの詳細な説明を提供し、[./docker/README](./docker/README.md) ファイルに記載されている全ての環境設定が [service_conf.yaml](./docker/service_conf.yaml) ファイルの対応するコンフィグと一致していることを確認することが義務付けられています。

デフォルトの HTTP サービングポート(80)を更新するには、[docker-compose.yml](./docker/docker-compose.yml) にアクセスして、`80:80` を `<YOUR_SERVING_PORT>:80` に変更します。

> すべてのシステム設定のアップデートを有効にするには、システムの再起動が必要です:
>
> ```bash
> $ docker compose -f docker/docker-compose.yml up -d
> ```

## 🔧 ソースコードでDockerイメージを作成（埋め込みモデルなし）

この Docker イメージのサイズは約 1GB で、外部の大モデルと埋め込みサービスに依存しています。

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
pip3 install huggingface-hub nltk
python3 download_deps.py
docker build -f Dockerfile.slim -t infiniflow/ragflow:dev-slim .
```

## 🔧 ソースコードをコンパイルしたDockerイメージ（埋め込みモデルを含む）

この Docker のサイズは約 9GB で、埋め込みモデルを含むため、外部の大モデルサービスのみが必要です。

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
pip3 install huggingface-hub nltk
python3 download_deps.py
docker build -f Dockerfile -t infiniflow/ragflow:dev .
```

## 🔨 ソースコードからサービスを起動する方法

1. Poetry をインストールする。すでにインストールされている場合は、このステップをスキップしてください:
   ```bash
   curl -sSL https://install.python-poetry.org | python3 -
   ```

2. ソースコードをクローンし、Python の依存関係をインストールする:
   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   export POETRY_VIRTUALENVS_CREATE=true POETRY_VIRTUALENVS_IN_PROJECT=true
   ~/.local/bin/poetry install --sync --no-root # install RAGFlow dependent python modules
   ```

3. Docker Compose を使用して依存サービス（MinIO、Elasticsearch、Redis、MySQL）を起動する:
   ```bash
   docker compose -f docker/docker-compose-base.yml up -d
   ```

   `/etc/hosts` に以下の行を追加して、**docker/service_conf.yaml** に指定されたすべてのホストを `127.0.0.1` に解決します:  
   ```
   127.0.0.1       es01 mysql minio redis
   ```  
   **docker/service_conf.yaml** で mysql のポートを `5455` に、es のポートを `1200` に更新します（**docker/.env** に指定された通り）.

4. HuggingFace にアクセスできない場合は、`HF_ENDPOINT` 環境変数を設定してミラーサイトを使用してください:
 
   ```bash
   export HF_ENDPOINT=https://hf-mirror.com
   ```

5. バックエンドサービスを起動する:
   ```bash
   source .venv/bin/activate
   export PYTHONPATH=$(pwd)
   bash docker/launch_backend_service.sh
   ```

6. フロントエンドの依存関係をインストールする:  
   ```bash
   cd web
   npm install --force
   ```  
7. フロントエンドを設定し、**.umirc.ts** の `proxy.target` を `http://127.0.0.1:9380` に更新します:
8. フロントエンドサービスを起動する:  
   ```bash
   npm run dev 
   ```

   _以下の画面で、システムが正常に起動したことを示します:_  

   ![](https://github.com/user-attachments/assets/0daf462c-a24d-4496-a66f-92533534e187)

## 📚 ドキュメンテーション

- [Quickstart](https://ragflow.io/docs/dev/)
- [User guide](https://ragflow.io/docs/dev/category/guides)
- [References](https://ragflow.io/docs/dev/category/references)
- [FAQ](https://ragflow.io/docs/dev/faq)

## 📜 ロードマップ

[RAGFlow ロードマップ 2024](https://github.com/infiniflow/ragflow/issues/162) を参照

## 🏄 コミュニティ

- [Discord](https://discord.gg/4XxujFgUN7)
- [Twitter](https://twitter.com/infiniflowai)
- [GitHub Discussions](https://github.com/orgs/infiniflow/discussions)

## 🙌 コントリビュート

RAGFlow はオープンソースのコラボレーションによって発展してきました。この精神に基づき、私たちはコミュニティからの多様なコントリビュートを受け入れています。 参加を希望される方は、まず [コントリビューションガイド](./CONTRIBUTING.md)をご覧ください。
