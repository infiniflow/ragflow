---
sidebar_position: 1
title: Basic Component Configuration
sidebar_label: Basic Component Configuration
slug: /basic_component_configuration
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Basic Component Configuration
## Begin Component
`Begin` is the start of the Agent workflow, used to set trigger mode, opening greeting and global input variables. Every Agent must contain exactly one Begin component.

### Trigger Modes
- **Conversational**: Triggered via dialogue, suitable for regular chat Agents.
- **Task**: Started as a task, suitable for non-dialog automated workflows.
- **Webhook**: Triggered by external HTTP requests, suitable for system integration, automation tasks and third-party callbacks.
When Webhook mode is selected, the system generates the Agent Webhook URL. You can further configure request method, security authentication, request Schema and response mode.

### Opening Greeting
In Conversational mode, set the first message sent by the Agent in Opening greeting. The greeting should clearly describe supported capabilities instead of lengthy introductions.

Example:
> Hello, I can help you query product materials, compare models and generate installation suggestions.
> Please describe your question or upload files for analysis.

### Input Variables
Input defines parameters users need to provide before dialogue starts. After configuration, subsequent components can reference these inputs via variables.

On the Input section of the Begin configuration panel, click the `+` button to add new input variables.

Common fields:
- **Name**: Display name of variable
- **Type**: Variable type
- **Key**: Variable key referenced by downstream components
- **Optional**: Whether this input is optional

Variable Types:
| Type | Description |
| ---- | ---- |
| Single-line text | Short text input: names, keywords, serial numbers |
| Paragraph text | Long content: requirement descriptions, prompts, problem details |
| Dropdown options | Predefined selection list, suitable for fixed options such as language, department |
| File upload | Allow users to upload files as workflow input for document analysis and image processing. Uploaded files will NOT be automatically saved to knowledge bases and are only used within the current workflow. |
| Number | Numeric input: quantity, threshold, Top K value, maximum return entries |
| Boolean | Toggle (True/False) to enable functions or confirm execution branches |

:::tip NOTE
Files uploaded through the Begin component are only used as workflow input. They will not be automatically saved to knowledge bases, nor use knowledge base parsing, OCR or chunking capabilities. File content can be passed to subsequent components as variables and limited by model context length.
:::

![Begin Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/begin_component_1.jpg)

![Begin Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/begin_component_2.jpg)

## Agent Component
The Agent component invokes LLMs for reasoning, content generation, task planning and tool calling. It can process user questions independently or cooperate with retrieval, HTTP requests, code, databases and sub-agents to complete multi-step tasks.

Capabilities:
- Reason, reflect and adjust logic based on context and execution results
- Call tools or sub-agents to complete tasks
- Control reply style, task boundaries and output format via system & user prompts

### Basic Configuration Steps
1. Click the Agent component to open the right configuration panel.
2. Select a chat model in Model.
3. Adjust Creativity or keep Precise by default.
4. Define role, constraints and output format in System prompt.
5. Write task instructions in User prompt and insert variables by typing `/`.
6. Add Tools or sub-Agents if retrieval, SQL, HTTP, MCP or nested agents are required.
7. Set output variable name.
8. Save and run tests.

### Prompt Configuration
- **System Prompt**: Define model role and behavior boundaries.
- **User Prompt**: Define current task and input data.

If the Agent component follows a Retrieval component, usually reference `formalized_content` in User Prompt to make the model answer based on retrieved documents.

Example:
> Please answer `/Retrieval_0.formalized_content` according to `/sys.query`. If retrieval results are insufficient, clearly state that confirmation cannot be obtained from the knowledge base and do not fabricate answers.

### Tools & Sub-Agents
When tools or sub-Agents are attached under an Agent component, the current Agent acts as a planner to judge when to invoke these capabilities. Tools include Retrieval, Execute SQL, HTTP Request, MCP Server and other available components. Sub-Agents split complex tasks for multi-role collaboration.

It is recommended to specify trigger conditions in system prompts:
> Retrieve knowledge base when questions involve product documents; call HTTP interface when querying order status.

