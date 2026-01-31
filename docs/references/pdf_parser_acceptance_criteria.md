# RAGflow PDF 解析器对比评测：验收标准（Acceptance Criteria）

> 目标：以**用户视角**客观评估 RAGflow 中不同 PDF 解析器（DeepDOC / MinerU / Docling / DeepSeek-OCR2）在“解析→结构化→分块→检索/问答可用性”链路上的差异，产出**可复现、可量化**结论。

---

## 1. 为什么需要对比（统一抽象≠统一效果）
RAGflow 虽然在入口层对 PDF 解析器做了统一抽象（同一配置入口、统一返回 sections/tables 等），但解析器内部管线存在本质差异：

- 是否 OCR（扫描件/图片页）
- Layout 分割策略（多栏、标题、页眉页脚、脚注）
- 表格检测/结构还原能力（cells/merged cells）
- 阅读顺序恢复策略（reading order）
- 输出表示差异（markdown vs blocks、是否带坐标 tag）

因此对比评测必须站在**用户最终得到的产物**上：

- 解析产物：`sections/tables` / markdown / blocks
- 下游产物：`chunks`（分块文本、表格 chunk、图片描述 chunk）、引用定位信息（页码、bbox/位置 tag 等）
- 性能与稳定性：时间、资源、失败率

> 单元测试只证明“代码能跑/接口没断”，不等价于“解析更好”。验收必须基于端到端产物。

---

## 2. 评测范围与边界（必须明确）

### 2.1 In Scope
- 同一批 PDF：对每个 PDF 使用不同解析器进行解析
- 统一抽取与归一化：将不同解析器输出转换到统一的中间表示（见第 4 节）
- 指标计算：性能、覆盖度、结构质量、稳定性；以及基于 GT 的准确性（如有）
- 报告产出：每 PDF 对比 + 汇总统计 + 场景推荐/替换边界

### 2.2 Out of Scope（除非额外约定）
- LLM 问答质量（需要检索/生成端到端 A/B 测试与标注）
- OCR2 模型训练/微调
- 对解析输出做“人为修复/后处理优化”以抹平差异（评测阶段不允许做质量修复）

---

## 3. 环境与依赖验收（Hard Gate）

### 3.1 环境分级（必须选一个，且全程一致）

**Level A（推荐：允许跑全量四者，结果可排名）**
- OS：Linux x86_64（Docker/裸机均可）
- GPU：NVIDIA ≥ 24GB VRAM（单卡固定）
- RAM：≥ 64GB
- Disk：≥ 200GB
- CUDA / Driver：与 torch/flash-attn 匹配

**Level B（可排名，但 DeepSeek-OCR2 必须走远程 HTTP GPU 服务）**
- OS：Linux/macOS
- CPU：≥ 16 cores
- RAM：≥ 32GB
- DeepSeek-OCR2：配置统一的 `DEEPSEEK_OCR2_API_URL`

**Level C（仅做降级对比，不允许宣称“四者客观排名”）**
- 仅 CPU、本地无 GPU 且无远程 OCR2
- 结论必须写：OCR2 Not Runnable，不参与排名

### 3.2 版本锁定（必须提供）
- RAGflow 代码 commit（含 worktree 分支 commit）
- 每个解析器的版本（pip 版本 / git commit / docker image digest）
- OCR2 模型版本（HF revision / sha）
- 依赖锁定文件：`pip freeze` 或 `uv.lock` 或 `requirements.lock`（至少一种）

### 3.3 配置冻结（必须记录）
对每个解析器必须固定并记录：
- page_from/page_to、DPI、语言、并发数
- OCR 模式（开启/关闭）、max_tokens
- OCR2：prompt、base_size/image_size/crop_mode、device、dtype

> Hard Gate：任何解析器若运行条件不满足（依赖缺失 / 不可访问 API / GPU 不足），必须标为 **Not Runnable**，不得进入排名。

---

## 4. 统一输出规范（Normalized Output Spec）

### 4.1 为什么要 Normalized
不同解析器原始输出形态差异极大（markdown / blocks / json / 带坐标 tag 的文本）。如果直接用原始输出计算指标，会把“输出格式差异”误当成“解析能力差异”。

### 4.2 NormalizedDocument（最小可行规范）
每个解析器对每个 PDF 必须产出一个 `NormalizedDocument`：

```jsonc
{
  "meta": {
    "file": "xxx.pdf",
    "size_mb": 0.62,
    "pages": 14,
    "parser": "DeepDOC|MinerU|Docling|DeepSeek-OCR2",
    "parser_version": "...",
    "run_env": {"os": "...", "gpu": "..."},
    "config_hash": "...",
    "run_id": "..."
  },
  "pages": [
    {
      "page_no": 1,
      "text": "...",              // 统一清洗后的纯文本
      "markdown": "...",           // 可选：若原始输出为 markdown
      "blocks": [
        {
          "type": "title|paragraph|list|table|figure|equation|other",
          "text": "...",
          "order": 1,
          "bbox": [x0, y0, x1, y1]  // 可选：有就填
        }
      ],
      "tables": [
        {
          "cells": [["a","b"],["c","d"]], // 或 html 二选一
          "bbox": [x0, y0, x1, y1]
        }
      ]
    }
  ]
}
```

