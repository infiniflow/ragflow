**Context**:
 - To achieve the goal: {{ goal }}.
 - You have executed following tool calls:
{% for call in tool_calls %}
Tool call: `{{ call.name }}`
Results: {{ call.result }}
{% endfor %}


**Reflection Instructions:**

Analyze the current state of the overall task ({{ goal }}), then provide structured responses to the following:

## 1. Goal Achievement Status
 - Does the current outcome align with the original purpose of this task phase? 
 - If not, what critical gaps exist?

## 2. Step Completion Check
 - Which planned steps were completed? (List verified items)
 - Which steps are pending/incomplete? (Specify exactly what’s missing)

## 3. Information Adequacy
 - Is the collected data sufficient to proceed?
 - What key information is still needed? (e.g., metrics, user input, external data)

## 4. Critical Observations
 - Unexpected outcomes: [Flag anomalies/errors]
 - Risks/blockers: [Identify immediate obstacles]
 - Accuracy concerns: [Highlight unreliable results]

## 5. Next-Step Recommendations
 - Proposed immediate action: [Concrete next step]
 - Alternative strategies if blocked: [Workaround solution]
 - Tools/inputs required for next phase: [Specify resources]