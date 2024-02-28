English | [简体中文](./README_zh.md)


## System Environment Preparation

### Install docker

If your machine doesn't have *Docker* installed, please refer to [Install Docker Engine](https://docs.docker.com/engine/install/)

### OS Setups
Inorder to run [ElasticSearch](https://www.elastic.co/guide/en/elasticsearch/reference/current/install-elasticsearch.html), 
you need to check the following command:
```bash
121:/ragflow# sysctl vm.max_map_count
vm.max_map_count = 262144
```
If **vm.max_map_count** is not larger  than 65535, please run the following commands:
```bash
121:/ragflow# sudo sysctl -w vm.max_map_count=262144
```
However, this change is not persistent and will be reset after a system reboot. 
To make the change permanent, you need to update the **/etc/sysctl.conf file**.
Add or update the following line in the file:
```bash
vm.max_map_count=262144
```

### Here we go!
> If you want to change the basic setups, like port, password .etc., please refer to [.env](./docker/.env) before starting the system.

> If you change anything in [.env](./docker/.env), please check [service_conf.yaml](./conf/service_conf.yaml) which is a 
> configuration of the back-end service and should be consistent with [.env](./docker/.env).

> - In [service_conf.yaml](./conf/service_conf.yaml), configuration of *LLM* in **user_default_llm** is strongly recommended. 
> In **user_default_llm** of [service_conf.yaml](./conf/service_conf.yaml), you need to specify LLM factory and your own _API_KEY_.
> It's O.K if you don't have _API_KEY_ at the moment, you can specify it later at the setting part after starting and logging in the system.
> - We have supported the flowing LLM factory, and the others is coming soon: 
> [OpenAI](https://platform.openai.com/login?launch), [通义千问/QWen](https://dashscope.console.aliyun.com/model), 
> [智普AI/ZhipuAI](https://open.bigmodel.cn/)
```bash
121:/ragflow# cd docker
121:/ragflow/docker# docker compose up 
```
If after a few minutes, it stops screening and halts like following picture, _**Hallelujah!**_ You have successfully launched the system.
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/7dc8b73f-7890-41b4-aa09-97a417cfd20b" width="1000"/>
</div>