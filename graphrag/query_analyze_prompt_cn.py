# 这个文件定义了用于知识图谱查询分析的提示模板。主要包含两个关键提示：

# minirag_query2kwd - 用于从用户查询中识别答案类型和实体。它引导大语言模型从预定义的类型池中选择适当的答案类型，并从查询中提取关键实体。
# 该模板引导大语言模型从预定义的类型池中选择适当的答案类型

# keywords_extraction - 用于从查询中提取高级和低级关键词，高级关键词关注概念和主题，低级关键词关注具体实体和细节。
# 这些提示模板是知识图谱检索过程中查询理解和重写的基础，帮助系统更准确地理解用户意图并在知识图谱中找到相关信息。

# 根据MIT许可证授权
"""
参考:
 - [LightRag](https://github.com/HKUDS/LightRAG)
 - [MiniRAG](https://github.com/HKUDS/MiniRAG)
"""
PROMPTS = {}

PROMPTS["minirag_query2kwd"] = """---角色---

你是一个有用的助手，负责识别用户查询中的答案类型和低级关键词。

---目标---

给定查询，列出答案类型和低级关键词。
answer_type_keywords（答案类型关键词）专注于特定查询的答案类型，而low-level keywords（低级关键词）专注于特定实体、细节或具体术语。
answer_type_keywords必须从答案类型池中选择。
这个池以字典形式呈现，其中键代表你应该选择的类型，值代表示例样本。

---指令---

- 以JSON格式输出关键词。
- JSON应该有三个键:
  - "answer_type_keywords" 用于答案的类型。在这个列表中，可能性最高的类型应该放在最前面。不超过3个。
  - "entities_from_query" 用于特定实体或细节。必须从查询中提取。
######################
-示例-
######################
示例1:

查询: "国际贸易如何影响全球经济稳定性？"
答案类型池: {{
 'PERSONAL LIFE': ['FAMILY TIME', 'HOME MAINTENANCE'],
 'STRATEGY': ['MARKETING PLAN', 'BUSINESS EXPANSION'],
 'SERVICE FACILITATION': ['ONLINE SUPPORT', 'CUSTOMER SERVICE TRAINING'],
 'PERSON': ['JANE DOE', 'JOHN SMITH'],
 'FOOD': ['PASTA', 'SUSHI'],
 'EMOTION': ['HAPPINESS', 'ANGER'],
 'PERSONAL EXPERIENCE': ['TRAVEL ABROAD', 'STUDYING ABROAD'],
 'INTERACTION': ['TEAM MEETING', 'NETWORKING EVENT'],
 'BEVERAGE': ['COFFEE', 'TEA'],
 'PLAN': ['ANNUAL BUDGET', 'PROJECT TIMELINE'],
 'GEO': ['NEW YORK CITY', 'SOUTH AFRICA'],
 'GEAR': ['CAMPING TENT', 'CYCLING HELMET'],
 'EMOJI': ['🎉', '🚀'],
 'BEHAVIOR': ['POSITIVE FEEDBACK', 'NEGATIVE CRITICISM'],
 'TONE': ['FORMAL', 'INFORMAL'],
 'LOCATION': ['DOWNTOWN', 'SUBURBS']
}}
################
输出:
{{
  "answer_type_keywords": ["STRATEGY","PERSONAL LIFE"],
  "entities_from_query": ["贸易协定", "关税", "货币兑换", "进口", "出口"]
}}
#############################
示例2:

查询: "SpaceX的第一次火箭发射是什么时候？"
答案类型池: {{
 'DATE AND TIME': ['2023-10-10 10:00', 'THIS AFTERNOON'],
 'ORGANIZATION': ['GLOBAL INITIATIVES CORPORATION', 'LOCAL COMMUNITY CENTER'],
 'PERSONAL LIFE': ['DAILY EXERCISE ROUTINE', 'FAMILY VACATION PLANNING'],
 'STRATEGY': ['NEW PRODUCT LAUNCH', 'YEAR-END SALES BOOST'],
 'SERVICE FACILITATION': ['REMOTE IT SUPPORT', 'ON-SITE TRAINING SESSIONS'],
 'PERSON': ['ALEXANDER HAMILTON', 'MARIA CURIE'],
 'FOOD': ['GRILLED SALMON', 'VEGETARIAN BURRITO'],
 'EMOTION': ['EXCITEMENT', 'DISAPPOINTMENT'],
 'PERSONAL EXPERIENCE': ['BIRTHDAY CELEBRATION', 'FIRST MARATHON'],
 'INTERACTION': ['OFFICE WATER COOLER CHAT', 'ONLINE FORUM DEBATE'],
 'BEVERAGE': ['ICED COFFEE', 'GREEN SMOOTHIE'],
 'PLAN': ['WEEKLY MEETING SCHEDULE', 'MONTHLY BUDGET OVERVIEW'],
 'GEO': ['MOUNT EVEREST BASE CAMP', 'THE GREAT BARRIER REEF'],
 'GEAR': ['PROFESSIONAL CAMERA EQUIPMENT', 'OUTDOOR HIKING GEAR'],
 'EMOJI': ['📅', '⏰'],
 'BEHAVIOR': ['PUNCTUALITY', 'HONESTY'],
 'TONE': ['CONFIDENTIAL', 'SATIRICAL'],
 'LOCATION': ['CENTRAL PARK', 'DOWNTOWN LIBRARY']
}}

################
输出:
{{
  "answer_type_keywords": ["DATE AND TIME", "ORGANIZATION", "PLAN"],
  "entities_from_query": ["SpaceX", "火箭发射", "航空航天", "动力回收"]

}}
#############################
示例3:

查询: "教育在减少贫困方面的作用是什么？"
答案类型池: {{
 'PERSONAL LIFE': ['MANAGING WORK-LIFE BALANCE', 'HOME IMPROVEMENT PROJECTS'],
 'STRATEGY': ['MARKETING STRATEGIES FOR Q4', 'EXPANDING INTO NEW MARKETS'],
 'SERVICE FACILITATION': ['CUSTOMER SATISFACTION SURVEYS', 'STAFF RETENTION PROGRAMS'],
 'PERSON': ['ALBERT EINSTEIN', 'MARIA CALLAS'],
 'FOOD': ['PAN-FRIED STEAK', 'POACHED EGGS'],
 'EMOTION': ['OVERWHELM', 'CONTENTMENT'],
 'PERSONAL EXPERIENCE': ['LIVING ABROAD', 'STARTING A NEW JOB'],
 'INTERACTION': ['SOCIAL MEDIA ENGAGEMENT', 'PUBLIC SPEAKING'],
 'BEVERAGE': ['CAPPUCCINO', 'MATCHA LATTE'],
 'PLAN': ['ANNUAL FITNESS GOALS', 'QUARTERLY BUSINESS REVIEW'],
 'GEO': ['THE AMAZON RAINFOREST', 'THE GRAND CANYON'],
 'GEAR': ['SURFING ESSENTIALS', 'CYCLING ACCESSORIES'],
 'EMOJI': ['💻', '📱'],
 'BEHAVIOR': ['TEAMWORK', 'LEADERSHIP'],
 'TONE': ['FORMAL MEETING', 'CASUAL CONVERSATION'],
 'LOCATION': ['URBAN CITY CENTER', 'RURAL COUNTRYSIDE']
}}

################
输出:
{{
  "answer_type_keywords": ["STRATEGY", "PERSON"],
  "entities_from_query": ["学校获取", "识字率", "职业培训", "收入不平等"]
}}
#############################
示例4:

查询: "美国首都在哪里？"
答案类型池: {{
 'ORGANIZATION': ['GREENPEACE', 'RED CROSS'],
 'PERSONAL LIFE': ['DAILY WORKOUT', 'HOME COOKING'],
 'STRATEGY': ['FINANCIAL INVESTMENT', 'BUSINESS EXPANSION'],
 'SERVICE FACILITATION': ['ONLINE SUPPORT', 'CUSTOMER SERVICE TRAINING'],
 'PERSON': ['ALBERTA SMITH', 'BENJAMIN JONES'],
 'FOOD': ['PASTA CARBONARA', 'SUSHI PLATTER'],
 'EMOTION': ['HAPPINESS', 'SADNESS'],
 'PERSONAL EXPERIENCE': ['TRAVEL ADVENTURE', 'BOOK CLUB'],
 'INTERACTION': ['TEAM BUILDING', 'NETWORKING MEETUP'],
 'BEVERAGE': ['LATTE', 'GREEN TEA'],
 'PLAN': ['WEIGHT LOSS', 'CAREER DEVELOPMENT'],
 'GEO': ['PARIS', 'NEW YORK'],
 'GEAR': ['CAMERA', 'HEADPHONES'],
 'EMOJI': ['🏢', '🌍'],
 'BEHAVIOR': ['POSITIVE THINKING', 'STRESS MANAGEMENT'],
 'TONE': ['FRIENDLY', 'PROFESSIONAL'],
 'LOCATION': ['DOWNTOWN', 'SUBURBS']
}}
################
输出:
{{
  "answer_type_keywords": ["LOCATION"],
  "entities_from_query": ["美国首都", "华盛顿", "纽约"]
}}
#############################

-真实数据-
######################
查询: {query}
答案类型池:{TYPE_POOL}
######################
输出:

"""

