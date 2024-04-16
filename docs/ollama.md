# Ollama

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/2019e7ee-1e8a-412e-9349-11bbf702e549" width="130"/>
</div>

One-click deployment of local LLMs, that is [Ollama](https://github.com/ollama/ollama).

## Install

- [Ollama on Linux](https://github.com/ollama/ollama/blob/main/docs/linux.md)
- [Ollama Windows Preview](https://github.com/ollama/ollama/blob/main/docs/windows.md)
- [Docker](https://hub.docker.com/r/ollama/ollama)

## Launch Ollama

Decide which LLM you want to deploy ([here's a list for supported LLM](https://ollama.com/library)), say, **mistral**:
```bash
$ ollama run mistral
```
Or,
```bash
$ docker exec -it ollama ollama run mistral
```

## Use Ollama in RAGFlow

- Go to 'Settings > Model Providers > Models to be added > Ollama'.
    
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/a9df198a-226d-4f30-b8d7-829f00256d46" width="1300"/>
</div>

> Base URL: Enter the base URL where the Ollama service is accessible, like, `http://<your-ollama-endpoint-domain>:11434`.

- Use Ollama Models.

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/60ff384e-5013-41ff-a573-9a543d237fd3" width="530"/>
</div>