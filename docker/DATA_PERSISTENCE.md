# RAGFlow Docker 操作指南

这份文档提供了详细的RAGFlow Docker环境管理流程，从初始设置到日常操作，再到数据备份和恢复。

## 目录

1. [环境要求](#环境要求)
2. [初始设置](#初始设置)
3. [标准操作流程](#标准操作流程)
4. [数据管理](#数据管理)
5. [故障排除](#故障排除)

## 环境要求

- Windows 10/11 操作系统
- Docker Desktop 已安装并配置
- PowerShell 5.0 或更高版本
- 最低 8GB 内存（推荐 16GB 或更高）
- 最低 50GB 可用磁盘空间

## 初始设置

### 步骤 1：克隆或获取RAGFlow代码库

确保您已经有RAGFlow项目代码并切换到正确的目录：

```powershell
cd D:\Hippo\PycharmProjects\RAGFLOW\ragflow\docker>
```

### 步骤 2：检查配置文件

首次运行前，请检查以下配置文件：

1. `.env` - 包含所有密码和环境变量
2. `docker-compose.yml` - 定义容器配置
3. `service_conf.yaml.template` - 服务配置模板

### 步骤 3：确保Docker Desktop正在运行

打开Docker Desktop并确保其正常运行，或使用我们的启动脚本自动启动Docker。

## 标准操作流程

### 启动流程

在`docker`目录中，执行以下命令启动容器：

```powershell
.\docker_startup.ps1 start
```

此命令将执行以下操作：

1. **检查Docker状态**：如果Docker未运行，尝试启动它
2. **尝试恢复数据**：执行`auto_restore.ps1`，从备份恢复卷数据（如果存在）
3. **启动容器**：使用`docker compose -p ragflow -f docker-compose.yml up -d`启动所有容器
4. **显示成功消息**：确认容器已启动并提供访问URL

> **注意**：所有容器和卷将使用前缀`ragflow`自动创建，便于识别和管理。

### 停止流程

要停止容器并备份数据：

```powershell
.\docker_startup.ps1 stop
```

此命令将执行以下操作：

1. **备份数据**：执行`backup_volumes.ps1`备份所有卷和绑定挂载的数据
2. **停止容器**：使用`docker compose -p ragflow -f docker-compose.yml down`停止所有容器

### 重启流程

若要重启容器（包括自动备份和恢复）：

```powershell
.\docker_startup.ps1 restart
```

此命令将执行：
1. 备份当前数据
2. 停止容器
3. 恢复数据
4. 重新启动容器

### 手动备份

不停止容器的情况下执行数据备份：

```powershell
.\docker_startup.ps1 backup
```

## 数据管理

### 备份策略

系统使用以下策略管理数据备份：

1. **默认备份位置**：`D:\Docker_Backups`
2. **备份组织**：
   - `raw_volumes/` - 存储原始卷备份（TAR格式）
   - `readable_data/` - 存储可读取的导出数据（如MySQL导出）
   - `metadata/` - 存储备份元数据（用于恢复）

### 备份内容

每次备份将保存：

1. **Docker卷**：
   - `ragflow_mysql_data` - MySQL数据库
   - `ragflow_elasticsearch_data` - ElasticSearch数据
   - `ragflow_minio_data` - MinIO对象存储
   - `ragflow_redis_data` - Redis缓存数据
   - `ragflow_ragflow_data` - 应用程序数据

2. **绑定挂载**：
   - `./data/ragflow-logs` - 应用日志
   - 其他配置目录

3. **可读数据**：
   - MySQL数据库导出

### 恢复流程详解

自动恢复过程中，`auto_restore.ps1`执行以下步骤：

1. **检查备份**：查找最新的备份元数据
2. **创建卷**：如果卷不存在则创建
3. **恢复数据**：从备份还原到相应卷
4. **处理绑定挂载**：恢复绑定挂载数据（如日志）

## 故障排除

### 常见问题 

1. **Docker未运行错误**
   - 确保Docker Desktop已启动
   - 检查Docker服务状态

2. **卷映射错误**
   - 检查卷名是否与备份匹配
   - 验证项目名称前缀是否正确

3. **数据未恢复**
   - 检查备份目录是否存在：`D:\Docker_Backups`
   - 检查备份元数据文件是否存在
   - 验证卷名称与备份中的匹配

4. **密码问题**
   - 默认密码存储在`.env`文件中
   - 常用默认密码：`infini_rag_flow`

### 命令参考

| 命令 | 描述 | 使用场景 |
|------|------|---------|
| `.\docker_startup.ps1 start` | 启动容器并恢复数据 | 首次使用或重新启动系统 |
| `.\docker_startup.ps1 stop` | 备份数据并停止容器 | 结束使用或系统维护 |
| `.\docker_startup.ps1 restart` | 备份、停止、恢复并重启 | 配置更改或系统刷新 |
| `.\docker_startup.ps1 backup` | 仅备份数据 | 定期数据保护 |
| `.\docker_startup.ps1 help` | 显示帮助信息 | 需要查看选项 |

### 日志访问

日志文件位于以下位置：
- 备份日志：`D:\Docker_Backups\backup_log.txt`
- 恢复日志：`D:\Docker_Backups\restore_log.txt`
- 应用日志：`./data/ragflow-logs/`

## 最佳实践

1. **定期备份**：建议设置定期备份任务
2. **在系统更改前备份**：进行任何配置修改前执行备份
3. **保留多个备份**：系统自动使用时间戳创建多个备份版本
4. **测试恢复过程**：定期测试数据恢复功能

希望这份文档能帮助您更好地理解和管理RAGFlow Docker环境。如有任何问题，请参考项目文档或联系支持团队。 