:::tip NOTE
Tool calling, sub-agents, reflection rounds and larger message window size will increase response latency. Prioritize simple pipelines for regular Q&A; enable planning logic only when necessary.
:::
### Advanced Settings
Advanced settings control context management, exception handling and output format during runtime. Keep default values in most cases; adjust when optimizing effect or adapting to business requirements.

| Parameter | Description | Suggestion |
| ---- | ---- | ---- |
| Message window size | Number of historical messages retained during LLM reasoning. Larger windows provide more context but consume more tokens. | Keep default; increase for multi-turn dialogue, decrease for single-turn tasks |
| Citation | Whether to return source citations in answers when using knowledge bases. | Enable for knowledge Q&A scenarios |
| Max retries | Maximum retry attempts after Agent execution failure. | Keep default; increase under unstable network |
| Delay after error | Waiting time (seconds) before each retry to avoid continuous failed requests. | Keep default |
| Exception handling method | Execution policy when exceptions occur. | Adjust based on business requirements |

### Output Configuration
Two output types are supported:
1. **content**: Default natural language text returned by Agent
2. **structured**: Structured data output. After enabling Structured output, define the format with JSON Schema to standardize returned fields for Code, HTTP Request, SQL and conditional judgment nodes.

![Agent Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/agent_component_1.jpg)

![Agent Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/agent_component_2.jpg)

![Agent Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/agent_component_3.jpg)

![Agent Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/agent_component_4.jpg)


## Retrieval Component
The Retrieval component fetches relevant content from specified knowledge bases or memory. It can be used as an independent workflow component or as a tool inside an Agent component.

Configuration Steps:
1. Click the Retrieval component.
2. Select query source for Query variable (commonly `sys.query`).
3. Select one or more knowledge bases or Memory as retrieval source.
4. Adjust similarity threshold, keyword similarity weight and Top N.
5. Enable Cross-language search for multilingual scenarios.
6. Enable Use knowledge graph for multi-hop graph Q&A.
7. Run and test retrieval results.

Parameter Explanation:
- **Similarity threshold**: Filter low-relevance chunks. Higher values enforce stricter matching but may omit useful content.
- **Keyword similarity weight**: Control vector weight in comprehensive similarity. All knowledge bases used together must share the same embedding model.
- **Top N**: Number of chunks passed to downstream components. Too few leads to insufficient information; too many increases latency and token usage.
- **Rerank model**: Improve sorting of retrieved results, adds latency. Disable for latency-sensitive Agents.

Initial Configuration Reference:
| Parameter | Suggested Value | Scenario | Description |
| ---- | ---- | ---- | ---- |
| Top N | 3～5 | FAQ, short knowledge entries | Faster response for clear answers |
| Top N | 5～10 | Product documentation, help center | Default recommended range |
| Top N | 10～20 | Legal contracts, long document analysis | More context, higher token consumption |
| Similarity threshold | 0.2 | Loose recall | Questions with diverse expression patterns |
| Similarity threshold | 0.5 | General Q&A | Default starting value |
| Similarity threshold | 0.8 | Strict precise matching | Scenarios requiring exact terminology matching |

![Knowledge Retrieval Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/knowledge_retrieval_component_1.jpg)

![Knowledge Retrieval Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/knowledge_retrieval_component_2.jpg)


## Message Component
The Message component outputs static or dynamic content to users, usually as the final component of a workflow. It supports fixed text and variable insertion. Multiple message entries can be configured; the system randomly selects one to send.

When Begin uses Webhook mode with Final response as response method, the Message component can set HTTP status codes (200 ~ 399).

**Save to Memory**: Enable this option to store dialogue sessions into specified memory. Bind User ID to associate conversations with users; subsequent retrieval can query memory filtered by user ID.

Applicable scenarios: Output final answers, branch hints, fallback replies or display intermediate processing results. Output content will be sent to dialogue windows, webhook responses or embedded pages.

![Reply Message Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/reply_message_component_1.jpg)

![Reply Message Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/reply_message_component_2.jpg)