PROMPTS["keywords_extraction"] = """---角色---

你是一个有用的助手，负责识别用户查询中的高级和低级关键词。

---目标---

给定查询，列出高级和低级关键词。高级关键词专注于总体概念或主题，而低级关键词专注于特定实体、细节或具体术语。

---指令---

- 以JSON格式输出关键词。
- JSON应该有两个键:
  - "high_level_keywords" 用于总体概念或主题。
  - "low_level_keywords" 用于特定实体或细节。

######################
-示例-
######################
{examples}

#############################
-真实数据-
######################
查询: {query}
######################
`输出`应该是人类文本，而不是Unicode字符。保持与`查询`相同的语言。
输出:

"""

PROMPTS["keywords_extraction_examples"] = [
    """示例1:

查询: "国际贸易如何影响全球经济稳定性？"
################
输出:
{
  "high_level_keywords": ["国际贸易", "全球经济稳定性", "经济影响"],
  "low_level_keywords": ["贸易协定", "关税", "货币兑换", "进口", "出口"]
}
#############################""",
    """示例2:

查询: "森林砍伐对生物多样性的环境后果是什么？"
################
输出:
{
  "high_level_keywords": ["环境后果", "森林砍伐", "生物多样性丧失"],
  "low_level_keywords": ["物种灭绝", "栖息地破坏", "碳排放", "雨林", "生态系统"]
}
#############################""",
    """示例3:

查询: "教育在减少贫困方面的作用是什么？"
################
输出:
{
  "high_level_keywords": ["教育", "减少贫困", "社会经济发展"],
  "low_level_keywords": ["学校获取", "识字率", "职业培训", "收入不平等"]
}
#############################""",
]