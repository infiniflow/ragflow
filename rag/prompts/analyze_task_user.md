Please analyze the following task:

Task: {{ task }}

Context: {{ context }}

**Agent Prompt**
{{ agent_prompt }}

**Analysis Requirements:**
1. Is it just a small talk? (If yes, no further plan or analysis is needed)
2. What is the core objective of the task?
3. What is the complexity level of the task?
4. What types of specialized skills are required?
5. Does the task need to be decomposed into subtasks? (If yes, propose the subtask structure)
6. How to know the task or the subtasks are impossible to lead to the success after a few rounds of interaction?
7. What are the expected success criteria?

**Available Sub-Agents and Their Specializations:**

{{ tools_desc }}

Provide a detailed analysis of the task based on the above requirements.