### 4.3 归一化规则（必须一致）
- UTF-8 编码
- 换行统一为 `\n`
- 空白规范（连续空格/空行压缩规则必须写明）
- 去除坐标 tag/控制符（如 `@@...##` 这类 tag 必须剥离到 bbox 字段或直接去掉）
- 页眉页脚处理策略必须统一（保留或删除：二选一，且全解析器一致）

> Hard Gate：若某解析器无法生成 NormalizedDocument（输出为空 / 解析异常 / 页成功率不足），该解析器对该 PDF 记为 Fail。

---

## 5. Ground Truth（GT）与“准确性”声明边界

### 5.1 GT 是什么
GT（Ground Truth）指**人工标注的黄金标准**：对抽样页标注“正确文本/标题/表格/阅读顺序”。

### 5.2 没有 GT 能说什么，不能说什么
- ✅ 可以客观对比：覆盖度、结构数量、稳定性、性能
- ❌ 不允许宣称：谁“更准确/识别率更高”

> 结论：要谈准确性，必须做 GT；否则只能做“结构与覆盖”报告。

---

## 6. 可复现指标体系（Metrics）

### 6.1 可运行性（Gate）
- `success_rate = success_pages / total_pages`
- `fail_pages` 与错误原因分类

建议 Gate：`success_rate >= 0.95` 才允许参与该 PDF 的排名（阈值可调整并写入报告）。

### 6.2 性能（自动、全量）
- `time_total_s`
- `time_per_page_s`
- `peak_rss_mb`
- `gpu_peak_vram_mb`（如适用）
- `throughput_chars_per_s`

### 6.3 覆盖度（自动、全量）
- `text_chars_total`
- `non_empty_pages_ratio`
- `unique_chars_ratio`（避免重复/乱码）

### 6.4 结构质量（自动、全量）
- `title_count`、`paragraph_count`、`list_count`
- `table_count`、`avg_table_cells`、空表比例
- `reading_order_proxy`（仅 proxy：例如多栏页块顺序一致性；真正 reading order 需 GT）

### 6.5 准确性（需要 GT，抽样）
抽样策略：按页类型分层（纯文本页/多栏页/表格页/目录页）各抽 N 页。

- 文本：CER/WER 或 Levenshtein distance
- 表格：TEDS
- 阅读顺序：ROED 或人工 1-5 分

---

## 7. 评测流程（必须可复现）

### 7.1 单个 PDF 流程
1) 记录 PDF 元信息（页数、大小、是否扫描件）
2) 每解析器运行 **3 次**（同配置）
3) 保存：原始输出、NormalizedDocument、日志、资源指标
4) 计算自动指标（性能/覆盖/结构/稳定性）
5) 抽样页做 GT（如要做准确性）

### 7.2 失败处理（必须严格）
- Fail 定义：异常退出 / 输出为空 / success_rate 不达标
- Fail 的解析器：不参与该 PDF 排名，只统计失败原因
- 禁止“0 chars 但标 success”这类假成功

### 7.3 稳定性要求
同一解析器同一 PDF 的 3 次运行：
- `time_total_s` 变异系数 CV < 0.2
- `text_chars_total` 差异 < 2%
不满足则标记为“不稳定”，报告中必须提示风险。

---

## 8. 评分与决策规则（可选，但必须透明）
若需要总分，必须声明权重与门槛：
- 可运行性：Gate（不计分但决定是否参赛）
- 性能：20%
- 覆盖/结构：30%
- 准确性（有 GT 才能计）：50%

> 没有 GT 时，准确性维度不得计分，也不得用于“更准”的结论。

---

## 9. 最终交付物（验收产出）

### 9.1 总报告（Markdown/PDF）
必须包含：
- 环境与版本（硬件、镜像、commit、依赖锁）
- 配置冻结（各解析器关键参数）
- 指标定义（公式、阈值、权重）
- 结果总览（均值、方差、失败率）
- 分 PDF 详细表格与关键样例页对比
- 结论：场景推荐 + 替换边界 + 风险

### 9.2 数据包（可复现证据）
- 原始输出（每解析器每 PDF）
- NormalizedDocument JSON
- metrics CSV/JSON
- 运行日志
- GT 标注（如果做准确性）

### 9.3 一键复现命令
- Docker / 本地命令（含环境变量）
- 生成报告命令

---

## 10. 通过/不通过（最终验收结论口径）

**通过**必须同时满足：
1) 环境分级明确且满足 Level A/B 的“可排名”条件
2) 4 个解析器均可运行（或明确 Not Runnable 的解析器不参与排名，并在结论中排除）
3) 输出归一化成功（NormalizedDocument 完整）
4) 指标可复现（重复运行波动在阈值内）
5) 报告中所有“准确性”结论都有 GT 支撑（否则不允许出现“更准确”措辞）

**不通过**任一条触发：
- 某解析器输出为空却被算作 success
- 依赖/环境未锁定导致不可复现
- 使用不同输出形态直接比较导致指标失真

---

## 附：实施建议（下一步落地）
1) 在 RAGflow 统一入口（例如 `rag/app/naive.py`）调用不同 parser，获取 sections/tables
2) 实现 adapter：`parser_output -> NormalizedDocument`
3) 实现 metrics：读取 NormalizedDocument 计算全量自动指标
4) 可选：构建 GT 标注工具/格式（JSONL），计算 CER/TEDS/ROED
