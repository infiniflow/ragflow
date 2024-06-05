---
sidebar_position: 5
slug: /deploy_local_llm
---

# Deploy a local LLM

RAGFlow supports deploying models locally using Ollama or Xinference. If you have locally deployed models to leverage or wish to enable GPU or CUDA for inference acceleration, you can bind Ollama or Xinference into RAGFlow and use either of them as a local "server" for interacting with your local models.

RAGFlow seamlessly integrates with Ollama and Xinference, without the need for further environment configurations. RAGFlow v0.7.0 supports running two types of local models: chat models and embedding models.

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

To deploy a local model, e.g., **7b-chat-v1.5-q4_0**, using Ollama: 

1. Ensure that the service URL of Ollama is accessible.
2. Run your local model: 

   ```bash
   ollama run qwen:7b-chat-v1.5-q4_0
   ```
<details>
  <summary>If your Ollama is installed through Docker, run the following instead:</summary>

   ```bash
   docker exec -it ollama ollama run qwen:7b-chat-v1.5-q4_0
   ```
</details>

3. In RAGFlow, click on your logo on the top right of the page **>** **Model Providers** and add Ollama to RAGFlow: 

   ![add llm](https://github.com/infiniflow/ragflow/assets/93570324/10635088-028b-4b3d-add9-5c5a6e626814)

4. In the popup window, complete basic settings for Ollama:

   - In this case, **qwen:7b-chat-v1.5-q4_0** is a chat model, so we choose **chat** as the model type. 
   - Ensure that the model name you enter here *precisely* matches the name of the local model you are running with Ollama.
   - Ensure that the base URL you enter is accessible to RAGFlow. 
   - OPTIONAL: Switch on the toggle under **Does it support Vision?**, if your model includes an image-to-text model.

![ollama settings](https://github.com/infiniflow/ragflow/assets/93570324/0ba3942e-27ba-457c-a26f-8ebe9edf0e52)

:::caution NOTE
- If your Ollama and RAGFlow run on the same machine, use `http://localhost:11434` as base URL.
- If your Ollama and RAGFlow run on the same machine and Ollama is in Docker, use `http://host.docker.internal:11434` as base URL. 
- If your Ollama runs on a different machine from RAGFlow, use `http://<IP_OF_OLLAMA_MACHINE>` as base URL. 
:::

:::danger WARNING
If your Ollama runs on a different machine, you may also need to update the system environments in **ollama.service**:

```bash
Environment="OLLAMA_HOST=0.0.0.0"
Environment="OLLAMA_MODELS=/APP/MODELS/OLLAMA"
```

See [here](https://github.com/ollama/ollama/blob/main/docs/faq.md#how-do-i-configure-ollama-server) for more information.
:::

5. Click on your logo **>** **Model Providers** **>** **System Model Settings** to update your model: 
   
   *You should now be able to find **7b-chat-v1.5-q4_0** from the dropdown list under **Chat model**.*

   > If your local model is an embedding model, you should find your local model under **Embedding model**.

![system model settings](https://github.com/infiniflow/ragflow/assets/93570324/c627fb16-785b-4b84-a77f-4dec604570ed)

6. In this case, update your chat model in **Chat Configuration**:

![chat config](https://github.com/infiniflow/ragflow/assets/93570324/7cec4026-a509-47a3-82ec-5f8e1f059442)

   > If your local model is an embedding model, update it on the configruation page of your knowledge base.

## Deploy a local model using Xinference

Xorbits Inference([Xinference](https://github.com/xorbitsai/inference)) empowers you to unleash the full potential of cutting-edge AI models. 

### Install

- [pip install "xinference[all]"](https://inference.readthedocs.io/en/latest/getting_started/installation.html)
- [Docker](https://inference.readthedocs.io/en/latest/getting_started/using_docker_image.html)

To start a local instance of Xinference, run the following command:
```bash
$ xinference-local --host 0.0.0.0 --port 9997
```
### Launch Xinference

Decide which LLM to deploy ([here's a list for supported LLM](https://inference.readthedocs.io/en/latest/models/builtin/)), say, **mistral**.
Execute the following command to launch the model, ensuring that you replace `${quantization}` with your chosen quantization method from the options listed above:
```bash
$ xinference launch -u mistral --model-name mistral-v0.1 --size-in-billions 7 --model-format pytorch --quantization ${quantization}
```

### Use Xinference in RAGFlow

- Go to 'Settings > Model Providers > Models to be added > Xinference'.
    
![](https://github.com/infiniflow/ragflow/assets/12318111/bcbf4d7a-ade6-44c7-ad5f-0a92c8a73789)

> Base URL: Enter the base URL where the Xinference service is accessible, like, `http://<your-xinference-endpoint-domain>:9997/v1`.

- Use Xinference Models.

![](https://github.com/infiniflow/ragflow/assets/12318111/b01fcb6f-47c9-4777-82e0-f1e947ed615a)
![](https://github.com/infiniflow/ragflow/assets/12318111/1763dcd1-044f-438d-badd-9729f5b3a144)