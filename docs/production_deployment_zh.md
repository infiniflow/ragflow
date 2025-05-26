# RAGFlow 生产环境部署指南

## 概述

RAGFlow 已从开发模式（FastAPI开发服务器）切换到生产模式（Gunicorn WSGI服务器）。本文档说明如何在生产环境中部署RAGFlow。

## 生产模式特性

- ✅ 使用 Gunicorn WSGI 服务器替代开发服务器
- ✅ 多进程工作模式，提升并发性能
- ✅ 自动重启机制，防止内存泄漏
- ✅ 完整的错误日志和访问日志
- ✅ 生产级别的安全和性能配置
- ✅ 保留所有原有的初始化操作

## Docker 部署（推荐）

### 自动生产模式

使用Docker时，`entrypoint.sh` 会自动使用Gunicorn启动生产模式：

```bash
# 默认启动（自动使用生产模式）
docker run -d ragflow:latest

# 自定义worker数量
docker run -d -e GUNICORN_WORKERS=8 ragflow:latest

# 自定义主机和端口
docker run -d -e RAGFLOW_HOST_IP=0.0.0.0 -e RAGFLOW_HOST_PORT=9380 ragflow:latest
```

### 环境变量配置

- `GUNICORN_WORKERS`: Gunicorn worker进程数（默认: 4）
- `RAGFLOW_HOST_IP`: 绑定IP地址（默认: 0.0.0.0）
- `RAGFLOW_HOST_PORT`: 绑定端口（默认: 9380）

## 手动部署

### 方式一：使用WSGI应用（推荐）

```bash
# 安装依赖
pip install -r requirements.txt

# 使用Gunicorn启动
gunicorn --config conf/gunicorn.conf.py api.wsgi:application

# 或者使用命令行参数
gunicorn --workers 4 --bind 0.0.0.0:9380 --preload api.wsgi:application
```

### 方式二：使用配置文件

```bash
# 使用自定义配置文件
gunicorn --config /path/to/your/gunicorn.conf.py api.wsgi:application
```

### 方式三：开发模式（不推荐生产环境）

```bash
# 仅用于开发和调试
python api/ragflow_server.py
```

## 配置说明

### Gunicorn 配置文件

位置：`conf/gunicorn.conf.py`

主要配置项：
- `workers`: 根据CPU核心数自动计算
- `worker_class`: 同步工作模式
- `timeout`: 120秒超时
- `max_requests`: 1000请求后重启worker
- `preload_app`: 预加载应用提升性能

### 性能调优

1. **Worker数量**：通常设置为 `CPU核心数 × 2 + 1`
2. **内存使用**：每个worker大约需要200-500MB内存
3. **超时设置**：根据实际请求处理时间调整
4. **连接数**：根据并发需求调整

## 监控和日志

### 日志配置

- 访问日志：输出到stdout，包含详细的请求信息
- 错误日志：输出到stderr，包含异常和错误信息
- 应用日志：RAGFlow应用的业务日志

### 健康检查

```bash
# 检查服务状态
curl http://localhost:9380/health

# 检查API版本
curl http://localhost:9380/api/v1/version
```

## 常见问题

### Q: 如何验证是否在生产模式运行？

A: 查看日志输出，生产模式会显示：
```
RAGFlow Gunicorn server is ready. Production mode active.
```

### Q: 如何调整worker数量？

A: 设置环境变量 `GUNICORN_WORKERS` 或修改配置文件中的 `workers` 参数。

### Q: 开发模式和生产模式的区别？

A: 
- 开发模式：使用Werkzeug开发服务器，单进程，有调试功能
- 生产模式：使用Gunicorn WSGI服务器，多进程，优化性能和稳定性

### Q: 如何回退到开发模式？

A: 直接运行 `python api/ragflow_server.py`，但不推荐在生产环境使用。

## 安全建议

1. 使用反向代理（如Nginx）处理静态文件和SSL
2. 设置合适的防火墙规则
3. 定期更新依赖包
4. 监控系统资源使用情况
5. 配置日志轮转避免磁盘空间不足

## 故障排除

### 启动失败

1. 检查端口是否被占用
2. 检查权限设置
3. 查看错误日志
4. 验证依赖包安装

### 性能问题

1. 调整worker数量
2. 检查内存使用情况
3. 优化数据库连接
4. 监控网络延迟

更多问题请参考项目文档或提交Issue。 