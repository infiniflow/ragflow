# RAGFlow Docker 配置指南

## 模型下载目录设置

### 1. 创建本地模型目录

在 Windows PowerShell 中执行：
```powershell
mkdir -p E:\github_myrepo\ragflow\ragflow\docker_download\deepdoc
```

### 2. 修改 docker-compose.yml

编辑 `E:\github_myrepo\ragflow\ragflow\docker\docker-compose.yml`，在 `ragflow-cpu` 服务的 `volumes` 部分添加：

```yaml
volumes:
  - ./ragflow-logs:/ragflow/logs
  - ./service_conf.yaml.template:/ragflow/conf/service_conf.yaml.template
  - ./entrypoint.sh:/ragflow/entrypoint.sh
  # 添加自定义模型目录挂载
  - E:/github_myrepo/ragflow/ragflow/docker_download/deepdoc:/ragflow/rag/res/deepdoc
```

### 3. 启动服务

```powershell
cd E:\github_myrepo\ragflow\ragflow\docker

# 启动基础服务
docker compose -f docker-compose-base.yml up -d

# 启动 RAGFlow
docker compose -f docker-compose.yml --profile cpu up -d
```

### 4. 验证模型加载

```powershell
# 查看容器日志
docker logs docker-ragflow-cpu-1

# 进入容器检查模型
docker exec -it docker-ragflow-cpu-1 ls -la /ragflow/rag/res/deepdoc/
```

## 注意事项

1. **首次启动**：容器会自动下载模型到挂载的目录
2. **后续启动**：使用已下载的模型，无需重复下载
3. **模型文件**：包括 det.onnx, rec.onnx, layout.onnx, tsr.onnx, updown_concat_xgb.model

## 手动下载模型（可选）

如果自动下载失败，可以手动下载：

```powershell
# 设置 HuggingFace 镜像
$env:HF_ENDPOINT = "https://hf-mirror.com"

# 使用 huggingface-cli 下载
huggingface-cli download InfiniFlow/deepdoc --local-dir E:\github_myrepo\ragflow\ragflow\docker_download\deepdoc
```

## 访问 RAGFlow

启动后访问：http://localhost

默认端口：
- Web UI: 80
- API: 9380
- Admin: 9381
