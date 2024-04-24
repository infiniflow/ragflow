# 常见问题解答

<p>
  <a href="./faq.md">English</a> |
  <a href="./faq_zh.md">简体中文</a>
</p>

## 常规

### 1. RAGFlow与其他RAG产品有何不同？

尽管大型语言模型（LLMs）显著提高了自然语言处理（NLP）的水平，但“垃圾进垃圾出”的现状仍未改变。为此，RAGFlow引入了两个与其它检索增强生成（RAG）产品不同的独特功能。

- 细粒度文档解析：文档解析涉及图像和表格，并允许您根据需要进行干预。
- 可追溯答案减少幻觉：您可以信任RAGFlow的响应，因为您可以查看支持它们的引用和参考资料。

### 2. RAGFlow支持哪些语言？

- 目前支持英语、简体中文和繁体中文。

## 性能

### 1. 为什么RAGFlow解析文档的时间比LangChain长？

我们在文档预处理任务上投入了大量努力，如布局分析、表格结构识别和使用我们的视图模型进行OCR（光学字符识别）。这导致了所需的额外时间。

### 2. 为什么RAGFlow需要比其他项目更多的资源？

RAGFlow内置了多个文档结构解析模型，这需要额外的计算资源。

## 功能

### 1. RAGFlow支持哪些架构或设备？

目前，我们仅支持x86 CPU和Nvidia GPU。

### 2. 你们是否提供与第三方应用程序集成的API？

对应的API现已可用。有关更多信息，请查看 [Conversation API](./conversation_api.md)。

### 3. 你们是否支持流输出？

不，此功能仍在开发中。欢迎贡献。

### 4. 是否可以通过URL共享对话？

是的，此功能现已可用。

### 5. 是否支持多轮对话，即引用先前的对话作为当前对话的上下文？

此功能及相关API仍在开发中。欢迎贡献。


## 故障排除

### 1. Docker镜像问题

#### 1.1　由于RAGFlow更新迭代速度快，建议从头构建镜像。

```
$ git clone https://github.com/infiniflow/ragflow.git
$ cd ragflow
$ docker build -t infiniflow/ragflow:v0.3.0 .
$ cd ragflow/docker
$ chmod +x ./entrypoint.sh
$ docker compose up -d
```

#### 1.2 `process "/bin/sh -c cd ./web && npm i && npm run build"`失败

1. 检查Docker内的网络，例如：
```bash
curl https://hf-mirror.com
```

2. 如果网络工作正常，问题可能出在Docker网络配置上。相应地调整Docker构建：
```
# Original：
docker build -t infiniflow/ragflow:v0.3.0 .
# Current：
docker build -t infiniflow/ragflow:v0.3.0 . --network host
```

### 2. 与huggingface模型相关的问题

#### 2.1. `MaxRetryError: HTTPSConnectionPool(host='hf-mirror.com', port=443)`

此错误表明您没有互联网访问权限或无法连接到hf-mirror.com。尝试以下操作：

