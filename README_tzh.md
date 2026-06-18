<div align="center">
  <img src="web/public/logo-light.png" width="200" alt="MetaGrossAI logo">
  <h1>MetaGrossAI</h1>
</div>

## 💡 什麼是 MetaGrossAI？
**MetaGrossAI** 是一款領先的檢索增強生成 (RAG) 引擎，它將尖端的 RAG 與 Agent 功能相融合，為 LLM 打造了卓越的上下文層。它提供了一種簡化的 RAG 工作流程，適用於任何規模的企業。借助融合的上下文引擎和預置的 Agent 模板，MetaGrossAI 使開發者能夠極其高效和精準地將複雜數據轉化為高保真、生產級的 AI 系統。

## 🌟 主要功能
### 🍭 **“高質量輸入，高質量輸出”**
- 基於深度文檔理解，從格式複雜的非結構化數據中提取知識。
- 能夠在無限的 Token 中尋找“數據大海中的針”。

### 🍱 **基於模板的文本分塊**
- 智能且具備可解釋性。
- 提供多種模板選項供選擇。

### 🌱 **有理有據的引用，減少幻覺**
- 文本分塊可視化，支持人工干預。
- 快速查看關鍵參考資料和可追溯的引用，以支持有理有據的回答。

### 🍔 **兼容異構數據源**
- 支持 Word、PPT、Excel、TXT、圖片、掃描件、結構化數據、網頁等。

### 🛀 **自動化且輕鬆的 RAG 工作流**
- 專為個人和大型企業量身定制的精簡 RAG 編排。
- 可配置的 LLM 和嵌入模型。
- 多路召回與融合重排。
- 直觀的 API，與業務無縫集成。

## 🎬 本地部署
### 📝 前置依賴
- CPU >= 4 核
- RAM >= 16 GB
- Disk >= 50 GB
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
- Python >= 3.13

### 🚀 啟動伺服器
1. 確保 `vm.max_map_count` >= 262144:
   ```bash
   $ sudo sysctl -w vm.max_map_count=262144
