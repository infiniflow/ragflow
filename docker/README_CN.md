# README

<details open>
<summary></b>📗 目录</b></summary>

- 🐳 [Docker Compose](#-docker-compose)
- 🐬 [Docker 环境变量](#-docker-环境变量)
- 🐋 [服务配置](#-服务配置)
- 📋 [设置示例](#-设置示例)

</details>

## 🐳 Docker Compose

- **docker-compose.yml**  
  设置RAGFlow及其依赖项的环境。
- **docker-compose-base.yml**  
  设置RAGFlow的依赖项环境：Elasticsearch/[Infinity](https://github.com/infiniflow/infinity)、MySQL、MinIO和Redis。

> [!CAUTION]
> 我们不会主动维护 **docker-compose-CN-oc9.yml**、**docker-compose-macos.yml**，因此使用它们需要自担风险。不过，欢迎您提交pull request来改进这些文件。

## 🐬 Docker 环境变量

[.env](./.env) 文件包含Docker的重要环境变量。

### Elasticsearch

- `STACK_VERSION`  
  Elasticsearch的版本。默认为 `8.11.3`
- `ES_PORT`  
  用于将Elasticsearch服务暴露给主机的端口，允许**外部**访问Docker容器内运行的服务。默认为 `1200`。
- `ELASTIC_PASSWORD`  
  Elasticsearch的密码。

### Kibana

- `KIBANA_PORT`  
  用于将Kibana服务暴露给主机的端口，允许**外部**访问Docker容器内运行的服务。默认为 `6601`。
- `KIBANA_USER`  
  Kibana的用户名。默认为 `rag_flow`。
- `KIBANA_PASSWORD`  
  Kibana的密码。默认为 `infini_rag_flow`。

### 资源管理

- `MEM_LIMIT`  
  Docker容器在运行时可使用的最大内存量（字节）。默认为 `8073741824`。

### MySQL

- `MYSQL_PASSWORD`  
  MySQL的密码。
- `MYSQL_PORT`  
  用于将MySQL服务暴露给主机的端口，允许**外部**访问Docker容器内运行的MySQL数据库。默认为 `5455`。

### MinIO

- `MINIO_CONSOLE_PORT`  
  用于将MinIO控制台界面暴露给主机的端口，允许**外部**访问Docker容器内运行的Web控制台。默认为 `9001`
- `MINIO_PORT`  
  用于将MinIO API服务暴露给主机的端口，允许**外部**访问Docker容器内运行的MinIO对象存储服务。默认为 `9000`。
- `MINIO_USER`  
  MinIO的用户名。
- `MINIO_PASSWORD`  
  MinIO的密码。

### Redis

- `REDIS_PORT`  
  用于将Redis服务暴露给主机的端口，允许**外部**访问Docker容器内运行的Redis服务。默认为 `6379`。
- `REDIS_PASSWORD`  
  Redis的密码。

### RAGFlow

- `SVR_HTTP_PORT`  
  用于将RAGFlow的HTTP API服务暴露给主机的端口，允许**外部**访问Docker容器内运行的服务。默认为 `9380`。
- `RAGFLOW-IMAGE`  
  Docker镜像版本。默认为 `infiniflow/ragflow:v0.23.0`。RAGFlow Docker镜像不包含嵌入模型。

  
> [!TIP]  
> 如果无法下载RAGFlow Docker镜像，请尝试以下镜像源。  
> 
> - 对于 `nightly` 版本：  
>   - `RAGFLOW_IMAGE=swr.cn-north-4.myhuaweicloud.com/infiniflow/ragflow:nightly` 或者，
>   - `RAGFLOW_IMAGE=registry.cn-hangzhou.aliyuncs.com/infiniflow/ragflow:nightly`。

### 时区

- `TZ`  
  本地时区。默认为 `'Asia/Shanghai'`。

### Hugging Face 镜像站

- `HF_ENDPOINT`  
  huggingface.co的镜像站。如果无法访问主要的Hugging Face域名，可以取消注释此行。默认禁用。

### MacOS

- `MACOS`  
  针对macOS的优化。默认禁用。如果您的操作系统是macOS，可以取消注释此行。

### 最大文件大小

- `MAX_CONTENT_LENGTH`  
  每个上传文件的最大大小（字节）。如果需要更改128M的文件大小限制，可以取消注释此行。修改后，请确保相应更新nginx/nginx.conf中的`client_max_body_size`。

### 文档批量大小

- `DOC_BULK_SIZE`  
  在文档解析过程中单批处理的文档块数。默认为 `4`。

### 嵌入批量大小

- `EMBEDDING_BATCH_SIZE`  
  在嵌入向量化过程中单批处理的文本块数。默认为 `16`。

## 🐋 服务配置

[service_conf.yaml](./service_conf.yaml) 指定RAGFlow的系统级配置，由其API服务器和任务执行器使用。在Docker化环境中，此文件会根据 [service_conf.yaml.template](./service_conf.yaml.template) 文件自动创建（将所有环境变量替换为其值）。

- `ragflow`
  - `host`: Docker容器内API服务器的IP地址。默认为 `0.0.0.0`。
  - `port`: Docker容器内API服务器的端口。默认为 `9380`。

- `mysql`
  - `name`: MySQL数据库名称。默认为 `rag_flow`。
  - `user`: MySQL的用户名。
  - `password`: MySQL的密码。
  - `port`: Docker容器内MySQL服务的端口。默认为 `3306`。
  - `max_connections`: MySQL数据库的最大并发连接数。默认为 `100`。
  - `stale_timeout`: 超时时间（秒）。

- `minio`
  - `user`: MinIO的用户名。
  - `password`: MinIO的密码。
  - `host`: Docker容器内MinIO服务的IP**和**端口。默认为 `minio:9000`。

- `oceanbase`
  - `scheme`: 连接方案。设置为 `mysql` 使用mysql配置，或使用其他值使用以下配置。
  - `config`:
    - `db_name`: OceanBase数据库名称。
    - `user`: OceanBase的用户名。
    - `password`: OceanBase的密码。
    - `host`: OceanBase服务的主机名。
    - `port`: OceanBase的端口。

- `oss`
  - `access_key`: 用于向OSS服务认证请求的访问密钥ID。
  - `secret_key`: 用于向OSS服务认证请求的密钥。
  - `endpoint_url`: OSS服务端点的URL。
  - `region`: 存储桶所在的OSS区域。
  - `bucket`: 存储文件的OSS存储桶名称。当您想将所有文件存储在指定的存储桶中时，需要此配置项。
  - `prefix_path`: 可选。文件名的前缀路径，可以帮助组织存储桶内的文件。

- `s3`:
  - `access_key`: 用于向S3服务认证请求的访问密钥ID。
  - `secret_key`: 用于向S3服务认证请求的密钥。
  - `endpoint_url`: S3兼容服务端点的URL。使用S3兼容协议而非默认的AWS S3端点时需要此配置。
  - `bucket`: 存储文件的S3存储桶名称。当您想将所有文件存储在指定的存储桶中时，需要此配置项。
  - `region`: S3存储桶所在的AWS区域。这对于将请求定向到正确的数据中心很重要。
  - `signature_version`: 可选。用于认证请求的签名版本。常见版本包括 `v4`。
  - `addressing_style`: 可选。S3端点的寻址样式。可以是 `path` 或 `virtual`。
  - `prefix_path`: 可选。文件名的前缀路径，可以帮助组织存储桶内的文件。

- `oauth`  
  使用第三方账户注册或登录RAGFlow的OAuth配置。
  - `<channel>`: 自定义渠道ID。
    - `type`: 认证类型，选项包括 `oauth2`、`oidc`、`github`。当提供 `issuer` 参数时，默认为 `oidc`，否则默认为 `oauth2`。
    - `icon`: 图标ID，选项包括 `github`、`sso`，默认为 `sso`。
    - `display_name`: 渠道名称，默认为渠道ID的Title Case格式。
    - `client_id`: 必需，分配给客户端应用程序的唯一标识符。
    - `client_secret`: 必需，客户端应用程序的密钥，用于与认证服务器通信。
    - `authorization_url`: 获取用户授权的基础URL。
    - `token_url`: 用于交换授权码并获取访问令牌的URL。
    - `userinfo_url`: 用于获取用户信息（用户名、电子邮件等）的URL。
    - `issuer`: 身份提供者的基础URL。OIDC客户端可以通过 `issuer` 动态获取身份提供者的元数据（`authorization_url`、`token_url`、`userinfo_url`）。
    - `scope`: 请求的权限范围，空格分隔的字符串。例如 `openid profile email`。
    - `redirect_uri`: 必需，认证流程中授权服务器重定向的URI，用于返回结果。必须与向认证服务器注册的回调URI匹配。格式：`https://your-app.com/v1/user/oauth/callback/<channel>`。对于本地配置，可以直接使用 `http://127.0.0.1:80/v1/user/oauth/callback/<channel>`。

- `user_default_llm`  
  新RAGFlow用户使用的默认LLM。默认禁用。要启用此功能，请取消注释 **service_conf.yaml.template** 中的相应行。  
  - `factory`: LLM供应商。可用选项：
    - `"OpenAI"`
    - `"DeepSeek"`
    - `"Moonshot"`
    - `"Tongyi-Qianwen"`
    - `"VolcEngine"`
    - `"ZHIPU-AI"`
  - `api_key`: 指定LLM的API密钥。您需要在线申请模型API密钥。

> [!TIP]  
> 如果不在此处设置默认LLM，请在RAGFlow UI的**设置**页面配置默认LLM。


## 📋 设置示例

### 🔒 HTTPS 设置

#### 前置条件

- 指向您服务器的已注册域名
- 服务器上打开端口80和443
- 已安装Docker和Docker Compose

#### 获取和配置证书（Let's Encrypt）

如果您希望实例可以通过 `https` 访问，请按照以下步骤操作：

1. **安装Certbot并获取证书**
   ```bash
   # Ubuntu/Debian
   sudo apt update && sudo apt install certbot
   
   # CentOS/RHEL
   sudo yum install certbot
   
   # 获取证书（替换为您的实际域名）
   sudo certbot certonly --standalone -d your-ragflow-domain.com
   ```

2. **定位您的证书**  
   生成后，您的证书将位于：
   - 证书：`/etc/letsencrypt/live/your-ragflow-domain.com/fullchain.pem`
   - 私钥：`/etc/letsencrypt/live/your-ragflow-domain.com/privkey.pem`

3. **更新docker-compose.yml**  
   在 `docker-compose.yml` 的 `ragflow` 服务中添加证书卷：
   ```yaml
   services:
     ragflow:
       # ...现有配置...
       volumes:
         # SSL证书
         - /etc/letsencrypt/live/your-ragflow-domain.com/fullchain.pem:/etc/nginx/ssl/fullchain.pem:ro
         - /etc/letsencrypt/live/your-ragflow-domain.com/privkey.pem:/etc/nginx/ssl/privkey.pem:ro
         # 切换到HTTPS nginx配置
         - ./nginx/ragflow.https.conf:/etc/nginx/conf.d/ragflow.conf
         # ...其他现有卷...
  
   ```

4. **更新nginx配置**  
   编辑 `nginx/ragflow.https.conf`，将 `my_ragflow_domain.com` 替换为您的实际域名。

5. **重启服务**
   ```bash
   docker-compose down
   docker-compose up -d
   ```


> [!IMPORTANT]
> - 确保您的域名的DNS A记录指向服务器的IP地址
> - 在使用 `--standalone` 获取证书之前，停止端口80/443上运行的任何服务

> [!TIP]  
> 对于开发或测试，您可以使用自签名证书，但浏览器会显示安全警告。

#### 替代方案：使用现有证书

如果您已有其他提供商提供的SSL证书：

1. 将证书放在Docker可访问的目录中
2. 更新 `docker-compose.yml` 中的卷路径，指向您的证书文件
3. 确保证书文件包含完整的证书链
4. 按照上面的Let's Encrypt指南中的步骤4-5操作
