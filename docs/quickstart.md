# Quickstart

RAGFlow is an open-source RAG (Retrieval-Augmented Generation) engine based on deep document understanding. When integrated with LLMs, it is capable of providing truthful question-answering capabilities, backed by well-founded citations from various complex formatted data.

This quick start guide describes a general process from:

- Starting up a local RAGFlow server, 
- Creating a knowledge base, 
- Intervening with file parsing, to 
- Establishing an AI chat based on your datasets. 

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

   > - With default settings, you only need to enter `http://IP_OF_YOUR_MACHINE` (**sans** port number) as the default HTTP serving port `80` can be omitted when using the default configurations.

## Configure LLMs

RAGFlow is a RAG solution, and it needs to work with an LLM to offer grounded, hallucination-free question-answering capabilities. For now, RAGFlow supports the following LLMs, and the list is expanding:

- OpenAI
- Tongyi-Qianwen
- Moonshot
- DeepSeek-V2

>  RAGFlow also supports deploying LLMs locally using Ollama or Xinference, but this part is not covered in this quick start guide. 

To add and configure an LLM: 

1. Click on your logo on the top right of the page **>** **Model Providers**:

   ![2 add llm](https://github.com/infiniflow/ragflow/assets/93570324/10635088-028b-4b3d-add9-5c5a6e626814)

   > Each RAGFlow account is able to use **text-embedding-v2** for free, a small embedding model of Tongyi-Qianwen. This is why you can see Tongyi-Qianwen in the **Added models** list. And you may need to update your Tongyi-Qianwen API key at a later point.

2. Click on the desired LLM and update the API key accordingly (DeepSeek-V2 in this case):

   ![update api key](https://github.com/infiniflow/ragflow/assets/93570324/4e5e13ef-a98d-42e6-bcb1-0c6045fc1666)

   *Your added models appear as follows:* 

   ![added available models](https://github.com/infiniflow/ragflow/assets/93570324/d08b80e4-f921-480a-b41d-11832489c916)

3. Click **System Mode Settings** to globally set the following models: 

   - Chat model, 
   - Embedding model, 
   - Image-to-text model. 

   ![system model settings](https://github.com/infiniflow/ragflow/assets/93570324/cdcc1da5-4494-44cd-ad5b-1222ed6acc3f)

> Some of the small models, such as the image-to-text model **qwen-vl-max**, are subsidiary to a particular LLM. And you may need to update your API key accordingly to use these models. 

## Create your first knowledge base

You are allowed to upload files to a knowledge base in RAGFlow and parse them into datasets. A knowledge base is virtually a collection of datasets. Question answering in RAGFlow can be based on a particular knowledge base or multiple knowledge bases. For now, RAGFlow supports Word, slides, excel, txt, images, scanned copies, structured data, and web pages. 

To create your first knowledge base:

1. Click the **Knowledge Base** tab in the top middle of the page **>** **Create knowledge base**.

2. Input the name of your knowledge base and click **OK** to confirm.

   _You are taken to the **Configuration** page of your knowledge base._

   ![knowledge base configuration](https://github.com/infiniflow/ragflow/assets/93570324/384c671a-8b9c-468c-b1c9-1401128a9b65)

3. Select the embedding model and chunk (parsing) method for your knowledge base, and click **Save** to confirm your change. 

   _You are taken to the **Dataset** page of your knowledge base._

4. Click **+ Add file** **>** **Local files** to start uploading a particular file to the knowledge base. 

5. In the uploaded file entry, click the play button to start file parsing:

   ![file parsing](https://github.com/infiniflow/ragflow/assets/93570324/19f273fa-0ab0-435e-bdf4-a47fb080a078)

   _When the file parsing completes, its parsing status changes to **SUCCESS**._

   

6. You will now see the uploaded files are chunked. And you are also allowed to intervene with the chunking. 

## Intervene with file parsing 

RAGFlow also features visibility and explainability. Users are allowed to view the chunking results and intervene where necessary. To do so: 

1. Click on the file that completes file parsing to view the chunking results: 

   

## Set up an AI chat

Conversations in RAGFlow are based on a particular knowledge base or multiple knowledge bases.

1. Click the **Chat** tab in the middle top of the UI > **Create an assistant**.
2. In the popup window, you can configure your chats including configuring assistant, configuring prompt, and configuring model behaviors in the chat. 
3. You can now start an AI chat. 
