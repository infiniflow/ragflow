<div align="center">
<a href="https://demo.ragflow.io/">
<img src="web/src/assets/logo-with-text.png" width="350" alt="ragflow logo">
</a>
</div>

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README_zh.md">简体中文</a> |
  <a href="./README_ja.md">日本語</a>
</p>

<p align="center">
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=Latest%20Release" alt="Latest Release">
    </a>
    <a href="https://demo.ragflow.io" target="_blank">
        <img alt="Static Badge" src="https://img.shields.io/badge/Online-Demo-4e6b99"></a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/badge/docker_pull-ragflow:v0.5.0-brightgreen"
            alt="docker pull infiniflow/ragflow:v0.5.0"></a>
      <a href="https://github.com/infiniflow/ragflow/blob/main/LICENSE">
    <img height="21" src="https://img.shields.io/badge/License-Apache--2.0-ffffff?style=flat-square&labelColor=d4eaf7&color=1570EF" alt="license">
  </a>
</p>

## 💡 RAGFlow とは？

[RAGFlow](https://demo.ragflow.io) は、深い文書理解に基づいたオープンソースの RAG (Retrieval-Augmented Generation) エンジンである。LLM（大規模言語モデル）を組み合わせることで、様々な複雑なフォーマットのデータから根拠のある引用に裏打ちされた、信頼できる質問応答機能を実現し、あらゆる規模のビジネスに適した RAG ワークフローを提供します。

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

## 📌 最新の機能

- 2024-05-08 
- 2024-04-26 「ファイル管理」機能を追加しました。
- 2024-04-19 会話 API をサポートします ([詳細](./docs/conversation_api.md))。
- 2024-04-16 [BCEmbedding](https://github.com/netease-youdao/BCEmbedding) から埋め込みモデル「bce-embedding-base_v1」を追加します。
- 2024-04-16 [FastEmbed](https://github.com/qdrant/fastembed) は、軽量かつ高速な埋め込み用に設計されています。
- 2024-04-11 ローカル LLM デプロイメント用に [Xinference](./docs/xinference.md) をサポートします。
- 2024-04-10 メソッド「Laws」に新しいレイアウト認識モデルを追加します。
- 2024-04-08 [Ollama](./docs/ollama.md) を使用した大規模モデルのローカライズされたデプロイメントをサポートします。
- 2024-04-07 中国語インターフェースをサポートします。

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

1. `vm.max_map_count` >= 262144 であることを確認する【[もっと](./docs/max_map_count.md)】:

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

   ```bash
   $ cd ragflow/docker
   $ chmod +x ./entrypoint.sh
   $ docker compose up -d
   ```

   > 上記のコマンドを実行すると、RAGFlowの開発版dockerイメージが自動的にダウンロードされます。 特定のバージョンのDockerイメージをダウンロードして実行したい場合は、docker/.envファイルのRAGFLOW_VERSION変数を見つけて、対応するバージョンに変更してください。 例えば、RAGFLOW_VERSION=v0.5.0として、上記のコマンドを実行してください。

   > コアイメージのサイズは約 9 GB で、ロードに時間がかかる場合があります。

4. サーバーを立ち上げた後、サーバーの状態を確認する:

   ```bash
   $ docker logs -f ragflow-server
   ```

   _以下の出力は、システムが正常に起動したことを確認するものです:_

   ```bash
       ____                 ______ __
      / __ \ ____ _ ____ _ / ____// /____  _      __
     / /_/ // __ `// __ `// /_   / // __ \| | /| / /
    / _, _// /_/ // /_/ // __/  / // /_/ /| |/ |/ /
   /_/ |_| \__,_/ \__, //_/    /_/ \____/ |__/|__/
                 /____/

    * Running on all addresses (0.0.0.0)
    * Running on http://127.0.0.1:9380
    * Running on http://x.x.x.x:9380
    INFO:werkzeug:Press CTRL+C to quit
   ```
   > もし確認ステップをスキップして直接 RAGFlow にログインした場合、その時点で RAGFlow が完全に初期化されていない可能性があるため、ブラウザーがネットワーク異常エラーを表示するかもしれません。

5. ウェブブラウザで、プロンプトに従ってサーバーの IP アドレスを入力し、RAGFlow にログインします。
   > デフォルトの設定を使用する場合、デフォルトの HTTP サービングポート `80` は省略できるので、与えられたシナリオでは、`http://IP_OF_YOUR_MACHINE`（ポート番号は省略）だけを入力すればよい。
6. [service_conf.yaml](./docker/service_conf.yaml) で、`user_default_llm` で希望の LLM ファクトリを選択し、`API_KEY` フィールドを対応する API キーで更新する。

   > 詳しくは [./docs/llm_api_key_setup.md](./docs/llm_api_key_setup.md) を参照してください。

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
> $ docker-compose up -d
> ```

## 🛠️ ソースからビルドする

ソースからDockerイメージをビルドするには:

```bash
$ git clone https://github.com/tiwater/ragflow.git
$ cd ragflow/
$ docker build -t penless/ragflow:v0.5.0 .
$ cd ragflow/docker
$ chmod +x ./entrypoint.sh
$ docker compose up -d
```

## 🛠️ ソースコードからサービスを起動する方法

ソースコードからサービスを起動する場合は、以下の手順に従ってください:

1. リポジトリをクローンします
```bash
$ git clone https://github.com/tiwater/ragflow.git
$ cd ragflow/
```

2. 仮想環境を作成します（AnacondaまたはMinicondaがインストールされていることを確認してください）
```bash
$ conda create -n ragflow python=3.11.0
$ conda activate ragflow
$ pip install -r requirements.txt
```
CUDAのバージョンが12.0以上の場合、以下の追加コマンドを実行してください：
```bash
$ pip uninstall -y onnxruntime-gpu
$ pip install onnxruntime-gpu --extra-index-url https://aiinfra.pkgs.visualstudio.com/PublicPackages/_packaging/onnxruntime-cuda-12/pypi/simple/
```

3. エントリースクリプトをコピーし、環境変数を設定します
```bash
$ cp docker/entrypoint.sh .
$ vi entrypoint.sh
```
以下のコマンドでPythonのパスとragflowプロジェクトのパスを取得します：
```bash
$ which python
$ pwd
```

`which python`の出力を`PY`の値として、`pwd`の出力を`PYTHONPATH`の値として設定します。

`LD_LIBRARY_PATH`が既に設定されている場合は、コメントアウトできます。

```bash
# 実際の状況に応じて設定を調整してください。以下の二つのexportは新たに追加された設定です
PY=${PY}
export PYTHONPATH=${PYTHONPATH}
# オプション：Hugging Faceミラーを追加
export HF_ENDPOINT=https://hf-mirror.com
```

4. 基本サービスを起動します
```bash
$ cd docker
$ docker compose -f docker-compose-base.yml up -d 
```

5. 設定ファイルを確認します
**docker/.env**内の設定が**conf/service_conf.yaml**内の設定と一致していることを確認してください。**service_conf.yaml**内の関連サービスのIPアドレスとポートは、ローカルマシンのIPアドレスとコンテナが公開するポートに変更する必要があります。

6. サービスを起動します
```bash
$ chmod +x ./entrypoint.sh
$ bash ./entrypoint.sh
```

## 📚 ドキュメンテーション

- [FAQ](./docs/faq.md)

## 📜 ロードマップ

[RAGFlow ロードマップ 2024](https://github.com/infiniflow/ragflow/issues/162) を参照

## 🏄 コミュニティ

- [Discord](https://discord.gg/4XxujFgUN7)
- [Twitter](https://twitter.com/infiniflowai)

## 🙌 コントリビュート

RAGFlow はオープンソースのコラボレーションによって発展してきました。この精神に基づき、私たちはコミュニティからの多様なコントリビュートを受け入れています。 参加を希望される方は、まず[コントリビューションガイド](https://github.com/infiniflow/ragflow/blob/main/docs/CONTRIBUTING.md)をご覧ください。
