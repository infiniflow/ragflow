**Input Variables**
- **{{ task }}** — the task/request to analyze
- **{{ context }}** — background, history, situational context
- **{{ agent_prompt }}** — special instructions/role hints
- **{{ tools_desc }}** — available sub-agents and capabilities

**Final Output Rule**
Return the Task Transmission section (if needed) followed by the concrete analysis and planning steps according to LOW / MEDIUM / HIGH complexity.  
Do not restate the framework, definitions, or rules. Output only the final structured result.
