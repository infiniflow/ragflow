"""Sufficiency judge prompt — 5-way verdict with claim-level assessment."""

SUFFICIENCY_JUDGE_PROMPT = """你是信息检索充分性判断专家。判断当前收集的证据是否足够回答问题。

问题: {question}

声明级证据:
{evidence_summary}

判断:
1. 逐条评估每个声明是否被充分验证
2. 全局判断是否足够回答用户问题
3. 如果不够，给出定向反馈

输出格式(JSON):
{
    "status": "SUFFICIENT" | "USEFUL_BUT_INCOMPLETE" | "INSUFFICIENT" | "UNANSWERABLE",
    "score": 0.85,
    "claim_assessments": [
        {
            "claim_id": "c1",
            "is_verified": true,
            "confidence": 0.95,
            "reason": "在3个chunk中找到一致数据"
        }
    ],
    "missing": ["c2部分数据未找到"],
    "feedback": "建议对c2使用web_search补充最新数据",
    "overall_reason": "主要事实已覆盖，部分细节需补充"
}
"""
