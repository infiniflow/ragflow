CITATION_PROMPT_TEMPLATE = """
## Citation Requirements

- Use a uniform citation format such as [ID:i] [ID:j], where "i" and "j" are document IDs enclosed in square brackets. Separate multiple IDs with spaces (e.g., [ID:0] [ID:1]).
- Citation markers must be placed at the end of a sentence, separated by a space from the final punctuation (e.g., period, question mark).
- A maximum of 4 citations are allowed per sentence.
- DO NOT insert citations if the content is not from retrieved chunks.
- DO NOT use standalone Document IDs (e.g., #ID#).
- Citations MUST always follow the [ID:i] format.
- STRICTLY prohibit the use of strikethrough symbols (e.g., ~~) or any other non-standard formatting syntax.
- Any violation of the above rules — including incorrect formatting, prohibited styles, or unsupported citations — will result in no citation being added for that sentence.

---

## Example START

<SYSTEM>: Here is the knowledge base:

Document: Elon Musk Breaks Silence on Crypto, Warns Against Dogecoin ...
URL: https://blockworks.co/news/elon-musk-crypto-dogecoin
ID: 0
The Tesla co-founder advised against going all-in on dogecoin, but Elon Musk said it’s still his favorite crypto...

Document: Elon Musk's Dogecoin tweet sparks social media frenzy
ID: 1
Musk said he is 'willing to serve' D.O.G.E. – shorthand for Dogecoin.

Document: Causal effect of Elon Musk tweets on Dogecoin price
ID: 2
If you think of Dogecoin — the cryptocurrency based on a meme — you can’t help but also think of Elon Musk...

Document: Elon Musk's Tweet Ignites Dogecoin's Future In Public Services
ID: 3
The market is heating up after Elon Musk's announcement about Dogecoin. Is this a new era for crypto?...

The above is the knowledge base.

<USER>: What's Elon's view on dogecoin?

<ASSISTANT>:
Musk has consistently expressed his fondness for Dogecoin, often citing its humor and the inclusion of dogs in its branding. He has referred to it as his favorite cryptocurrency [ID:0] [ID:1].

Recently, Musk has hinted at potential future roles for Dogecoin. His tweets have sparked speculation about Dogecoin's potential integration into public services [ID:3].

Overall, while Musk enjoys Dogecoin and often promotes it, he also warns against over-investing in it, reflecting both his personal amusement and caution regarding its speculative nature.

## Example END
""".strip()


KEYWORD_PROMPT_TEMPLATE = """
## Role
You are a text analyzer.

## Task
Extract the most important keywords/phrases of a given piece of text content.

## Requirements
- Summarize the text content, and give the top {{ topn }} important keywords/phrases.
- The keywords MUST be in the same language as the given piece of text content.
- The keywords are delimited by ENGLISH COMMA.
- Output keywords ONLY.

---

## Text Content
{{ content }}
""".strip()


QUESTION_PROMPT_TEMPLATE = """
## Role
You are a text analyzer.

## Task
Propose {{ topn }} questions about a given piece of text content.

## Requirements
- Understand and summarize the text content, and propose the top {{ topn }} important questions.
- The questions SHOULD NOT have overlapping meanings.
- The questions SHOULD cover the main content of the text as much as possible.
- The questions MUST be in the same language as the given piece of text content.
- One question per line.
- Output questions ONLY.

---

## Text Content
{{ content }}
""".strip()


FULL_QUESTION_PROMPT_TEMPLATE = """
## Role
A helpful assistant.

## Task & Steps
1. Generate a full user question that would follow the conversation.
2. If the user's question involves relative dates, convert them into absolute dates based on today ({{ today }}).
   - "yesterday" = {{ yesterday }}, "tomorrow" = {{ tomorrow }}

## Requirements & Restrictions
- If the user's latest question is already complete, don't do anything — just return the original question.
- DON'T generate anything except a refined question.
{% if language %}
- Text generated MUST be in {{ language }}.
{% else %}
- Text generated MUST be in the same language as the original user's question.
{% endif %}

---

## Examples

### Example 1
**Conversation:**

USER: What is the name of Donald Trump's father?
ASSISTANT: Fred Trump.
USER: And his mother?

**Output:** What's the name of Donald Trump's mother?

---

### Example 2
**Conversation:**

USER: What is the name of Donald Trump's father?
ASSISTANT: Fred Trump.
USER: And his mother?
ASSISTANT: Mary Trump.
USER: What's her full name?

**Output:** What's the full name of Donald Trump's mother Mary Trump?

---

### Example 3
**Conversation:**

USER: What's the weather today in London?
ASSISTANT: Cloudy.
USER: What's about tomorrow in Rochester?

**Output:** What's the weather in Rochester on {{ tomorrow }}?

---

## Real Data

**Conversation:**

{{ conversation }}

""".strip()


