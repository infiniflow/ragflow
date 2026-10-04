<div align="center">
<a href="https://cloud.ragflow.io/">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow/main/web/src/assets/logo-with-text.svg" width="520" alt="ragflow logo">
</a>
</div>

<p align="center">
  <a href="./README.md"><img alt="README in English" src="https://img.shields.io/badge/English-DFE0E5"></a>
  <a href="./README_zh.md"><img alt="简体中文版自述文件" src="https://img.shields.io/badge/简体中文-DFE0E5"></a>
  <a href="./README_tzh.md"><img alt="繁體中文版自述文件" src="https://img.shields.io/badge/繁體中文-DFE0E5"></a>
  <a href="./README_ja.md"><img alt="日本語のREADME" src="https://img.shields.io/badge/日本語-DBEDFA"></a>
  <a href="./README_ko.md"><img alt="한국어" src="https://img.shields.io/badge/한국어-DFE0E5"></a>
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

## 💡 RAGFlow とは？

[RAGFlow](https://ragflow.io/) は、先進的な[RAG](https://ragflow.io/basics/what-is-rag)（Retrieval-Augmented Generation）技術と Agent 機能を融合し、大規模言語モデル（LLM）に優れたコンテキスト層を構築する最先端のオープンソース RAG エンジンです。あらゆる規模の企業に対応可能な合理化された RAG ワークフローを提供し、統合型[コンテキストエンジン](https://ragflow.io/basics/what-is-agent-context-engine)と事前構築されたAgentテンプレートにより、開発者が複雑なデータを驚異的な効率性と精度で高精細なプロダクションレディAIシステムへ変換することを可能にします。

## 🎮 はじめに

当社のクラウドサービスをぜひお試しください：[https://cloud.ragflow.io](https://cloud.ragflow.io)。

ローカルにデプロイする場合は、[セルフホスティング](#-セルフホスティング)を参照してください。

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="Chunking demonstration" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/chunking.gif" width="1200"/>
<img alt="Agentic workflow demonstration" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/agentic-dark.gif" width="1200"/>
</div>

## 🔥 最新情報

- 2026-09-29 RAGFlow 1.0.0-rc1 をリリース。

- 2026-09-10 SitemapによるWebコンテンツの取り込みに対応。
- 2026-08-19 Knowledge Compilationを導入。ドキュメントおよびデータセット単位でWiki、Graph、Tree、PageIndex、Mind Map、Timeline、Skillsを生成できます。
- 2026-08-19 Low、Medium、High、Ultraの思考モードを備えたAgentic RAGを導入。
- 2026-07-02 Google BigQueryデータソースの取り込みと増分同期に対応。
- 2026-06-29 WhatsApp、DingTalk、WeComのチャットチャネルに対応。
- 2026-05-26 AgentがWebページを閲覧・操作できるBrowserコンポーネントを追加。
- 2026-04-21 7種類の組み込みデータ取り込みパイプラインテンプレートを追加。
- 2026-04-21 Agentアプリの公開、Sandboxでのコード実行、グラフ生成に対応。
- 2026-04-21 ユーザー単位のメモリ保存と検索に対応。

その他の更新については[リリースノート全文](./docs/release_notes.md)を参照してください。


## 🎉 続きを楽しみに

⭐️ リポジトリをスター登録して、エキサイティングな新機能やアップデートを最新の状態に保ちましょう！すべての新しいリリースに関する即時通知を受け取れます！ 🌟

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="RAGFlow feature updates" src="https://github.com/user-attachments/assets/18c9707e-b8aa-4caf-a154-037089c105ba" width="1200"/>
</div>

## 🌟 主な特徴

### 🍭 **"Quality in, quality out"**

- 複雑な形式の非構造化データからの深い文書理解ベースの知識抽出。
- 無限のトークンから"干し草の山の中の針"を見つける。

### 🍱 **テンプレートベースのチャンク化**

- 知的で解釈しやすい。
- テンプレートオプションが豊富。

### 🧩 **ナレッジコンパイル（Knowledge Compilation）**

- ドキュメントやデータセットのコンテンツを、Wiki、Graph、Tree、PageIndex、Mind Map、Timeline、Skills などの構造化された成果物に整理します。
- コンパイルモデルと処理ルールを設定し、成果物の表示、更新、再生成を行えます。

### 🧠 **Agentic Retrieval**

- 複雑な質問を分析し、必要に応じて分解、ナレッジ検索、根拠確認を複数段階で行います。
- Low、Medium、High、Ultra の思考モードで、質問の複雑さに応じて検索と推論の深さを調整できます。

### ⚙️ **Go ネイティブサービスアーキテクチャ**

- API、Admin、Ingestor、Syncer は統合された Go サービスが提供します。DeepDoc は Go プロセス内で動作し、レイアウト解析、OCR、表認識を担当します。
- Go サービスは CGO 経由でネイティブ文書解析ライブラリと ONNX Runtime を呼び出します。MCP と Sandbox Executor は必要に応じて有効化できます。

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
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/structure.jpg" alt="RAGFlow システムアーキテクチャ" width="1000" />
</div>

## 🎬 セルフホスティング

### 🐳 Docker デプロイ

#### 📝 Docker デプロイの前提条件

- 推奨する開始時の構成：CPU 4コア、RAM 16 GB、空きディスク容量 50 GB。実際の要件は、ドキュメントエンジン、データ量、解析タスク、同時実行数によって異なります。ローカルモデルやその他のオプションコンポーネントでは、追加のリソースが必要になる場合があります。
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
- [gVisor](https://gvisor.dev/docs/user_guide/install/): Self-ManagedコンテナSandboxを使用する場合のみ必要です。

DockerデプロイではホストへのGoのインストールは不要です。Self-ManagedコンテナSandboxではgVisorのインストールと設定が必要ですが、他のSandboxプロバイダーではRAGFlowホストへのgVisorのインストールは不要です。

> [!TIP]
> ローカルマシン（Windows、Mac、または Linux）に Docker をインストールしていない場合は、[Docker Engine のインストール](https://docs.docker.com/engine/install/) を参照してください。

#### 🚀 サーバーを起動

1. `vm.max_map_count` >= 262144 であることを確認する:

   > `vm.max_map_count` の値をチェックするには:
   >
   > ```bash
   > sysctl vm.max_map_count
   > ```
   >
   > `vm.max_map_count` が 262144 より大きい値でなければリセットする。
   >
   > ```bash
   > # In this case, we set it to 262144:
   > sudo sysctl -w vm.max_map_count=262144
   > ```
   >
   > この変更はシステム再起動後にリセットされる。変更を恒久的なものにするには、**/etc/sysctl.conf** の `vm.max_map_count` 値を適宜追加または更新する:
   >
   > ```bash
   > vm.max_map_count=262144
   > ```
   >
2. リポジトリをクローンする:

   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   ```
3. Go リリースタグに切り替え、Docker Compose で事前ビルド済み Go イメージを起動します:

> [!CAUTION]
> 現在、公式に提供されているすべての Docker イメージは x86 アーキテクチャ向けにビルドされており、ARM64 用の Docker イメージは提供されていません。
> ARM64 アーキテクチャのオペレーティングシステムを使用している場合は、[このドキュメント](https://ragflow.io/docs/dev/build_docker_image)を参照して Docker イメージを自分でビルドしてください。



   Docker デプロイディレクトリに移動します。

   ```bash
   cd ragflow/docker
   ```

   Go v1.0.0-rc1 リリースタグに切り替えます。

   ```bash
   git checkout v1.0.0-rc1
   ```

   Go サービスと依存サービスをバックグラウンドで起動します。

   ```bash
   docker compose -f docker-compose.yml up -d
   ```

   デフォルトの MySQL 構成では、Go イメージのエントリーポイントが最初にデータベース移行を実行し、その後 `bin/ragflow_server` を介して Syncer、Admin、API、Ingestor を起動します。

> RAGFlow オープンソース 1.0 の DeepDoc は、レイアウト解析、OCR、表認識に CPU 推論を使用します。

4. 起動後にサービスの状態と API の準備状況を確認します：

   ```bash
   docker ps
   ```

   上記のコマンドは依存サービスの状態を表示します。RAGFlow 自体には Compose healthcheck が定義されていないため、API で準備状況を確認します：

   ```bash
   curl -f http://localhost/api/v1/system/healthz
   ```

   HTTP 200 レスポンスは準備完了を示します。`SVR_WEB_HTTP_PORT` を変更した場合は、ヘルスチェック URL でそのポートを使用してください。起動に失敗した場合は、`docker logs --tail 50 <service>` で該当サービスのログを確認してください。

5. ウェブブラウザで、プロンプトに従ってサーバーの IP アドレスを入力し、RAGFlow にログインします。

   > デフォルトの設定を使用する場合、デフォルトの HTTP サービングポート `80` は省略できるので、与えられたシナリオでは、`http://IP_OF_YOUR_MACHINE`（ポート番号は省略）だけを入力すればよい。
   >
6. RAGFlow にログインした後、モデルプロバイダーページで LLM、Embedding、Reranker を追加し、モデル名、サービスアドレス、API キーを入力します。

   > 詳しくは [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup) を参照してください。
   >

   _これで初期設定完了！ショーの開幕です！_

#### ⚙️ Docker の設定と調整

Go版のDockerデプロイでは `docker/.env` と `docker/docker-compose.yml` を使用し、キャッシュとCheckpointの保存にKvrocks、メッセージキューにNATS JetStreamを使用します。イメージ、ポート、パスワード、ドキュメントエンジン、モデルイメージの取得元を変更する場合は、[Docker設定ガイド](./docker/README.md)に従ってください。プラットフォームの制限とmacOSの要件については、[Go Dockerイメージのビルドとプラットフォームサポートガイド](./docs/develop/build_docker_image.mdx)を参照してください。

ドキュメントエンジンの切り替え、設定変更後のサービス再起動、既存データの保持または削除についても、上記のDocker設定ガイドに従ってください。

### 🔨 ソースコードからサービスを起動する方法

#### 📝 ソースビルドの前提条件

1. `go.mod` で指定されたGoバージョン（現在はGo 1.27）、Clang 20、LLD 20、CMake 4.0以降、およびPCRE2開発ファイルをインストールします。GoサービスはCGOとネイティブライブラリに依存し、[build.sh](./build.sh)が必要なビルド設定を行います。
2. リポジトリをクローンし、必要なネイティブライブラリとモデルファイルを準備してからGoサービスをビルドします:

   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   ```

   ```bash
   python3 -m venv /tmp/ragflow-go-download-venv
   /tmp/ragflow-go-download-venv/bin/python -m pip install requests huggingface-hub
   /tmp/ragflow-go-download-venv/bin/python ragflow_deps/download_deps.py
   bash build.sh --all
   ```

   このスクリプトはGoビルドに必要なネイティブライブラリとモデルリソースを準備し、`requests`と`huggingface-hub`を使用します。同じリソースを別の方法で準備済みの場合は、この手順を省略できます。リポジトリルートから起動するとGoサービスは`internal/rag/res/deepdoc`を自動検出します。別のディレクトリから起動する場合は、`DEEPDOC_MODEL_DIR`にその絶対パスを設定してください。
3. Docker Composeで必要な依存サービス（Elasticsearch、MySQL、MinIO、NATS、Kvrocks、ClickHouse）を起動します:

   ```bash
   sudo sysctl -w vm.max_map_count=262144
   docker compose --env-file docker/.env -f docker/docker-compose-base.yml \
     up -d --wait es01 mysql minio nats kvrocks clickhouse
   ```

   ソースから起動する Go サービスは `localhost:6379` で Kvrocks に接続するため、提供されている設定では `/etc/hosts` の変更は不要です。

4. データベースのマイグレーション後、サービスを順番に起動します。各コマンドはリポジトリのルートから別々のターミナルで実行し、サービス用の4つのターミナルは開いたままにします:

   ```bash
   ./bin/ragflow_server --migrate
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --admin
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --ingestor
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --syncer
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --api
   ```

   各起動モードの役割は次のとおりです:

   - `--migrate`: データベース移行を実行して終了します。
   - `--admin`: 管理と初期化を行うAdminサービスを起動します。
   - `--ingestor`: データ取り込みと解析を行うIngestorサービスを起動します。
   - `--syncer`: データ同期を行うSyncerサービスを起動します。
   - `--api`: Web UI、SDK、外部クライアント向けのAPIサービスを起動します。

   `RAGFLOW_DEV_MODE=true`は開発専用です。コードとデータベース移行バージョン間のダウングレードチェックを無効にしますが、移行の実行やスキーマ変更は行いません。本番環境では設定しないでください。Adminを他のサービスより先に起動します。移行後は`RAGFLOW_DEV_MODE=true bash build.sh --run`でAdmin、Ingestor、APIを起動できますが、Syncerは起動しません。完全なサービスチェーンには`RAGFLOW_DEV_MODE=true ./bin/ragflow_server --syncer`を別途実行してください。
5. フロントエンドを開発する場合に限り、Node.jsとnpmをインストールしてReactフロントエンドを起動します:

   ```bash
   cd web
   npm install
   API_PROXY_SCHEME=go npm run dev
   ```

   別のターミナルでGo APIの準備が完了したことを確認します:

   ```bash
   curl -f http://127.0.0.1:9380/api/v1/system/healthz
   ```

   HTTP 200が返ればAPIは応答しています。開発終了時は各サービスターミナルで`Ctrl+C`を押します。コンテナを保持したまま依存サービスを停止するには `docker compose --env-file docker/.env -f docker/docker-compose-base.yml stop es01 mysql minio nats kvrocks clickhouse` を実行します。名前付きボリュームを保持したまま依存コンテナとComposeネットワークを削除するには `docker compose --env-file docker/.env -f docker/docker-compose-base.yml down` を実行します。

詳細は[ソースコードからサービスを起動](./docs/develop/launch_ragflow_from_source.md)を参照してください。

## 📚 ドキュメンテーション

- [Quickstart](https://ragflow.io/docs/dev/)
- [Configuration](https://ragflow.io/docs/dev/configurations)
- [Release notes](https://ragflow.io/docs/dev/release_notes)
- [User guides](https://ragflow.io/docs/category/user-guides)
- [Developer guides](https://ragflow.io/docs/category/developer-guides)
- [References](https://ragflow.io/docs/dev/category/references)
- [FAQs](https://ragflow.io/docs/dev/faq)

## 📜 ロードマップ

[RAGFlow ロードマップ 2026](https://github.com/infiniflow/ragflow/issues/12241) を参照

## 🏄 コミュニティ

- [Discord](https://discord.gg/NjYzJD3GM3)
- [X](https://x.com/infiniflowai)
- [GitHub Discussions](https://github.com/orgs/infiniflow/discussions)

## 🙌 コントリビュート

RAGFlow はオープンソースのコラボレーションによって発展してきました。この精神に基づき、私たちはコミュニティからの多様なコントリビュートを受け入れています。 参加を希望される方は、まず [コントリビューションガイド](https://ragflow.io/docs/dev/contributing)をご覧ください。