1. 手动从[huggingface.co/InfiniFlow/deepdoc](https://huggingface.co/InfiniFlow/deepdoc)下载资源文件到本地文件夹 **~/deepdoc**。
2. 在**docker-compose.yml**中添加卷，例如：
```
- ~/deepdoc:/ragflow/rag/res/deepdoc
```

#### 2.2 `FileNotFoundError: [Errno 2] No such file or directory: '/root/.cache/huggingface/hub/models--InfiniFlow--deepdoc/snapshots/FileNotFoundError: [Errno 2] No such file or directory: '/ragflow/rag/res/deepdoc/ocr.res'be0c1e50eef6047b412d1800aa89aba4d275f997/ocr.res'`

1. 检查Docker内部的网络，例如： 
```bash
curl https://hf-mirror.com
```
2. 运行`ifconfig`检查`mtu`值。如果服务器的`mtu`为`1450`，而容器中NIC的`mtu`为`1500`，这种不匹配可能导致网络不稳定。按以下方式调整`mtu`策略：

```
vim docker-compose-base.yml
# Original configuration：
networks:
  ragflow:
    driver: bridge
# Modified configuration：
networks:
  ragflow:
    driver: bridge
    driver_opts:
      com.docker.network.driver.mtu: 1450
```

### 3. RAGFlow服务器问题

#### 3.1 `WARNING: can't find /raglof/rag/res/borker.tm`

忽略此警告并继续。所有系统警告都可以忽略。

#### 3.2 `network anomaly There is an abnormality in your network and you cannot connect to the server.`

![anomaly](https://github.com/infiniflow/ragflow/assets/93570324/beb7ad10-92e4-4a58-8886-bfb7cbd09e5d)

除非服务器完全初始化，否则您将无法登录RAGFlow。运行`docker logs -f ragflow-server`。

*如果系统显示以下内容，则服务器已成功初始化：*

```
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


### 4. RAGFlow后端服务问题

#### 4.1 `dependency failed to start: container ragflow-mysql is unhealthy`

`dependency failed to start: container ragflow-mysql is unhealthy`意味着您的MySQL容器未能启动。如果mysql无法启动，请尝试在 **docker-compose-base.yml** 中将`mysql:5.7.18`替换为`mariadb:10.5.8`。

#### 4.2 `Realtime synonym is disabled, since no redis connection`

忽略此警告并继续。所有系统警告都可以忽略。

![](https://github.com/infiniflow/ragflow/assets/93570324/ef5a6194-084a-4fe3-bdd5-1c025b40865c)

#### 4.3 解析一个2MB的文档为什么要这么久？

由于服务器资源有限，解析请求必须在队列中等待。我们目前正在增强我们的算法并增加计算能力。

#### 4.4 为什么我的文档解析在不到百分之一的地方停滞不前？

![stall](https://github.com/infiniflow/ragflow/assets/93570324/3589cc25-c733-47d5-bbfc-fedb74a3da50)

如果您在 *本地* 部署了RAGFlow，请尝试以下操作：

1. 检查您的RAGFlow服务器日志，看看它是否运行正常：
```bash
docker logs -f ragflow-server
```
2. 检查 **tast_executor.py** 进程是否存在。
3. 检查您的RAGFlow服务器是否可以访问hf-mirror.com或huggingface.com。


#### 4.5 `Index failure`

索引失败通常表示Elasticsearch服务不可用。

#### 4.6 如何查看RAGFlow的日志？

```bash
tail -f path_to_ragflow/docker/ragflow-logs/rag/*.log
```

#### 4.7 如何检查RAGFlow中每个组件的状态？

```bash
$ docker ps
```
*如果您的所有RAGFlow组件都运行正常，系统将显示以下内容：* 

```
5bc45806b680   infiniflow/ragflow:v0.3.0     "./entrypoint.sh"        11 hours ago   Up 11 hours               0.0.0.0:80->80/tcp, :::80->80/tcp, 0.0.0.0:443->443/tcp, :::443->443/tcp, 0.0.0.0:9380->9380/tcp, :::9380->9380/tcp   ragflow-server
91220e3285dd   docker.elastic.co/elasticsearch/elasticsearch:8.11.3   "/bin/tini -- /usr/l…"   11 hours ago   Up 11 hours (healthy)     9300/tcp, 0.0.0.0:9200->9200/tcp, :::9200->9200/tcp           ragflow-es-01
d8c86f06c56b   mysql:5.7.18        "docker-entrypoint.s…"   7 days ago     Up 16 seconds (healthy)   0.0.0.0:3306->3306/tcp, :::3306->3306/tcp     ragflow-mysql
cd29bcb254bc   quay.io/minio/minio:RELEASE.2023-12-20T01-00-02Z       "/usr/bin/docker-ent…"   2 weeks ago    Up 11 hours      0.0.0.0:9001->9001/tcp, :::9001->9001/tcp, 0.0.0.0:9000->9000/tcp, :::9000->9000/tcp     ragflow-minio
```

#### 4.8 `Exception: Can't connect to ES cluster`

1. 检查您的Elasticsearch组件的状态：

```bash
$ docker ps
```
   *您的RAGFlow中的 'healthy' Elasticsearch组件的状态应如下所示：*
```
91220e3285dd   docker.elastic.co/elasticsearch/elasticsearch:8.11.3   "/bin/tini -- /usr/l…"   11 hours ago   Up 11 hours (healthy)     9300/tcp, 0.0.0.0:9200->9200/tcp, :::9200->9200/tcp           ragflow-es-01
```

2. 如果您的容器持续重启，请确保`vm.max_map_count` >= 262144，如本[README](https://github.com/infiniflow/ragflow?tab=readme-ov-file#-start-up-the-server)所述。如果您希望更改永久生效，需要在 **/etc/sysctl.conf** 中更新`vm.max_map_count`的值。此配置仅适用于Linux。


3. 如果您的问题仍然存在，请确保ES主机设置正确：

    - 如果您使用Docker运行RAGFlow，它在 **docker/service_conf.yml** 中。设置如下：
    ```
    es:
      hosts: 'http://es01:9200'
    ```
    - 如果您在Docker之外运行RAGFlow，请使用以下方式在 **conf/service_conf.yml** 中验证ES主机设置：: 
    ```bash
    curl http://<IP_OF_ES>:<PORT_OF_ES>
    ```


#### 4.9 `{"data":null,"retcode":100,"retmsg":"<NotFound '404: Not Found'>"}`

 您的IP地址或端口号可能不正确。如果您使用默认配置，请在浏览器中输入http://（**不要使用 `localhost`，不要使用9380，也不需要端口号！**）。这应该就可以正常工作了。

#### 4.10 `Ollama - Mistral instance running at 127.0.0.1:11434 but cannot add Ollama as model in RagFlow`

添加模型到Ollama的正确Ollama IP地址和端口至关重要：

- 如果您在demo.ragflow.io上，请确保托管Ollama的服务器具有可公开访问的IP地址。注意127.0.0.1不是可公开访问的IP地址。
- 如果您在本地部署RAGFlow，请确保Ollama和RAGFlow在同一个局域网中，并且可以相互通信。

#### 4.11 你们是否提供使用deepdoc解析PDF或其他文件的示例？

是的，我们确实提供了。请参阅**rag/app**文件夹下的Python文件。

#### 4.12 为什么我无法将10MB+的文件上传到本地部署的RAGFlow？

您可能忘记了更新**MAX_CONTENT_LENGTH**环境变量：

1.  在**ragflow/docker/.env**中添加环境变量`MAX_CONTENT_LENGTH`：
```
MAX_CONTENT_LENGTH=100000000
```
2. 更新**docker-compose.yml**：
```
environment:
  - MAX_CONTENT_LENGTH=${MAX_CONTENT_LENGTH}
```
3. 重启RAGFlow服务器：
```
docker compose up ragflow -d
```
   *现在您应该能够上传小于100MB的文件。*

#### 4.13 `Table 'rag_flow.document' doesn't exist`

此异常发生在启动RAGFlow服务器时。尝试以下操作：

  1. 延长睡眠时间：转到**docker/entrypoint.sh**，找到第26行，并将`sleep 60`替换为`sleep 280`。
  2. 如果使用Windows，请确保**entrypoint.sh**的行尾是LF。
  3. 转到**docker/docker-compose.yml**，添加以下内容：
  ```
  ./entrypoint.sh:/ragflow/entrypoint.sh
  ```
  4. 更改目录：
  ```bash
  cd docker
  ```
  5. 停止RAGFlow服务器：
  ```bash
  docker compose stop
  ```
  6. 重新启动RAGFlow服务器：
  ```bash
  docker compose up
  ```

#### 4.14 `hint : 102  Fail to access model  Connection error`

![hint102](https://github.com/infiniflow/ragflow/assets/93570324/6633d892-b4f8-49b5-9a0a-37a0a8fba3d2)

1. 确保RAGFlow服务器可以访问基础URL。
2. 不要忘记在**http://IP:port**后面添加 **/v1/**：
   **http://IP:port/v1/**

   
## 使用

### 1. 如何增加RAGFlow响应的长度？

1. 右键单击所需的对话以显示**聊天配置**窗口。 
2. 切换到**模型设置**选项卡，并调整**最大令牌**滑块以获得所需的长度。 
3. 点击**确定**以确认您的更改。


### 2. 空响应是什么意思？如何设置它？

如果您的知识库中没有检索到任何内容，您可以通过在空响应中指定的内容来限制系统响应的内容。 如果您在空响应中没有指定任何内容，您将允许您的LLM即兴发挥，给它一个产生幻觉的机会。

### 3. 我可以在OpenAI的某个地方设置基础URL吗？

![](https://github.com/infiniflow/ragflow/assets/93570324/8cfb6fa4-8a97-415d-b9fa-b6f405a055f3)


### 4. 如何使用本地部署的LLM运行RAGFlow？

您可以使用Ollama来部署本地LLM。有关更多信息，请查看[此处](https://github.com/infiniflow/ragflow/blob/main/docs/ollama.md)。

### 5. 如何将ragflow和ollama服务器连接起来？

- 如果RAGFlow是本地部署的，确保您的RAGFlow和Ollama在同一个局域网中。
- 如果您使用我们的在线演示，请确保您的Ollama服务器的IP地址是公开可访问的。

### 6. 如何配置RAGFlow以响应100%匹配的结果，而不是使用LLM？

1. 点击页面中间顶部的 **知识库** 选项卡。 
2. 右键单击所需的知识库以显示 **配置** 对话框。 
3. 选择 **Q&A** 作为块方法，然后点击 **保存** 以确认您的更改。
