# Xinference

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/2c5e86a7-807b-4d29-bd2b-f73fb1018866" width="130"/>
</div>

Xorbits Inference([Xinference](https://github.com/xorbitsai/inference)) empowers you to unleash the full potential of cutting-edge AI models. 

## Install

- [pip install "xinference[all]"](https://inference.readthedocs.io/en/latest/getting_started/installation.html)
- [Docker](https://inference.readthedocs.io/en/latest/getting_started/using_docker_image.html)

To start a local instance of Xinference, run the following command:
```bash
$ xinference-local --host 0.0.0.0 --port 9997
```
## Launch Xinference

Decide which LLM you want to deploy ([here's a list for supported LLM](https://inference.readthedocs.io/en/latest/models/builtin/)), say, **mistral**.
Execute the following command to launch the model, remember to replace ${quantization} with your chosen quantization method from the options listed above:
```bash
$ xinference launch -u mistral --model-name mistral-v0.1 --size-in-billions 7 --model-format pytorch --quantization ${quantization}
```

## Use Xinference in RAGFlow

- Go to 'Settings > Model Providers > Models to be added > Xinference'.
    
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/bcbf4d7a-ade6-44c7-ad5f-0a92c8a73789" width="1300"/>
</div>

> Base URL: Enter the base URL where the Xinference service is accessible, like, `http://<your-xinference-endpoint-domain>:9997/v1`.

- Use Xinference Models.

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/b01fcb6f-47c9-4777-82e0-f1e947ed615a" width="530"/>
</div>
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/1763dcd1-044f-438d-badd-9729f5b3a144" width="530"/>
</div>