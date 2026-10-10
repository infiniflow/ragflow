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

<details open>
<summary><b>📕 目次</b></summary>

- 💡 [RAGFlow とは？](#-ragflow-とは)
- 🎮 [はじめに](#-はじめに)
- 🔥 [最新情報](#-最新情報)
- 🌟 [主な特徴](#-主な特徴)
- 🔎 [システム構成](#-システム構成)
- 🏠 [ローカルデプロイ](#-ローカルデプロイ)
- 📚 [ドキュメンテーション](#-ドキュメンテーション)
- 📜 [ロードマップ](#-ロードマップ)
- 🏄 [コミュニティ](#-コミュニティ)
- 🙌 [コントリビュート](#-コントリビュート)

</details>

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

その他の更新については[リリースノート全文](https://ragflow.io/docs/dev/release_notes)を参照してください。


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

### 🌱 **ハルシネーションが軽減された根拠のある引用**

- 可視化されたテキストチャンキング（text chunking）で人間の介入を可能にする。
- 重要な参考文献のクイックビューと、追跡可能な引用によって根拠ある答えをサポートする。

### 🍔 **多様なデータソースとの互換性**

- Word 文書、PowerPoint プレゼンテーション、Excel スプレッドシート、TXT ファイル、画像、PDF、スキャンデータ、構造化データ、Web ページなどをサポート。

### 🛀 **自動化された楽な RAG ワークフロー**

- 個人から大企業まで対応できる RAG オーケストレーション（orchestration）。
- カスタマイズ可能な LLM とエンベッディングモデル。
- 複数の想起と融合された再ランク付け。
- 直感的な API によってビジネスとの統合がシームレスに。

## 🔎 システム構成

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/structure.jpg" alt="RAGFlow system architecture" width="1000" />
</div>

## 🏠 ローカルデプロイ

Docker は迅速な評価、統合テスト、本番環境に、ソース起動は開発とデバッグに適しています。Docker に Go は不要です。ソース起動には `go.mod` 指定の Go、フロントエンド開発には Node.js と npm が必要です。

### 🐳 Docker デプロイ

#### 📝 前提条件

- 推奨される初期構成：4 CPU コア、16 GB RAM、50 GB の空きディスク容量。
- Docker ≥ 24.0.0, Docker Compose ≥ v2.26.1.

#### 🚀 サーバーを起動

1. Elasticsearch: `vm.max_map_count` ≥ 262144.

2. `git clone https://github.com/infiniflow/ragflow.git`

3. Docker:

   ```bash
   cd ragflow/docker
   git checkout v1.0.0-rc1
   docker compose -f docker-compose.yml up -d
   ```

4. サービスと API の準備状況を確認します:

   ```bash
   curl -f http://localhost/api/v1/system/healthz
   docker logs --tail 50 ragflow-cpu
   ```

5. ブラウザで `http://IP_OF_YOUR_MACHINE` を開いてログインします。

6. モデルプロバイダーページで LLM、Embedding、Reranker を追加します。詳細は [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup) を参照してください。

詳細は[クイックスタート](./docs/quickstart.mdx)を参照してください。

#### ⚙️ Docker の設定と調整

`docker/.env` と `docker/docker-compose.yml` を使用します。[Docker 設定](./docker/README.md)および[イメージのビルドとプラットフォームサポート](./docs/develop/build_docker_image.mdx)を参照してください。macOS は現在サポートされていません。

### 🔨 ソースコードから起動

#### 📝 前提条件

1. リポジトリをクローンし、`go.mod` で指定された Go、Clang 20、LLD 20、CMake ≥ 4.0、PCRE2、および CGO に必要なネイティブライブラリをインストールします。

2. 依存関係を準備します:

   ```bash
   python3 -m venv /tmp/ragflow-go-download-venv
   /tmp/ragflow-go-download-venv/bin/python -m pip install requests huggingface-hub
   /tmp/ragflow-go-download-venv/bin/python ragflow_deps/download_deps.py
   ./build.sh --all
   ```

3. サービスをビルドします:

   ```bash
   ./build.sh --all
   ```

4. ローカル依存サービスを起動します:

   ```bash
   sudo sysctl -w vm.max_map_count=262144
   docker compose --env-file docker/.env -f docker/docker-compose-base.yml up -d --wait es01 mysql minio nats kvrocks
   ```

5. データベースをマイグレーションします:

   ```bash
   ./bin/ragflow_server --migrate
   ```

6. 4つのサービスを4つのターミナルで起動します:

   ターミナル1: Admin (`9381`)。

   ```bash
   ./bin/ragflow_server --admin
   ```

   ターミナル2: Ingestor。

   ```bash
   ./bin/ragflow_server --ingestor
   ```

   ターミナル3: Syncer。

   ```bash
   ./bin/ragflow_server --syncer
   ```

   ターミナル4: API (`9380`)。

   ```bash
   ./bin/ragflow_server --api
   ```

7. フロントエンド開発時のみ Node.js と npm をインストールし、API を確認します:

   ```bash
   cd web
   npm install
   curl -f http://127.0.0.1:9380/api/v1/system/healthz
   ```

詳細は[ソースコードからサービスを起動](./docs/develop/launch_ragflow_from_source.md)を参照してください。

> **運用上の注意:** Elasticsearch の設定を永続化するには、`/etc/sysctl.conf` に `vm.max_map_count=262144` を追加します。既定の MySQL 構成では、Docker エントリポイントが移行後に Admin、Syncer、Ingestor、API を起動します。オープンソース 1.0 の DeepDoc はレイアウト解析、OCR、表認識に CPU 推論を使用します。Docker 設定変更後の再起動とデータ保持・削除は [Docker ガイド](./docker/README.md)に従ってください。`download_deps.py` はネイティブライブラリとモデルを準備し、`requests` と `huggingface-hub` を使用します。`--migrate` は移行後に終了し、残りの各オプションは対応サービスを起動します。HTTP 200 は API の準備完了を示します。

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
