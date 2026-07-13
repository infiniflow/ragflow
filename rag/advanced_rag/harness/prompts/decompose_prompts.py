"""Planner decompose prompts — one per question type."""

DECOMPOSE_FACTUAL = """这是一个事实型问题。列出需要检索的所有原子事实。
如果有多个事实，逐条列出；如果只有一个事实，只输出1条。

问题: {question}
最大声明数: {max_claims}
详细程度: {detail_level}

输出格式(JSON):
{
    "claims": [
        {
            "claim_id": "c1",
            "description": "苹果收购Beats的年份",
            "priority": 1
        }
    ]
}
"""


DECOMPOSE_COMPARATIVE = """这是一个比较型问题。需要分解为:
1. 实体A关于比较维度的信息
2. 实体B关于比较维度的信息
3. (可选) 直接比较两者的信息

问题: {question}
最大声明数: {max_claims}
详细程度: {detail_level}

输出格式(JSON):
{
    "claims": [
        {
            "claim_id": "c1",
            "description": "杭州到北京的距离",
            "priority": 1
        },
        {
            "claim_id": "c2",
            "description": "上海到北京的距离",
            "priority": 1
        },
        {
            "claim_id": "c3",
            "description": "哪个城市离北京更近",
            "priority": 2
        }
    ]
}
"""


DECOMPOSE_PROCEDURAL = """这是一个步骤型问题。请分解为完成此操作所需的每个步骤。

问题: {question}
最大声明数: {max_claims}
详细程度: {detail_level}

输出格式(JSON):
{
    "claims": [
        {
            "claim_id": "c1",
            "description": "第一步所需的信息",
            "priority": 1
        },
        {
            "claim_id": "c2",
            "description": "第二步所需的信息",
            "priority": 2
        }
    ]
}
"""


DECOMPOSE_EXPLORATORY = """这是一个分析/探索型问题。请分解为需要研究的多个方面/维度。

问题: {question}
最大声明数: {max_claims}
详细程度: {detail_level}

输出格式(JSON):
{
    "claims": [
        {
            "claim_id": "c1",
            "description": "需要研究的第一个方面",
            "priority": 1
        },
        {
            "claim_id": "c2",
            "description": "需要研究的第二个方面",
            "priority": 2
        }
    ]
}
"""
