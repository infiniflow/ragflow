<div align="center">
<a href="https://cloud.ragflow.io/">
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
  <a href="./README_pt_br.md"><img alt="Português(Brasil)" src="https://img.shields.io/badge/Português(Brasil)-DFE0E5"></a>
  <a href="./README_fr.md"><img alt="README en Français" src="https://img.shields.io/badge/Français-DBEDFA"></a>
  <a href="./README_ar.md"><img alt="README in Arabic" src="https://img.shields.io/badge/Arabic-DFE0E5"></a>
  <a href="./README_tr.md"><img alt="Türkçe README" src="https://img.shields.io/badge/Türkçe-DFE0E5"></a>
</p>

<p align="center">
    <a href="https://x.com/intent/follow?screen_name=infiniflowai" target="_blank">
        <img src="https://img.shields.io/twitter/follow/infiniflow?logo=X&color=%20%23f5f5f5" alt="suivre sur X(Twitter)">
    </a>
    <a href="https://cloud.ragflow.io" target="_blank">
        <img alt="Badge statique" src="https://img.shields.io/badge/Online-Demo-4e6b99">
    </a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/docker/pulls/infiniflow/ragflow?label=Docker%20Pulls&color=0db7ed&logo=docker&logoColor=white&style=flat-square" alt="docker pull infiniflow/ragflow:v0.24.0">
    </a>
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=Dernière%20version" alt="Dernière version">
    </a>
    <a href="https://github.com/infiniflow/ragflow/blob/main/LICENSE">
        <img height="21" src="https://img.shields.io/badge/License-Apache--2.0-ffffff?labelColor=d4eaf7&color=2e6cc4" alt="licence">
    </a>
    <a href="https://deepwiki.com/infiniflow/ragflow">
        <img alt="Ask DeepWiki" src="https://deepwiki.com/badge.svg">
    </a>
</p>

<h4 align="center">
  <a href="https://ragflow.io/docs/dev/">Documentation</a> |
  <a href="https://github.com/infiniflow/ragflow/issues/12241">Roadmap</a> |
  <a href="https://twitter.com/infiniflowai">Twitter</a> |
  <a href="https://discord.gg/NjYzJD3GM3">Discord</a> |
  <a href="https://cloud.ragflow.io">Démo</a>
</h4>

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/ragflow-octoverse.png" width="1200"/>
</div>

<div align="center">
<a href="https://trendshift.io/repositories/9064" target="_blank"><img src="https://trendshift.io/api/badge/repositories/9064" alt="infiniflow%2Fragflow | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/></a>
</div>

<details open>
<summary><b>📕 Table des matières</b></summary>