CROSS_LANGUAGES_SYS_PROMPT_TEMPLATE = """
## Role
A streamlined multilingual translator.

## Behavior Rules
1. Accept batch translation requests in the following format:
   **Input:** `[text]`
   **Target Languages:** comma-separated list

2. Maintain:
   - Original formatting (tables, lists, spacing)
   - Technical terminology accuracy
   - Cultural context appropriateness

3. Output translations in the following format:

[Translation in language1]
###
[Translation in language2]

---

## Example

**Input:**
Hello World! Let's discuss AI safety.
===
Chinese, French, Japanese

**Output:**
你好世界！让我们讨论人工智能安全问题。
###
Bonjour le monde ! Parlons de la sécurité de l'IA.
###
こんにちは世界！AIの安全性について話し合いましょう。
""".strip()

CROSS_LANGUAGES_USER_PROMPT_TEMPLATE = """
**Input:**
{{ query }}
===
{{ languages | join(', ') }}

**Output:**
""".strip()


CONTENT_TAGGING_PROMPT_TEMPLATE = """
Role: You are a text analyzer.

Task: Add tags (labels) to a given piece of text content based on the examples and the entire tag set.

Steps:
  - Review the tag/label set.
  - Review examples which all consist of both text content and assigned tags with relevance score in JSON format.
  - Summarize the text content, and tag it with the top {{ topn }} most relevant tags from the set of tags/labels and the corresponding relevance score.

Requirements:
  - The tags MUST be from the tag set.
  - The output MUST be in JSON format only, the key is tag and the value is its relevance score.
  - The relevance score must range from 1 to 10.
  - Output keywords ONLY.

# TAG SET
{{ all_tags | join(', ') }}

{% for ex in examples %}
# Examples {{ loop.index0 }}
### Text Content
{{ ex.content }}

Output:
{{ ex.tags_json }}

{% endfor %}
# Real Data
### Text Content
{{ content }}
""".strip()


VISION_LLM_DESCRIBE_PROMPT = """
## INSTRUCTION
Transcribe the content from the provided PDF page image into clean Markdown format.

- Only output the content transcribed from the image.
- Do NOT output this instruction or any other explanation.
- If the content is missing or you do not understand the input, return an empty string.

## RULES
1. Do NOT generate examples, demonstrations, or templates.
2. Do NOT output any extra text such as 'Example', 'Example Output', or similar.
3. Do NOT generate any tables, headings, or content that is not explicitly present in the image.
4. Transcribe content word-for-word. Do NOT modify, translate, or omit any content.
5. Do NOT explain Markdown or mention that you are using Markdown.
6. Do NOT wrap the output in ```markdown or ``` blocks.
7. Only apply Markdown structure to headings, paragraphs, lists, and tables, strictly based on the layout of the image. Do NOT create tables unless an actual table exists in the image.
8. Preserve the original language, information, and order exactly as shown in the image.

{% if page %}
At the end of the transcription, add the page divider: `--- Page {{ page }} ---`.
{% endif %}

> If you do not detect valid content in the image, return an empty string.
""".strip()


VISION_LLM_FIGURE_DESCRIBE_PROMPT = """
## ROLE
You are an expert visual data analyst.

## GOAL
Analyze the image and provide a comprehensive description of its content. Focus on identifying the type of visual data representation (e.g., bar chart, pie chart, line graph, table, flowchart), its structure, and any text captions or labels included in the image.

## TASKS
1. Describe the overall structure of the visual representation. Specify if it is a chart, graph, table, or diagram.
2. Identify and extract any axes, legends, titles, or labels present in the image. Provide the exact text where available.
3. Extract the data points from the visual elements (e.g., bar heights, line graph coordinates, pie chart segments, table rows and columns).
4. Analyze and explain any trends, comparisons, or patterns shown in the data.
5. Capture any annotations, captions, or footnotes, and explain their relevance to the image.
6. Only include details that are explicitly present in the image. If an element (e.g., axis, legend, or caption) does not exist or is not visible, do not mention it.

## OUTPUT FORMAT (Include only sections relevant to the image content)
- Visual Type: [Type]
- Title: [Title text, if available]
- Axes / Legends / Labels: [Details, if available]
- Data Points: [Extracted data]
- Trends / Insights: [Analysis and interpretation]
- Captions / Annotations: [Text and relevance, if available]

> Ensure high accuracy, clarity, and completeness in your analysis, and include only the information present in the image. Avoid unnecessary statements about missing elements.
""".strip()
