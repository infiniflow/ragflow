---
sidebar_position: 5
slug: /deploy_local_llm
---

# Deploy a local LLM

RAGFlow supports deploying LLMs locally using Ollama or Xinference.

## Ollama

One-click deployment of local LLMs, that is [Ollama](https://github.com/ollama/ollama).

### Install

- [Ollama on Linux](https://github.com/ollama/ollama/blob/main/docs/linux.md)
- [Ollama Windows Preview](https://github.com/ollama/ollama/blob/main/docs/windows.md)
- [Docker](https://hub.docker.com/r/ollama/ollama)

### Launch Ollama

Decide which LLM you want to deploy ([here's a list for supported LLM](https://ollama.com/library)), say, **mistral**:
```bash
$ ollama run mistral
```
Or,
```bash
$ docker exec -it ollama ollama run mistral
```

### Use Ollama in RAGFlow

- Go to 'Settings > Model Providers > Models to be added > Ollama'.
    
![](https://github.com/infiniflow/ragflow/assets/12318111/a9df198a-226d-4f30-b8d7-829f00256d46)

> Base URL: Enter the base URL where the Ollama service is accessible, like, `http://<your-ollama-endpoint-domain>:11434`.

- Use Ollama Models.

![](https://github.com/infiniflow/ragflow/assets/12318111/60ff384e-5013-41ff-a573-9a543d237fd3)

## Xinference

Xorbits Inference([Xinference](https://github.com/xorbitsai/inference)) empowers you to unleash the full potential of cutting-edge AI models. 

### Install

- [pip install "xinference[all]"](https://inference.readthedocs.io/en/latest/getting_started/installation.html)
- [Docker](https://inference.readthedocs.io/en/latest/getting_started/using_docker_image.html)

To start a local instance of Xinference, run the following command:
```bash
$ xinference-local --host 0.0.0.0 --port 9997
```
### Launch Xinference

Decide which LLM you want to deploy ([here's a list for supported LLM](https://inference.readthedocs.io/en/latest/models/builtin/)), say, **mistral**.
Execute the following command to launch the model, remember to replace `${quantization}` with your chosen quantization method from the options listed above:
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