- 💡 [Qu'est-ce que RAGFlow?](#-quest-ce-que-ragflow)
- 🎮 [Démo](#-démo)
- 📌 [Dernières mises à jour](#-dernières-mises-à-jour)
- 🌟 [Fonctionnalités clés](#-fonctionnalités-clés)
- 🔎 [Architecture du système](#-architecture-du-système)
- 🎬 [Démarrage](#-démarrage)
- 🔧 [Configurations](#-configurations)
- 🔧 [Construire une image Docker](#-construire-une-image-docker)
- 🔨 [Lancer le service depuis les sources pour le développement](#-lancer-le-service-depuis-les-sources-pour-le-développement)
- 📚 [Documentation](#-documentation)
- 📜 [Roadmap](#-feuille-de-route)
- 🏄 [Communauté](#-communauté)
- 🙌 [Contribuer](#-contribuer)

</details>

## 💡 Qu'est-ce que RAGFlow?

[RAGFlow](https://ragflow.io/) est un moteur de [RAG](https://ragflow.io/basics/what-is-rag) (Retrieval-Augmented Generation) open-source de premier plan qui fusionne les technologies RAG de pointe avec des capacités Agent pour créer une couche de contexte supérieure pour les LLM. Il offre un flux de travail RAG rationalisé, adaptable aux entreprises de toute taille. Alimenté par un [moteur de contexte](https://ragflow.io/basics/what-is-agent-context-engine) convergent et des modèles d'agents préconstruits, RAGFlow permet aux développeurs de transformer des données complexes en systèmes d'IA haute-fidélité, prêts pour la production, avec une efficacité et une précision exceptionnelles.

## 🎮 Démo

Essayez notre démo sur [https://cloud.ragflow.io](https://cloud.ragflow.io).

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/chunking.gif" width="1200"/>
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/agentic-dark.gif" width="1200"/>
</div>

## 🔥 Dernières mises à jour

- 24-03-2026 [RAGFlow Skill on OpenClaw](https://clawhub.ai/yingfeng/ragflow-skill) — Fournit un skill officiel pour accéder aux datasets RAGFlow via OpenClaw.
- 26-12-2025 Prise en charge de la « Mémoire » pour l'agent IA.
- 19-11-2025 Prise en charge de Gemini 3 Pro.
- 12-11-2025 Prise en charge de la synchronisation de données depuis Confluence, S3, Notion, Discord et Google Drive.
- 23-10-2025 Prise en charge de MinerU & Docling comme méthodes d'analyse de documents.
- 15-10-2025 Prise en charge du pipeline d'ingestion orchestrable.
- 08-08-2025 Prise en charge des derniers modèles de la série GPT-5 d'OpenAI.
- 01-08-2025 Prise en charge du flux de travail agentique et de MCP.
- 23-05-2025 Ajout d'un composant exécuteur de code Python/JavaScript à l'Agent.
- 05-05-2025 Prise en charge des requêtes inter-langues.
- 19-03-2025 Prise en charge de l'utilisation d'un modèle multi-modal pour analyser les images dans les fichiers PDF ou DOCX.

## 🎉 Restez informé

⭐️ Mettez une étoile à notre dépôt pour rester informé des nouvelles fonctionnalités et améliorations passionnantes ! Recevez des notifications instantanées pour les nouvelles versions ! 🌟

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/user-attachments/assets/18c9707e-b8aa-4caf-a154-037089c105ba" width="1200"/>
</div>

## 🌟 Fonctionnalités clés

### 🍭 **"Quality in, quality out"**

- Extraction de connaissances basée sur la [compréhension approfondie des documents](./deepdoc/README.md) à partir de données non structurées aux formats complexes.
- Trouve "l'aiguille dans la meule de données" de tokens littéralement illimités.

### 🍱 **Découpage(Chunking) basé sur des templates**

- Intelligent et explicable.
- De nombreuses options de templates disponibles.

### 🌱 **Citations fondées avec réduction des hallucinations**

- Visualisation du découpage de texte pour permettre une intervention humaine.
- Aperçu rapide des références clés et citations traçables pour soutenir des réponses fondées.

### 🍔 **Compatibilité avec des sources de données hétérogènes**

- Prend en charge Word, présentations, Excel, txt, images, copies numérisées, données structurées, pages web, et plus encore.

### 🛀 **Flux de travail RAG automatisé et sans effort**

- Orchestration RAG rationalisée adaptée aux particuliers comme aux grandes entreprises.
- LLM et modèles d'embedding configurables.
- Rappel multiple associé à un ré-classement fusionné.
- APIs intuitives pour une intégration transparente avec les entreprises.

## 🔎 Architecture du système

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/user-attachments/assets/31b0dd6f-ca4f-445a-9457-70cb44a381b2" width="1000"/>
</div>

## 🎬 Démarrage

### 📝 Prérequis

- CPU >= 4 cœurs
- RAM >= 16 Go
- Disque >= 50 Go
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
- [gVisor](https://gvisor.dev/docs/user_guide/install/) : Requis uniquement si vous souhaitez utiliser la fonctionnalité d'exécuteur de code (sandbox) de RAGFlow.

> [!TIP]
> Si vous n'avez pas installé Docker sur votre machine locale (Windows, Mac ou Linux), consultez [Installer Docker Engine](https://docs.docker.com/engine/install/).

### 🚀 Démarrer le serveur

1. Assurez-vous que `vm.max_map_count` >= 262144 :

   > Pour vérifier la valeur de `vm.max_map_count` :
   >
   > ```bash
   > $ sysctl vm.max_map_count
   > ```
   >
   > Réinitialisez `vm.max_map_count` à une valeur d'au moins 262144 si ce n'est pas le cas.
   >
   > ```bash
   > # Dans ce cas, nous le définissons à 262144 :
   > $ sudo sysctl -w vm.max_map_count=262144
   > ```
   >
   > Ce changement sera réinitialisé après un redémarrage du système. Pour que votre modification reste permanente, ajoutez ou mettez à jour la valeur `vm.max_map_count` dans **/etc/sysctl.conf** :
   >
   > ```bash
   > vm.max_map_count=262144
   > ```
   >
2. Clonez le dépôt :

   ```bash
   $ git clone https://github.com/infiniflow/ragflow.git
   ```
3. Démarrez le serveur en utilisant les images Docker préconstruites :

> [!CAUTION]
> Toutes les images Docker sont construites pour les plateformes x86. Nous ne proposons pas actuellement d'images Docker pour ARM64.
> Si vous êtes sur une plateforme ARM64, suivez [ce guide](https://ragflow.io/docs/dev/build_docker_image) pour construire une image Docker compatible avec votre système.

> La commande ci-dessous télécharge l'édition `v0.24.0` de l'image Docker RAGFlow. Consultez le tableau suivant pour les descriptions des différentes éditions de RAGFlow. Pour télécharger une édition de RAGFlow différente de `v0.24.0`, mettez à jour la variable `RAGFLOW_IMAGE` dans **docker/.env** avant d'utiliser `docker compose` pour démarrer le serveur.

```bash
   $ cd ragflow/docker

   # git checkout v0.24.0
   # Optionnel : utiliser un tag stable (voir les versions : https://github.com/infiniflow/ragflow/releases)
   # Cette étape garantit que le fichier **entrypoint.sh** dans le code correspond à la version de l'image Docker.

   # Use CPU for DeepDoc tasks:
   $ docker compose -f docker-compose.yml up -d

   # To use GPU to accelerate DeepDoc tasks:
   # sed -i '1i DEVICE=gpu' .env
   # docker compose -f docker-compose.yml up -d
```

> Remarque : Avant `v0.22.0`, nous fournissions à la fois des images avec des modèles d'embedding et des images slim sans modèles d'embedding. Détails ci-dessous :

| RAGFlow image tag | Image size (GB) | Has embedding models? | Stable?        |
|-------------------|-----------------|-----------------------|----------------|
| v0.21.1           | &approx;9       | ✔️                    | Stable release |
| v0.21.1-slim      | &approx;2       | ❌                     | Stable release |

> À partir de `v0.22.0`, nous ne distribuons que l'édition slim et ne rajoutons plus le suffixe **-slim** au tag d'image.

4. Vérifiez l'état du serveur après son démarrage :

   ```bash
   $ docker logs -f docker-ragflow-cpu-1
   ```

   _La sortie suivante confirme un lancement réussi du système :_

   ```bash

         ____   ___    ______ ______ __
        / __ \ /   |  / ____// ____// /____  _      __
       / /_/ // /| | / / __ / /_   / // __ \| | /| / /
      / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ /
     /_/ |_|/_/  |_|\____//_/    /_/ \____/ |__/|__/

    * Running on all addresses (0.0.0.0)
   ```

   > Si vous sautez cette étape de confirmation et vous connectez directement à RAGFlow, votre navigateur peut afficher une erreur `network abnormal`, car à ce moment-là, votre RAGFlow peut ne pas être entièrement initialisé.
   >
5. Dans votre navigateur web, entrez l'adresse IP de votre serveur et connectez-vous à RAGFlow.

   > Avec les paramètres par défaut, il vous suffit d'entrer `http://IP_OF_YOUR_MACHINE` (**sans** numéro de port), car le port HTTP par défaut `80` peut être omis lors de l'utilisation des configurations par défaut.
   >
6. Dans [service_conf.yaml.template](./docker/service_conf.yaml.template), sélectionnez la fabrique LLM souhaitée dans `user_default_llm` et mettez à jour le champ `API_KEY` avec la clé API correspondante.

   > Voir [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup) pour plus d'informations.
   >

   _Le spectacle commence !_

## 🔧 Configurations

En ce qui concerne les configurations système, vous devrez gérer les fichiers suivants :

- [.env](./docker/.env) : Conserve les paramètres de base du système, tels que `SVR_HTTP_PORT`, `MYSQL_PASSWORD` et `MINIO_PASSWORD`.
- [service_conf.yaml.template](./docker/service_conf.yaml.template) : Configure les services back-end. Les variables d'environnement dans ce fichier seront automatiquement renseignées au démarrage du conteneur Docker. Toutes les variables d'environnement définies dans le conteneur Docker seront disponibles, vous permettant de personnaliser le comportement du service en fonction de l'environnement de déploiement.
- [docker-compose.yml](./docker/docker-compose.yml) : Le système s'appuie sur [docker-compose.yml](./docker/docker-compose.yml) pour démarrer.

> Le fichier [./docker/README](./docker/README.md) fournit une description détaillée des paramètres d'environnement et des configurations de services qui peuvent être utilisés comme `${ENV_VARS}` dans le fichier [service_conf.yaml.template](./docker/service_conf.yaml.template).

Pour mettre à jour le port HTTP de service par défaut (80), accédez à [docker-compose.yml](./docker/docker-compose.yml) et changez `80:80` en `<YOUR_SERVING_PORT>:80`.

Les mises à jour des configurations ci-dessus nécessitent un redémarrage de tous les conteneurs pour prendre effet :

> ```bash
> $ docker compose -f docker-compose.yml up -d
> ```

### Passer du moteur de documents Elasticsearch à Infinity

RAGFlow utilise Elasticsearch par défaut pour stocker le texte intégral et les vecteurs. Pour passer à [Infinity](https://github.com/infiniflow/infinity/), suivez ces étapes :

1. Arrêtez tous les conteneurs en cours d'exécution :

   ```bash
   $ docker compose -f docker/docker-compose.yml down -v
   ```

> [!WARNING]
> `-v` supprimera les volumes des conteneurs Docker, et les données existantes seront effacées.

2. Définissez `DOC_ENGINE` dans **docker/.env** sur `infinity`.
3. Démarrez les conteneurs :

   ```bash
   $ docker compose -f docker-compose.yml up -d
   ```

> [!WARNING]
> Le passage à Infinity sur une machine Linux/arm64 n'est pas encore officiellement pris en charge.

## 🔧 Construire une image Docker

Cette image fait environ 2 Go et dépend de services LLM et d'embedding externes.

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
docker build --platform linux/amd64 -f Dockerfile -t infiniflow/ragflow:nightly .
```

Ou si vous êtes derrière un proxy, vous pouvez passer des arguments de proxy :

```bash
docker build --platform linux/amd64 \
  --build-arg http_proxy=http://YOUR_PROXY:PORT \
  --build-arg https_proxy=http://YOUR_PROXY:PORT \
  -f Dockerfile -t infiniflow/ragflow:nightly .
```

## 🔨 Lancer le service depuis les sources pour le développement

1. Installez `uv` et `pre-commit`, ou ignorez cette étape s'ils sont déjà installés :

   ```bash
   pipx install uv pre-commit
   ```
2. Clonez le code source et installez les dépendances Python :

   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   uv sync --python 3.12 # install RAGFlow dependent python modules
   uv run download_deps.py
   pre-commit install
   ```
3. Lancez les services dépendants (MinIO, Elasticsearch, Redis et MySQL) avec Docker Compose :

   ```bash
   docker compose -f docker/docker-compose-base.yml up -d
   ```

   Ajoutez la ligne suivante à `/etc/hosts` pour résoudre tous les hôtes spécifiés dans **docker/.env** vers `127.0.0.1` :

   ```
   127.0.0.1       es01 infinity mysql minio redis sandbox-executor-manager
   ```
4. Si vous ne pouvez pas accéder à HuggingFace, définissez la variable d'environnement `HF_ENDPOINT` pour utiliser un site miroir :

   ```bash
   export HF_ENDPOINT=https://hf-mirror.com
   ```
5. Si votre système d'exploitation n'a pas jemalloc, installez-le comme suit :

   ```bash
   # Ubuntu
   sudo apt-get install libjemalloc-dev
   # CentOS
   sudo yum install jemalloc
   # OpenSUSE
   sudo zypper install jemalloc
   # macOS
   sudo brew install jemalloc
   ```
6. Lancez le service back-end :

   ```bash
   source .venv/bin/activate
   export PYTHONPATH=$(pwd)
   bash docker/launch_backend_service.sh
   ```
7. Installez les dépendances front-end :

   ```bash
   cd web
   npm install
   ```
8. Lancez le service front-end :

   ```bash
   npm run dev
   ```

   _La sortie suivante confirme un lancement réussi du système :_

   ![](https://github.com/user-attachments/assets/0daf462c-a24d-4496-a66f-92533534e187)
9. Arrêtez les services front-end et back-end de RAGFlow une fois le développement terminé :

   ```bash
   pkill -f "ragflow_server.py|task_executor.py"
   ```

## 📚 Documentation

- [Quickstart](https://ragflow.io/docs/dev/)
- [Configuration](https://ragflow.io/docs/dev/configurations)
- [Release notes](https://ragflow.io/docs/dev/release_notes)
- [User guides](https://ragflow.io/docs/category/user-guides)
- [Developer guides](https://ragflow.io/docs/category/developer-guides)
- [References](https://ragflow.io/docs/dev/category/references)
- [FAQs](https://ragflow.io/docs/dev/faq)

## 📜 Roadmap

Voir la [Feuille de route RAGFlow 2026](https://github.com/infiniflow/ragflow/issues/12241)

## 🏄 Communauté

- [Discord](https://discord.gg/NjYzJD3GM3)
- [Twitter](https://twitter.com/infiniflowai)
- [GitHub Discussions](https://github.com/orgs/infiniflow/discussions)

## 🙌 Contribuer

RAGFlow s'épanouit grâce à la collaboration open-source. Dans cet esprit, nous accueillons des contributions diverses de la communauté.
Si vous souhaitez en faire partie, consultez d'abord nos [Directives de contribution](https://ragflow.io/docs/dev/contributing).
