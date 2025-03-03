# RAGFlow 数据持久化指南

本文档介绍如何在RAGFlow项目中处理数据持久化、备份和恢复。

## 目录
1. [数据持久化概述](#数据持久化概述)
2. [支持的持久化数据](#支持的持久化数据)
3. [自动备份和恢复系统](#自动备份和恢复系统)
4. [手动备份和恢复](#手动备份和恢复)
5. [故障排除](#故障排除)

## 数据持久化概述

RAGFlow使用Docker容器化技术部署各种服务，为了确保数据不会在容器重启或删除时丢失，我们利用了Docker的数据卷(volumes)和绑定挂载(bind mounts)机制。

- **Docker卷**: 由Docker管理的持久化数据存储，如MySQL和Elasticsearch的数据
- **绑定挂载**: 将主机系统目录直接映射到容器内，用于配置文件和日志

## 支持的持久化数据

RAGFlow的Docker配置支持以下服务的持久化数据：

| 服务 | 存储方式 | 存储内容 | 卷/目录名称 |
|-----|---------|---------|-----------|
| MySQL | Docker卷 | 数据库文件 | ragflow_mysql_data 或 [项目名]_mysql_data |
| Elasticsearch | Docker卷 | 索引和文档 | ragflow_esdata01 或 [项目名]_esdata01 |
| MinIO | Docker卷 | 对象存储 | ragflow_minio_data 或 [项目名]_minio_data |
| Redis | Docker卷 | 键值数据 | ragflow_redis_data 或 [项目名]_redis_data |
| RAGFlow服务 | Docker卷 | 应用数据 | ragflow_data |
| RAGFlow配置 | 绑定挂载 | 配置文件 | ./data/conf |
| RAGFlow日志 | 绑定挂载 | 日志文件 | ./data/ragflow-logs |

## 自动备份和恢复系统

RAGFlow现在包含一个全自动的备份和恢复系统，可以在容器生命周期的关键时刻自动执行数据管理。

### 自动化工具脚本

以下是可用的脚本文件：

- `docker_startup.ps1`: 主控制脚本，整合了备份和恢复功能
- `backup_volumes.ps1`: 备份脚本，提供全面的数据备份功能，支持自动检测项目卷名称
- `auto_restore.ps1`: 自动恢复脚本，可在新容器启动时应用

### 使用方法

主脚本 `docker_startup.ps1` 支持以下命令：

```powershell
# 启动容器并尝试自动恢复数据（默认动作）
./docker_startup.ps1 start

# 停止容器前自动备份数据
./docker_startup.ps1 stop

# 重启容器，包括备份和恢复过程
./docker_startup.ps1 restart

# 仅执行备份，不停止容器
./docker_startup.ps1 backup

# 显示帮助信息
./docker_startup.ps1 help
```

### 卷名称自动检测

备份脚本现在能够智能检测Docker卷的名称，无论它们使用何种命名约定。支持的命名方式包括：

- 使用项目名称作为前缀（如 `ragflow_mysql_data`）
- 直接使用服务名称（如 `mysql_data`）
- 使用自定义前缀（将自动检测）

这确保了脚本可以适应不同的部署环境和Docker Compose配置。

### 备份存储位置

备份文件默认存储在 `D:\Docker_Backups` 目录下，并按以下结构组织：

```
D:\Docker_Backups\
  ├── raw_volumes\       # Docker卷的原始备份
  │   ├── ragflow_mysql_data_2023-07-30_14-30-00.tar.gz
  │   ├── ragflow_esdata01_2023-07-30_14-30-00.tar.gz
  │   └── ...
  ├── readable_data\     # 可读格式的数据导出
  │   ├── mysql\
  │   │   └── mysql_dump_2023-07-30_14-30-00.sql
  │   └── ...
  └── metadata\          # 备份元数据
      ├── backup_metadata_2023-07-30_14-30-00.json
      └── latest_backup.json
```

## 手动备份和恢复

除了自动化工具外，您还可以手动执行备份和恢复操作。

### 手动备份

1. 运行备份脚本：

```powershell
./backup_volumes.ps1 [备份目录路径]
```

2. 或使用Docker命令备份单个卷：

```powershell
docker run --rm -v ragflow_mysql_data:/source -v D:/my-backups:/backup alpine tar -czf /backup/mysql-backup.tar.gz -C /source ./
```

### 手动恢复

1. 运行自动恢复脚本：

```powershell
./auto_restore.ps1 [备份目录路径]
```

2. 或使用Docker命令恢复单个卷：

```powershell
docker run --rm -v ragflow_mysql_data:/destination -v D:/my-backups/mysql-backup.tar.gz:/backup.tar.gz alpine sh -c "rm -rf /destination/* && tar -xzf /backup.tar.gz -C /destination"
```

## 故障排除

### 常见问题

1. **卷数据丢失**
   - 确保未使用 `docker compose down -v` 命令，该命令会删除卷
   - 尝试使用最新备份进行恢复

2. **找不到备份文件**
   - 检查默认备份路径 `D:\Docker_Backups`
   - 查看备份脚本的运行日志

3. **恢复失败**
   - 确保Docker正在运行
   - 检查备份文件的完整性
   - 查看恢复脚本生成的日志

4. **卷名称不匹配**
   - 备份脚本会自动检测可能的卷名称
   - 如果仍有问题，检查Docker Compose配置中的卷定义

### 卷管理命令参考

```powershell
# 列出所有Docker卷
docker volume ls

# 检查卷的详细信息
docker volume inspect ragflow_mysql_data

# 清理未使用的卷（谨慎使用）
docker volume prune
```

## 注意事项

- 建议定期执行备份操作，可考虑设置计划任务
- 避免使用 `docker compose down -v` 命令，除非您明确知道这会删除所有持久化数据
- 对于重要数据，建议保存多个备份副本并测试恢复流程 