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
  <a href="./README_fr.md"><img alt="README en Français" src="https://img.shields.io/badge/Français-DBEDFA"></a>
  <a href="./README_id.md"><img alt="Bahasa Indonesia" src="https://img.shields.io/badge/Bahasa Indonesia-DFE0E5"></a>
  <a href="./README_pt_br.md"><img alt="Português(Brasil)" src="https://img.shields.io/badge/Português(Brasil)-DFE0E5"></a>
  <a href="./README_ar.md"><img alt="README in Arabic" src="https://img.shields.io/badge/Arabic-DFE0E5"></a>
  <a href="./README_tr.md"><img alt="Türkçe README" src="https://img.shields.io/badge/Türkçe-DFE0E5"></a>
  <a href="./README_ru.md"><img alt="Русская версия README" src="https://img.shields.io/badge/Русский-DFE0E5"></a>
</p>

<p align="center">
    <a href="https://x.com/intent/follow?screen_name=infiniflowai" target="_blank">
        <img src="https://img.shields.io/twitter/follow/infiniflow?logo=X&color=%20%23f5f5f5" alt="suivre sur X(Twitter)">
    </a>
    <a href="https://cloud.ragflow.io" target="_blank">
        <img alt="Badge statique" src="https://img.shields.io/badge/Get-Started-4e6b99">
    </a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/infiniflow/ragflow-stats/main/badges/docker-pulls.json&style=flat-square&logo=docker&logoColor=white" alt="docker pull infiniflow/ragflow:v0.27.2">
    </a>
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=Derniere%20version" alt="Dernière version">
    </a>
    <a href="https://github.com/infiniflow/ragflow/blob/main/LICENSE">
        <img height="21" src="https://img.shields.io/badge/License-Apache--2.0-ffffff?labelColor=d4eaf7&color=2e6cc4" alt="licence">
    </a>
    <a href="https://deepwiki.com/infiniflow/ragflow">
        <img alt="Ask DeepWiki" src="https://deepwiki.com/badge.svg">
    </a>
</p>

<h4 align="center">
  <a href="https://cloud.ragflow.io">Cloud</a> |
  <a href="https://ragflow.io/docs/dev/">Documentation</a> |
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
<summary><b>📕 Table des matières</b></summary>

