# 搜索查询开始标记
BEGIN_SEARCH_QUERY = "<|begin_search_query|>"
# 搜索查询结束标记
END_SEARCH_QUERY = "<|end_search_query|>"
# 搜索结果开始标记
BEGIN_SEARCH_RESULT = "<|begin_search_result|>"
# 搜索结果结束标记
END_SEARCH_RESULT = "<|end_search_result|>"
# 最大搜索次数限制
MAX_SEARCH_LIMIT = 6

# 推理提示模板
REASON_PROMPT = (
        "你是一个具有执行数据集搜索能力的推理助手，可以帮助你准确回答用户问题。你有特殊工具：\n\n"
        f"- 执行搜索：写入 {BEGIN_SEARCH_QUERY} 你的查询内容 {END_SEARCH_QUERY}。\n"
        f"然后，系统将搜索并分析相关内容，并以 {BEGIN_SEARCH_RESULT} ...搜索结果... {END_SEARCH_RESULT} 格式为你提供有用信息。\n\n"
        f"如有必要，你可以多次重复搜索过程。最大搜索尝试次数限制为 {MAX_SEARCH_LIMIT}。\n\n"
        "一旦你获得所需的全部信息，继续你的推理。\n\n"
        "-- 示例 1 --\n" ########################################
        "问题：\"《大白鲨》和《皇家赌场》的导演是否来自同一个国家？\"\n"
        "助手：\n"
        f"    {BEGIN_SEARCH_QUERY}《大白鲨》的导演是谁？{END_SEARCH_QUERY}\n\n"
        "用户：\n"
        f"    {BEGIN_SEARCH_RESULT}\n《大白鲨》的导演是史蒂文·斯皮尔伯格...\n{END_SEARCH_RESULT}\n\n"
        "继续使用新信息进行推理。\n"
        "助手：\n"
        f"    {BEGIN_SEARCH_QUERY}史蒂文·斯皮尔伯格来自哪里？{END_SEARCH_QUERY}\n\n"
        "用户：\n"
        f"    {BEGIN_SEARCH_RESULT}\n史蒂文·艾伦·斯皮尔伯格是一位美国电影制作人...\n{END_SEARCH_RESULT}\n\n"
        "继续使用新信息进行推理...\n\n"
        "助手：\n"
        f"    {BEGIN_SEARCH_QUERY}《皇家赌场》的导演是谁？{END_SEARCH_QUERY}\n\n"
        "用户：\n"
        f"    {BEGIN_SEARCH_RESULT}\n《皇家赌场》是一部2006年的间谍电影，由马丁·坎贝尔执导...\n{END_SEARCH_RESULT}\n\n"
        "继续使用新信息进行推理...\n\n"
        "助手：\n"
        f"    {BEGIN_SEARCH_QUERY}马丁·坎贝尔来自哪里？{END_SEARCH_QUERY}\n\n"
        "用户：\n"
        f"    {BEGIN_SEARCH_RESULT}\n马丁·坎贝尔（生于1943年10月24日）是一位新西兰电影和电视导演...\n{END_SEARCH_RESULT}\n\n"
        "继续使用新信息进行推理...\n\n"
        "助手：\n回答问题已有足够信息\n"

        "-- 示例 2 --\n" #########################################
        "问题：\"craigslist的创始人何时出生？\"\n"
        "助手：\n"
        f"    {BEGIN_SEARCH_QUERY}谁是craigslist的创始人？{END_SEARCH_QUERY}\n\n"
        "用户：\n"
        f"    {BEGIN_SEARCH_RESULT}\nCraigslist由Craig Newmark创立...\n{END_SEARCH_RESULT}\n\n"
        "继续使用新信息进行推理。\n"
        "助手：\n"
        f"    {BEGIN_SEARCH_QUERY}Craig Newmark何时出生？{END_SEARCH_QUERY}\n\n"
        "用户：\n"
        f"    {BEGIN_SEARCH_RESULT}\nCraig Newmark出生于1952年12月6日...\n{END_SEARCH_RESULT}\n\n"
        "继续使用新信息进行推理...\n\n"
        "助手：\n回答问题已有足够信息\n"
        "**记住**：\n"
        f"- 你有一个可搜索的数据集，所以只需提供适当的搜索查询。\n"
        f"- 使用 {BEGIN_SEARCH_QUERY} 请求数据集搜索，并以 {END_SEARCH_QUERY} 结束。\n"
        "- 查询的语言必须与'问题'或'搜索结果'的语言相同。\n"
        "- 完成搜索后，继续你的推理。\n\n"
        '请回答以下问题。你应该逐步思考来解决它。\n\n'
    )

# 相关信息提取提示模板
RELEVANT_EXTRACTION_PROMPT = """**任务说明：**

    你的任务是根据以下输入阅读和分析网页：**先前推理步骤**、**当前搜索查询**和**搜索到的网页**。你的目标是从**搜索到的网页**中提取与**当前搜索查询**相关且有帮助的信息，并将这些信息无缝整合到**先前推理步骤**中，以继续为原始问题进行推理。

    **指南：**

    1. **分析搜索到的网页：**
    - 仔细审查每个搜索到的网页的内容。
    - 识别与**当前搜索查询**相关且能够帮助原始问题推理过程的事实信息。

    2. **提取相关信息：**
    - 从搜索到的网页中选择直接有助于推进**先前推理步骤**的信息。
    - 确保提取的信息准确且相关。

    3. **输出格式：**
    - **如果网页为当前搜索查询提供有用信息：** 以`**最终信息**`开头呈现信息，如下所示。
    - 查询的语言**必须**与'搜索查询'或'网页'的语言相同。\n"
    **最终信息**

    [有用的信息]

    - **如果网页没有为当前搜索查询提供任何有用信息：** 输出以下文本。

    **最终信息**

    未找到有用信息。

    **输入：**
    - **先前推理步骤：**
    {prev_reasoning}

    - **当前搜索查询：**
    {search_query}

    - **搜索到的网页：**
    {document}

    """