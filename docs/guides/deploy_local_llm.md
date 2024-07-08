---
sidebar_position: 5
slug: /deploy_local_llm
---

# Deploy a local LLM

RAGFlow supports deploying models locally using Ollama or Xinference. If you have locally deployed models to leverage or wish to enable GPU or CUDA for inference acceleration, you can bind Ollama or Xinference into RAGFlow and use either of them as a local "server" for interacting with your local models.

RAGFlow seamlessly integrates with Ollama and Xinference, without the need for further environment configurations. You can use them to deploy two types of local models in RAGFlow: chat models and embedding models.

:::tip NOTE
This user guide does not intend to cover much of the installation or configuration details of Ollama or Xinference; its focus is on configurations inside RAGFlow. For the most current information, you may need to check out the official site of Ollama or Xinference.
:::

## Deploy a local model using Ollama

[Ollama](https://github.com/ollama/ollama) enables you to run open-source large language models that you deployed locally. It bundles model weights, configurations, and data into a single package, defined by a Modelfile, and optimizes setup and configurations, including GPU usage.

:::note
- For information about downloading Ollama, see [here](https://github.com/ollama/ollama?tab=readme-ov-file#ollama).
- For information about configuring Ollama server, see [here](https://github.com/ollama/ollama/blob/main/docs/faq.md#how-do-i-configure-ollama-server).
- For a complete list of supported models and variants, see the [Ollama model library](https://ollama.com/library).
:::

To deploy a local model, e.g., **Llama3**, using Ollama: 

### 1. Check firewall settings

Ensure that your host machine's firewall allows inbound connections on port 11434. For example:
   
```bash
sudo ufw allow 11434/tcp
```
### 2. Ensure Ollama is accessible

Restart system and use curl or your web browser to check if the service URL of your Ollama service at `http://localhost:11434` is accessible.
   
```bash
Ollama is running
```

### 3. Run your local model

```bash
ollama run llama3
```
<details>
  <summary>If your Ollama is installed through Docker, run the following instead:</summary>

   ```bash
   docker exec -it ollama ollama run llama3
   ```
</details>

### 4. Add Ollama

In RAGFlow, click on your logo on the top right of the page **>** **Model Providers** and add Ollama to RAGFlow: 

![add ollama](https://github.com/infiniflow/ragflow/assets/93570324/10635088-028b-4b3d-add9-5c5a6e626814)


### 5. Complete basic Ollama settings

In the popup window, complete basic settings for Ollama:

1. Because **llama3** is a chat model, choose **chat** as the model type.
2. Ensure that the model name you enter here *precisely* matches the name of the local model you are running with Ollama.
3. Ensure that the base URL you enter is accessible to RAGFlow.
4. OPTIONAL: Switch on the toggle under **Does it support Vision?** if your model includes an image-to-text model.

:::caution NOTE
- If your Ollama and RAGFlow run on the same machine, use `http://localhost:11434` as base URL.
- If your Ollama and RAGFlow run on the same machine and Ollama is in Docker, use `http://host.docker.internal:11434` as base URL. 
- If your Ollama runs on a different machine from RAGFlow, use `http://<IP_OF_OLLAMA_MACHINE>:11434` as base URL. 
:::

:::danger WARNING
If your Ollama runs on a different machine, you may also need to set the `OLLAMA_HOST` environment variable to `0.0.0.0` in **ollama.service** (Note that this is *NOT* the base URL):

```bash
Environment="OLLAMA_HOST=0.0.0.0"
```

See [this guide](https://github.com/ollama/ollama/blob/main/docs/faq.md#how-do-i-configure-ollama-server) for more information.
:::

:::caution WARNING
Improper base URL settings will trigger the following error:
```bash
Max retries exceeded with url: /api/chat (Caused by NewConnectionError('<urllib3.connection.HTTPConnection object at 0xffff98b81ff0>: Failed to establish a new connection: [Errno 111] Connection refused'))
```
:::

### 6. Update System Model Settings

Click on your logo **>** **Model Providers** **>** **System Model Settings** to update your model: 
   
*You should now be able to find **llama3** from the dropdown list under **Chat model**.*

> If your local model is an embedding model, you should find your local model under **Embedding model**.

### 7. Update Chat Configuration

Update your chat model accordingly in **Chat Configuration**:

> If your local model is an embedding model, update it on the configruation page of your knowledge base.

## Deploy a local model using Xinference

Xorbits Inference([Xinference](https://github.com/xorbitsai/inference)) enables you to unleash the full potential of cutting-edge AI models.

:::note
- For information about installing Xinference Ollama, see [here](https://inference.readthedocs.io/en/latest/getting_started/).
- For a complete list of supported models, see the [Builtin Models](https://inference.readthedocs.io/en/latest/models/builtin/).
:::

To deploy a local model, e.g., **Mistral**, using Xinference:

### 1. Check firewall settings

Ensure that your host machine's firewall allows inbound connections on port 9997. 

### 2. Start an Xinference instance

```bash
$ xinference-local --host 0.0.0.0 --port 9997
```

### 3. Launch your local model

Launch your local model (**Mistral**), ensuring that you replace `${quantization}` with your chosen quantization method
:
```bash
$ xinference launch -u mistral --model-name mistral-v0.1 --size-in-billions 7 --model-format pytorch --quantization ${quantization}
```
### 4. Add Xinference

In RAGFlow, click on your logo on the top right of the page **>** **Model Providers** and add Xinference to RAGFlow: 

![add xinference](https://github.com/infiniflow/ragflow/assets/93570324/10635088-028b-4b3d-add9-5c5a6e626814)

### 5. Complete basic Xinference settings

Enter an accessible base URL, such as `http://<your-xinference-endpoint-domain>:9997/v1`. 

### 6. Update System Model Settings

Click on your logo **>** **Model Providers** **>** **System Model Settings** to update your model.
   
*You should now be able to find **mistral** from the dropdown list under **Chat model**.*

> If your local model is an embedding model, you should find your local model under **Embedding model**.

### 7. Update Chat Configuration

Update your chat model accordingly in **Chat Configuration**:

> If your local model is an embedding model, update it on the configruation page of your knowledge base.

## Deploy a local model using IPEX-LLM

IPEX-LLM([IPEX-LLM](https://github.com/intel-analytics/ipex-llm)) is a PyTorch library for running LLM on Intel CPU and GPU (e.g., local PC with iGPU, discrete GPU such as Arc, Flex and Max) with very low latency

To deploy a local model, eg., **Qwen2**, using IPEX-LLM, follow the steps below:

### 1. Check firewall settings

Ensure that your host machine's firewall allows inbound connections on port 11434. For example:
   
```bash
sudo ufw allow 11434/tcp
```

### 2. Install and Start Ollama serve using IPEX-LLM

#### 2.1 Install IPEX-LLM for Ollama

IPEX-LLM's support for `ollama` now is available for Linux system and Windows system.

Visit [Run llama.cpp with IPEX-LLM on Intel GPU Guide](https://github.com/intel-analytics/ipex-llm/blob/main/docs/mddocs/Quickstart/llama_cpp_quickstart.md), and follow the instructions in section [Prerequisites](https://github.com/intel-analytics/ipex-llm/blob/main/docs/mddocs/Quickstart/llama_cpp_quickstart.md#0-prerequisites) to setup and section [Install IPEX-LLM cpp](https://github.com/intel-analytics/ipex-llm/blob/main/docs/mddocs/Quickstart/llama_cpp_quickstart.md#1-install-ipex-llm-for-llamacpp) to install the IPEX-LLM with Ollama binaries.

**After the installation, you should have created a conda environment, named `llm-cpp` for instance, for running `ollama` commands with IPEX-LLM.**

#### 2.2 Initialize Ollama

Activate the `llm-cpp` conda environment and initialize Ollama by executing the commands below. A symbolic link to `ollama` will appear in your current directory.

- For **Linux users**:
  
  ```bash
  conda activate llm-cpp
  init-ollama
  ```

- For **Windows users**:

  Please run the following command with **administrator privilege in Miniforge Prompt**.

  ```cmd
  conda activate llm-cpp
  init-ollama.bat
  ```

> [!NOTE]
> If you have installed higher version `ipex-llm[cpp]` and want to upgrade your ollama binary file, don't forget to remove old binary files first and initialize again with `init-ollama` or `init-ollama.bat`.

**Now you can use this executable file by standard ollama's usage.**

#### 2.3 Run Ollama Serve

You may launch the Ollama service as below:

- For **Linux users**:

  ```bash
  export OLLAMA_NUM_GPU=999
  export no_proxy=localhost,127.0.0.1
  export ZES_ENABLE_SYSMAN=1
  source /opt/intel/oneapi/setvars.sh
  export SYCL_CACHE_PERSISTENT=1

  ./ollama serve
  ```

- For **Windows users**:

  Please run the following command in Miniforge Prompt.

  ```cmd
  set OLLAMA_NUM_GPU=999
  set no_proxy=localhost,127.0.0.1
  set ZES_ENABLE_SYSMAN=1
  set SYCL_CACHE_PERSISTENT=1

  ollama serve
  ```


> Please set environment variable `OLLAMA_NUM_GPU` to `999` to make sure all layers of your model are running on Intel GPU, otherwise, some layers may run on CPU.


> If your local LLM is running on Intel Arc™ A-Series Graphics with Linux OS (Kernel 6.2), it is recommended to additionaly set the following environment variable for optimal performance before executing `ollama serve`:
>
> ```bash
> export SYCL_PI_LEVEL_ZERO_USE_IMMEDIATE_COMMANDLISTS=1
> ```


> To allow the service to accept connections from all IP addresses, use `OLLAMA_HOST=0.0.0.0 ./ollama serve` instead of just `./ollama serve`.

The console will display messages similar to the following:

![](https://llm-assets.readthedocs.io/en/latest/_images/ollama_serve.png)

### 3. Pull and Run Ollama Model

Keep the Ollama service on and open another terminal and run `./ollama pull <model_name>` in Linux (`ollama.exe pull <model_name>` in Windows) to automatically pull a model. e.g. `qwen2:latest`:

![](https://llm-assets.readthedocs.io/en/latest/_images/ollama_pull.png)

#### Run Ollama Model

- For **Linux users**:
  ```bash
  ./ollama run qwen2:latest
  ```
  
- For **Windows users**:
  ```cmd
  ollama run qwen2:latest
  ```
### 4. Configure RAGflow to use IPEX-LLM accelerated Ollama

The confiugraiton follows the steps in 

Ollama Section 4 [Add Ollama](#4-add-ollama), 

Section 5 [Complete basic Ollama settings](#5-complete-basic-ollama-settings), 

Section 6 [Update System Model Settings](#6-update-system-model-settings), 

Section 7 [Update Chat Configuration](#7-update-chat-configuration)
