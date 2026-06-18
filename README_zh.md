<div align="center">
  <img src="web/public/logo-light.png" width="200" alt="MetaGrossAI logo">
  <h1>MetaGrossAI</h1>
</div>

## 💡 什么是 MetaGrossAI？
**MetaGrossAI** 是一款领先的检索增强生成 (RAG) 引擎，它将尖端的 RAG 与 Agent 功能相融合，为 LLM 打造了卓越的上下文层。它提供了一种简化的 RAG 工作流程，适用于任何规模的企业。借助融合的上下文引擎和预置的 Agent 模板，MetaGrossAI 使开发者能够极其高效和精准地将复杂数据转化为高保真、生产级的 AI 系统。

## 🌟 主要功能
### 🍭 **“高质量输入，高质量输出”**
- 基于深度文档理解，从格式复杂的非结构化数据中提取知识。
- 能够在无限的 Token 中寻找“数据大海中的针”。

### 🍱 **基于模板的文本分块**
- 智能且具备可解释性。
- 提供多种模板选项供选择。

### 🌱 **有理有据的引用，减少幻觉**
- 文本分块可视化，支持人工干预。
- 快速查看关键参考资料和可追溯的引用，以支持有理有据的回答。

### 🍔 **兼容异构数据源**
- 支持 Word、PPT、Excel、TXT、图片、扫描件、结构化数据、网页等。

### 🛀 **自动化且轻松的 RAG 工作流**
- 专为个人和大型企业量身定制的精简 RAG 编排。
- 可配置的 LLM 和嵌入模型。
- 多路召回与融合重排。
- 直观的 API，与业务无缝集成。

## 🎬 本地部署
### 📝 前置依赖
- CPU >= 4 核
- RAM >= 16 GB
- Disk >= 50 GB
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
- Python >= 3.13

### 🚀 启动服务器
1. 确保 `vm.max_map_count` >= 262144:
   ```bash
   $ sudo sysctl -w vm.max_map_count=262144