- 💡 [Qu'est-ce que RAGFlow?](#-quest-ce-que-ragflow)
- 🎮 [Démarrage](#-démarrage)
- 📌 [Dernières mises à jour](#-dernières-mises-à-jour)
- 🌟 [Fonctionnalités clés](#-fonctionnalités-clés)
- 🔎 [Architecture du système](#-architecture-du-système)
- 🎬 [Auto-hébergement](#-auto-hébergement)
- 🔧 [Configurations](#-configurations)
- 🔧 [Construire une image Docker](#-construire-une-image-docker)
- 🔨 [Lancer le service depuis les sources pour le développement](#-lancer-le-service-depuis-les-sources-pour-le-développement)
- 📚 [Documentation](#-documentation)
- 📜 [Roadmap](#-roadmap)
- 🏄 [Communauté](#-communauté)
- 🙌 [Contribuer](#-contribuer)

</details>

## 💡 Qu'est-ce que RAGFlow?

[RAGFlow](https://ragflow.io/) est un moteur de [RAG](https://ragflow.io/basics/what-is-rag) (Retrieval-Augmented Generation) open-source de premier plan qui fusionne les technologies RAG de pointe avec des capacités Agent pour créer une couche de contexte supérieure pour les LLM. Il offre un flux de travail RAG rationalisé, adaptable aux entreprises de toute taille. Alimenté par un [moteur de contexte](https://ragflow.io/basics/what-is-agent-context-engine) convergent et des modèles d'agents préconstruits, RAGFlow permet aux développeurs de transformer des données complexes en systèmes d'IA haute-fidélité, prêts pour la production, avec une efficacité et une précision exceptionnelles.

## 🎮 Démarrage

Essayez notre service cloud sur [https://cloud.ragflow.io](https://cloud.ragflow.io).

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="Chunking demonstration" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/chunking.gif" width="1200"/>
<img alt="Agentic workflow demonstration" src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/agentic-dark.gif" width="1200"/>
</div>

## 🔥 Dernières mises à jour

- 2026-09-10 Ajout de l’ingestion de contenus Web à partir de sitemaps.
- 2026-08-19 Lancement de Knowledge Compilation pour générer des wikis, graphes, arbres, PageIndex, cartes mentales, chronologies et compétences au niveau des documents et des bases de connaissances.
- 2026-08-19 Lancement d’Agentic RAG avec quatre modes de réflexion : Low, Medium, High et Ultra.
- 2026-07-02 Ajout de l’ingestion de données depuis Google BigQuery et de la synchronisation incrémentielle.
- 2026-06-29 Ajout de canaux de conversation WhatsApp, DingTalk et WeCom.
- 2026-05-26 Ajout du composant Browser, qui permet aux Agents de parcourir et d’utiliser des pages Web.
- 2026-04-21 Ajout de sept modèles prédéfinis de pipelines d’ingestion de données.
- 2026-04-21 Ajout de la publication d’applications Agent, de l’exécution de code en sandbox et de la génération de graphiques.
- 2026-04-21 Ajout du stockage et de la récupération de mémoire au niveau utilisateur.

Consultez les [notes de version complètes](./docs/release_notes.md) pour découvrir les autres nouveautés.

## 🎉 Restez informé

⭐️ Mettez une étoile à notre dépôt pour rester informé des nouvelles fonctionnalités et améliorations passionnantes ! Recevez des notifications instantanées pour les nouvelles versions ! 🌟

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img alt="RAGFlow feature updates" src="https://github.com/user-attachments/assets/18c9707e-b8aa-4caf-a154-037089c105ba" width="1200"/>
</div>

## 🌟 Fonctionnalités clés

### 🍭 **"Quality in, quality out"**

- Extraction de connaissances basée sur la [compréhension approfondie des documents](./deepdoc/README.md) à partir de données non structurées aux formats complexes.
- Trouve "l'aiguille dans la meule de données" de tokens littéralement illimités.

### 🍱 **Découpage(Chunking) basé sur des templates**

- Intelligent et explicable.
- De nombreuses options de templates disponibles.

### 🧩 **Compilation des connaissances (Knowledge Compilation)**

- Transformez le contenu des documents et des jeux de données en connaissances structurées, notamment Wiki, Graph, Tree, PageIndex, Mind Map, Timeline et Skills.
- Configurez les modèles et règles de traitement, puis consultez, mettez à jour ou régénérez les résultats.

### 🧠 **Recherche agentique (Agentic Retrieval)**

- Le modèle analyse les questions complexes et peut les décomposer, rechercher des connaissances et vérifier les preuves en plusieurs étapes.
- Les modes Low, Medium, High et Ultra permettent de régler la profondeur de recherche et de raisonnement selon la complexité de la question.

### ⚙️ **Architecture de services native Go**

- Une implémentation Go unifiée fournit API, Admin, Ingestor et Syncer. DeepDoc s’exécute dans le processus Go pour l’analyse de mise en page, l’OCR et la reconnaissance des tableaux.
- Les services Go appellent les bibliothèques natives d’analyse de documents et ONNX Runtime via CGO. MCP et Sandbox Executor sont activables à la demande.

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
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/structure.jpg" alt="Architecture du système RAGFlow" width="1000" />
</div>

## 🎬 Auto-hébergement

### 📝 Prérequis

- Configuration de départ recommandée : 4 cœurs CPU, 16 Go de RAM et 50 Go d’espace disque disponible. Les besoins réels dépendent du moteur de documents, du volume de données, des tâches d’analyse et de la concurrence. Les modèles locaux et OceanBase nécessitent des ressources supplémentaires.
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
- [gVisor](https://gvisor.dev/docs/user_guide/install/) : requis uniquement avec le Sandbox de conteneurs Self-Managed.

Le déploiement Docker ne nécessite pas l’installation de Go sur l’hôte. Le déploiement GPU nécessite également NVIDIA Container Toolkit. Le Sandbox de conteneurs Self-Managed nécessite l’installation et la configuration de gVisor ; les autres fournisseurs de Sandbox n’en ont pas besoin sur l’hôte RAGFlow.

> [!TIP]
> Si vous n'avez pas installé Docker sur votre machine locale (Windows, Mac ou Linux), consultez [Installer Docker Engine](https://docs.docker.com/engine/install/).

### 🚀 Démarrer le serveur

1. Assurez-vous que `vm.max_map_count` >= 262144 :

   > Pour vérifier la valeur de `vm.max_map_count` :
   >
   > ```bash
   > sysctl vm.max_map_count
   > ```
   >
   > Réinitialisez `vm.max_map_count` à une valeur d'au moins 262144 si ce n'est pas le cas.
   >
   > ```bash
   > # Dans ce cas, nous le définissons à 262144 :
   > sudo sysctl -w vm.max_map_count=262144
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
   git clone https://github.com/infiniflow/ragflow.git
   ```
3. Construisez l’image Go et démarrez le serveur avec la configuration Compose Go. La cible de construction officielle est `linux/amd64` ; le mode CPU est utilisé par défaut. Le déploiement GPU nécessite NVIDIA Container Toolkit. Consultez le [guide de construction de l’image Go et de prise en charge des plateformes](./docs/develop/build_docker_image.mdx) pour connaître les exigences détaillées.

   Avant le premier déploiement, définissez `RAGFLOW_IMAGE=ragflow:go-local` dans **docker/.env**, puis exécutez depuis la racine du dépôt :

   ```bash
   cd ragflow
   docker build --platform linux/amd64 -f Dockerfile -t ragflow:go-local .
   cd docker
   docker compose --env-file .env -f docker-compose.yml up -d
   ```

   Avec la configuration MySQL par défaut, le point d’entrée de l’image exécute d’abord les migrations, puis démarre Syncer, Admin, API et Ingestor via `bin/ragflow_server`. Dans RAGFlow open-source 1.0, DeepDoc utilise l’inférence CPU pour l’analyse de mise en page, l’OCR et la reconnaissance des tableaux, même si le profil GPU est activé.

4. Vérifiez l’état des dépendances avec `docker compose --env-file .env -f docker-compose.yml ps`, puis confirmez que RAGFlow est prêt avec l’interface HTTP (le conteneur RAGFlow ne définit pas de healthcheck Compose) :

   ```bash
   curl -f http://localhost/api/v1/system/healthz
   ```

   Une réponse HTTP 200 indique que le service est prêt. Si `SVR_WEB_HTTP_PORT` a été modifié, utilisez ce port dans l’URL. En cas d’échec du démarrage, consultez les journaux du service concerné avec Compose.
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
> docker compose -f docker-compose.yml up -d
> ```

### Passer du moteur de documents Elasticsearch à Infinity

RAGFlow utilise Elasticsearch par défaut pour stocker le texte intégral et les vecteurs. Pour passer à [Infinity](https://github.com/infiniflow/infinity/), suivez ces étapes :

1. Arrêtez tous les conteneurs en cours d'exécution :

   ```bash
   docker compose -f docker/docker-compose.yml down -v
   ```

> [!WARNING]
> `-v` supprimera les volumes des conteneurs Docker, et les données existantes seront effacées.

2. Définissez `DOC_ENGINE` dans **docker/.env** sur `infinity`.
3. Démarrez les conteneurs :

   ```bash
   docker compose -f docker/docker-compose.yml up -d
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

1. Installez la version de Go indiquée dans `go.mod` (Go 1.27 actuellement), Clang 20, LLD 20, CMake 4.0 ou version ultérieure, ainsi que les fichiers de développement PCRE2. Les services Go dépendent de CGO et de bibliothèques natives ; [build.sh](./build.sh) configure les paramètres nécessaires.
2. Clonez le dépôt, préparez les bibliothèques natives et les fichiers de modèles requis, puis compilez les services Go :

   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   python3 -m venv /tmp/ragflow-go-download-venv
   /tmp/ragflow-go-download-venv/bin/python -m pip install requests huggingface-hub
   /tmp/ragflow-go-download-venv/bin/python ragflow_deps/download_go_deps.py
   bash build.sh --all
   ```
3. Lancez les dépendances requises (Elasticsearch, MySQL, MinIO, NATS, Kvrocks et ClickHouse) avec Docker Compose :

   ```bash
   docker compose --env-file docker/.env -f docker/docker-compose-base.yml \
     up -d --wait es01 mysql minio nats kvrocks clickhouse
   ```

   Ajoutez la ligne suivante à `/etc/hosts` pour résoudre les hôtes définis dans **conf/service_conf.yaml** vers `127.0.0.1` :

   ```text
   127.0.0.1       es01 mysql minio nats kvrocks clickhouse
   ```
4. Après la migration de la base de données, démarrez les services dans l’ordre. Exécutez chaque commande dans un terminal distinct depuis la racine du dépôt et laissez ouverts les quatre terminaux des services :

   ```bash
   ./bin/ragflow_server --migrate
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --admin
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --ingestor
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --syncer
   RAGFLOW_DEV_MODE=true ./bin/ragflow_server --api
   ```
5. Installez Node.js et npm uniquement pour développer le front-end :

   ```bash
   cd web
   npm install
   ```
8. Lancez le service front-end :

   ```bash
   API_PROXY_SCHEME=go npm run dev
   ```

   _La sortie suivante confirme un lancement réussi du système :_

   ![RAGFlow web interface](https://github.com/user-attachments/assets/0daf462c-a24d-4496-a66f-92533534e187)
9. Une fois le développement terminé, arrêtez chaque service avec Ctrl+C dans son terminal :

   ```bash
   # Appuyez sur Ctrl+C dans chaque terminal de service pour arrêter le processus correspondant.
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
- [X](https://x.com/infiniflowai)
- [GitHub Discussions](https://github.com/orgs/infiniflow/discussions)

## 🙌 Contribuer

RAGFlow s'épanouit grâce à la collaboration open-source. Dans cet esprit, nous accueillons des contributions diverses de la communauté.
Si vous souhaitez en faire partie, consultez d'abord nos [Directives de contribution](https://ragflow.io/docs/dev/contributing).
