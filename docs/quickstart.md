# Quickstart

RAGFlow is an open-source RAG (Retrieval-Augmented Generation) engine based on deep document understanding.

When integrated with LLM, it is capable of providing truthful question-answering capabilities, backed by well-founded citations from various complex formatted data.

This document describes how to set up a local RAGFlow server, how to create a knowledge base, and how to set up an AI chat based on your dataset. 

## Prerequisites

- CPU >= 4 cores

- RAM >= 16 GB

- Disk >= 50 GB

- Docker >= 24.0.0 & Docker Compose >= v2.26.1

  > If you have not installed Docker on your local machine (Windows, Mac, or Linux), see [Install Docker Engine](https://docs.docker.com/engine/install/).

## Start up the server

1. Ensure `vm.max_map_count` >= 262144 ([more](./docs/max_map_count.md)):

   > To check the value of `vm.max_map_count`:
   >
   > ```bash
   > $ sysctl vm.max_map_count
   > ```
   >
   > Reset `vm.max_map_count` to a value at least 262144 if it is not.
   >
   > ```bash
   > # In this case, we set it to 262144:
   > $ sudo sysctl -w vm.max_map_count=262144
   > ```
   >
   > This change will be reset after a system reboot. To ensure your change remains permanent, add or update the `vm.max_map_count` value in **/etc/sysctl.conf** accordingly:
   >
   > ```bash
   > vm.max_map_count=262144
   > ```

2. Clone the repo:

   ```bash
   $ git clone https://github.com/infiniflow/ragflow.git
   ```

3. Build the pre-built Docker images and start up the server:

   > Running the following commands automatically downloads the *dev* version RAGFlow Docker image. To download and run a specified Docker version, update `RAGFLOW_VERSION` in **docker/.env** to the intended version, for example `RAGFLOW_VERSION=v0.5.0`, before running the following commands.

   ```bash
   $ cd ragflow/docker
   $ chmod +x ./entrypoint.sh
   $ docker compose up -d
   ```

> The core image is about 9 GB in size and may take a while to load.

4. Check the server status after having the server up and running:

   ```bash
   $ docker logs -f ragflow-server
   ```

   _The following output confirms a successful launch of the system:_

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

   > If you skip this confirmation step and directly log in to RAGFlow, your browser may prompt a `network anomaly` error because, at that moment, your RAGFlow may not be fully initialized.  

5. In your web browser, enter the IP address of your server and log in to RAGFlow.

   > With default settings, you only need to enter `http://IP_OF_YOUR_MACHINE` (**sans** port number) as the default HTTP serving port `80` can be omitted when using the default configurations.

6. In [service_conf.yaml](./docker/service_conf.yaml), select the desired LLM factory in `user_default_llm` and update the `API_KEY` field with the corresponding API key.

   > See [./docs/llm_api_key_setup.md](./docs/llm_api_key_setup.md) for more information.

   _The show is now on!_

## LLM configurations

For now, RAGFlow supports DeepSeek-V2, Moonshot, OpenAI, Tongyi-Qianwen, and also supports deploy LLM locally through Ollama and Xinference. To configure your LLM:

1. Click on your logo on the top right of the page > **Model Providers**.
2. Choose the desired LLM and set the corresponding API-key.

## Create a knowledge base

1. Click the **Knowledge Base** tab in the top middle of the page. 

2. Click **Create knowledge base** 

3. Input the name of your knowledge base and click **OK** to confirm.

4. Upload your dataset (document): Click **dataset** > **Add file** to upload and parse your files.

   _When the file parsing completes, its parsing status changes to **Success**._

   > For now, RAGFlow supports Word, slides, excel, txt, images, scanned copies, structured data, and web pages. The list is still expanding. 

5. You will now see the uploaded files are chunked. And you are also allowed to intervene with the chunking. 

## Set up an AI chat

Conversations in RAGFlow are based on a particular knowledge base or multiple knowledge bases.

1. Click the **Chat** tab in the middle top of the UI > **Create an assistant**.
2. In the popup window, you can configure your chats including configuring assistant, configuring prompt, and configuring model behaviors in the chat. 
3. You can now start an AI chat. 
