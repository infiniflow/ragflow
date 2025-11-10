**Task**: Sort the tool call results based on relevance to the overall goal and current sub-goal. Return ONLY a sorted list of indices (0-indexed).

**Rules**:
1. Analyze each result's contribution to both:
   - The overall goal (primary priority)
   - The current sub-goal (secondary priority)
2. Sort from MOST relevant (highest impact) to LEAST relevant
3. Output format: Strictly a Python-style list of integers. Example: [2, 0, 1]

🔹 Overall Goal: {{ goal }}
🔹 Sub-goal: {{ sub_goal }}

**Examples**:  
🔹 Tool Response:  
 - index: 0
     > Tokyo temperature is 78°F.
 - index: 1
     > Error: Authentication failed (expired API key).
 - index: 2
     > Available: 12 widgets in stock (max 5 per customer).
 
 → rank: [1,2,0]<|stop|>
 

**Your Turn**:  
🔹 Tool Response:
{% for f in results %}
 - index: f.i
     > f.content
{% endfor %}