"""Route node prompt — classify query type."""

ROUTE_PROMPT = """分析以下问题，输出结构化的查询分析。

问题: {question}

请从以下维度分析:
1. 问题类型: factual(事实型)/comparative(比较型)/analytical(分析型)/procedural(步骤型)/exploratory(探索型)/verification(验证型)/summarization(总结型)
2. 是否需要分解为原子事实(即:是否需要分别检索多个独立信息才能回答): true/false
3. 建议使用的知识编译工具: null(无)/toc(文档目录)/graph(知识图谱)/wiki(编译的领域知识)

输出格式(JSON):
{
    "question_type": "comparative",
    "requires_decomposition": true,
    "suggests_compilation": null,
    "reasoning": "比较型问题，需要分解为两个独立事实和一个比较关系"
}
"""
