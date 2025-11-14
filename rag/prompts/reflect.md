**Context**:
 - To achieve the goal: {{ goal }}.
 - You have executed following tool calls:
{% for call in tool_calls %}
Tool call: `{{ call.name }}`
Results: {{ call.result }}
{% endfor %}

## Task Complexity Analysis & Reflection Scope

**First, analyze the task complexity using these dimensions:**

### Complexity Assessment Matrix
- **Scope Breadth**: Single-step (1) | Multi-step (2) | Multi-domain (3)
- **Data Dependency**: Self-contained (1) | External inputs (2) | Multiple sources (3)
- **Decision Points**: Linear (1) | Few branches (2) | Complex logic (3)
- **Risk Level**: Low (1) | Medium (2) | High (3)

**Complexity Score**: Sum all dimensions (4-12 points)

---

##  Task Transmission Assessment
**Note**: This section is not subject to word count limitations when transmission is needed, as it serves critical handoff functions.
**Evaluate if task transmission information is needed:**
- **Is this an initial step?** If yes, skip this section
- **Are there downstream agents/steps?** If no, provide minimal transmission
- **Is there critical state/context to preserve?** If yes, include full transmission

### If Task Transmission is Needed:
- **Current State Summary**: [1-2 sentences on where we are]
- **Key Data/Results**: [Critical findings that must carry forward]
- **Context Dependencies**: [Essential context for next agent/step]
- **Unresolved Items**: [Issues requiring continuation]
- **Status for User**: [Clear status update in user terms]
- **Technical State**: [System state for technical handoffs]

---

##  Situational Reflection (Adjust Length Based on Complexity Score)

### Reflection Guidelines:
- **Simple Tasks (4-5 points)**: ~50-100 words, focus on completion status and immediate next step
- **Moderate Tasks (6-8 points)**: ~100-200 words, include core details and main risks  
- **Complex Tasks (9-12 points)**: ~200-300 words, provide full analysis and alternatives

### 1. Goal Achievement Status
 - Does the current outcome align with the original purpose of this task phase? 
 - If not, what critical gaps exist?

### 2. Step Completion Check
 - Which planned steps were completed? (List verified items)
 - Which steps are pending/incomplete? (Specify exactly what's missing)

### 3. Information Adequacy
 - Is the collected data sufficient to proceed?
 - What key information is still needed? (e.g., metrics, user input, external data)

### 4. Critical Observations
 - Unexpected outcomes: [Flag anomalies/errors]
 - Risks/blockers: [Identify immediate obstacles]
 - Accuracy concerns: [Highlight unreliable results]

### 5. Next-Step Recommendations
 - Proposed immediate action: [Concrete next step]
 - Alternative strategies if blocked: [Workaround solution]
 - Tools/inputs required for next phase: [Specify resources]

---

**Output Instructions:**
1. First determine your complexity score
2. Assess if task transmission section is needed using the evaluation questions
3. Provide situational reflection with length appropriate to complexity
4. Use clear headers for easy parsing by downstream systems
