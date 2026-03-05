# Xinference ASR 服务部署指南

Xinference 是一个支持多种 AI 模型的推理框架，支持 Whisper 等 ASR 模型，完全离线运行。

## 快速开始

### 1. 启动 Xinference 服务

```bash
cd docker/xinference
docker compose up -d
```

查看日志：
```bash
docker logs -f xinference-server
```

服务启动后，访问 Web UI：http://localhost:9997

### 2. 部署 Whisper 模型

#### 方法 1: 通过 Web UI 部署（推荐）

1. 打开浏览器访问 http://localhost:9997
2. 点击 **Launch Model**
3. 选择 **Audio** 类型
4. 选择 Whisper 模型：
   - `whisper-tiny`: 最快，适合测试（39M）
   - `whisper-base`: 平衡（74M）
   - `whisper-small`: 推荐，中文效果好（244M）
   - `whisper-medium`: 更好的效果（769M）
   - `whisper-large-v3`: 最佳效果，需要更多资源（1.5G）

5. 点击 **Launch** 开始部署

#### 方法 2: 通过 API 部署

```bash
# 部署 whisper-small（推荐用于中文）
curl -X POST http://localhost:9997/v1/models \
  -H "Content-Type: application/json" \
  -d '{
    "model_type": "audio",
    "model_name": "whisper-small",
    "model_engine": "transformers"
  }'
```

#### 方法 3: 通过命令行部署

```bash
# 进入容器
docker exec -it xinference-server bash

# 部署模型
xinference launch --model-name whisper-small --model-type audio
```

### 3. 验证模型部署

```bash
# 查看已部署的模型
curl http://localhost:9997/v1/models
```

### 4. 测试 ASR 功能

```bash
# 测试语音识别（需要准备 test.wav 文件）
curl -X POST http://localhost:9997/v1/audio/transcriptions \
  -H "Content-Type: multipart/form-data" \
  -F "file=@test.wav" \
  -F "model=whisper-small"
```

## 在 RAGFlow 中配置

### 步骤 1: 确保 Xinference 和模型已启动

访问 http://localhost:9997 确认 Whisper 模型状态为 **Running**

### 步骤 2: 在 RAGFlow 中配置

1. 登录 RAGFlow
2. 进入 **设置 (Settings)** > **模型设置 (Model Settings)**
3. 找到 **ASR** 配置项
4. 选择 **Xinference** 作为提供商
5. 配置如下：
   - **Base URL**: `http://localhost:9997`（**注意：不要加 /v1**）
     - 如果 RAGFlow 在 Docker 中，使用 `http://host.docker.internal:9997`
   - **API Key**: 留空或随便填（Xinference 默认不需要）
   - **Model**: `whisper-large-v3` （或你部署的其他 Whisper 模型名称）

### 步骤 3: 测试

在 RAGFlow 的对话界面中上传音频文件或使用语音输入功能。

## 网络配置

### 如果 RAGFlow 和 Xinference 都在 Docker 中

**选项 A: 使用 host.docker.internal**
```yaml
# RAGFlow 配置
ASR_BASE_URL=http://host.docker.internal:9997
```

**选项 B: 使用同一网络**
```bash
# 创建网络
docker network create ragflow-network

# 将 Xinference 连接到网络
docker network connect ragflow-network xinference-server

# 在 RAGFlow 中使用容器名
ASR_BASE_URL=http://xinference-server:9997
```

## 模型选择建议

| 模型 | 大小 | 速度 | 中文效果 | 推荐场景 |
|------|------|------|----------|----------|
| whisper-tiny | 39M | 最快 | 一般 | 快速测试 |
| whisper-base | 74M | 快 | 较好 | 轻量部署 |
| whisper-small | 244M | 中等 | 好 | **推荐，平衡性能和效果** |
| whisper-medium | 769M | 慢 | 很好 | 高质量需求 |
| whisper-large-v3 | 1.5G | 最慢 | 最好 | 最高质量，需要 GPU |

## 性能优化

### 使用 GPU 加速

如果有 NVIDIA GPU，取消 docker-compose.yml 中的 GPU 配置注释：

```yaml
deploy:
  resources:
    reservations:
      devices:
        - driver: nvidia
          count: 1
          capabilities: [gpu]
```

### 调整并发数

在启动命令中添加参数：

```yaml
command: xinference-local --host 0.0.0.0 --port 9997 --max-workers 4
```

## 故障排查

### 查看日志
```bash
docker logs xinference-server
```

### 检查模型状态
```bash
curl http://localhost:9997/v1/models
```

### 重新部署模型
```bash
# 删除模型
curl -X DELETE http://localhost:9997/v1/models/{model_uid}

# 重新部署
curl -X POST http://localhost:9997/v1/models \
  -H "Content-Type: application/json" \
  -d '{"model_type": "audio", "model_name": "whisper-small"}'
```

### 清理缓存
```bash
# 停止服务
docker compose down

# 清理模型缓存（如果需要重新下载）
rm -rf models/*

# 重新启动
docker compose up -d
```

## 支持的音频格式

Whisper 支持多种音频格式：
- WAV
- MP3
- M4A
- FLAC
- OGG
- 等等

推荐使用 16kHz 采样率的 WAV 格式以获得最佳效果。

## 离线部署说明

首次启动时，Xinference 会自动从 ModelScope 下载模型。如果需要完全离线：

1. 在有网络的环境中先启动一次，让模型下载完成
2. 模型会保存在 `./models` 目录
3. 将整个 `./models` 目录复制到离线环境
4. 在离线环境启动服务即可

## 更多信息

- Xinference 官方文档: https://inference.readthedocs.io/
- Whisper 模型介绍: https://github.com/openai/whisper
