---
sidebar_position: 1
slug: /begin_component
sidebar_custom_props: {
  categoryIcon: LucideHome
}
---


# Agent
## Chapter Overview
This chapter introduces the creation, configuration, debugging and publishing methods of RAGFlow Agent. You can connect models, knowledge bases, tools and workflow control components through a visual canvas to build intelligent agent applications such as knowledge Q&A, business query, content processing and automated execution.

> [!TIP]
> > [!NOTE]
> Before using an Agent, please confirm that at least one available chat model has been configured. If the workflow needs to query knowledge bases, you also need to create knowledge bases in advance, upload files and complete parsing.

## Agent Overview
### Purpose of Agent
Agent is the business workflow orchestration capability in RAGFlow. Users can add components on a no-code canvas and define execution order via connections. Components can be executed sequentially, or enter different paths according to conditional branches, classification results or loop logic.

Agents are commonly used in the following scenarios:
- Answering user questions based on knowledge bases.
- Identifying user intents and routing to different processing flows.
- Calling HTTP interfaces, databases, MCP tools or custom code.
- Splitting and batch processing long texts.
- Saving session memory or exporting processing results.

### Relationship Between Agent and Knowledge Base Q&A
- **Chat**: Suitable for applications mainly based on knowledge base Q&A and multi-turn dialogue.
- **Agent**: Suitable for business workflows requiring conditional branching, tool calling, data processing or multi-step orchestration.

The `Retrieval` component can be used inside an Agent to query knowledge bases. Retrieval can also be used as a tool under the Agent component, allowing the LLM to autonomously decide when to perform retrieval.

For example, an after-sales Agent can first use `Categorize` to judge the type of user questions, then use `Retrieval` to query product materials. If the user requests installation reservation, an HTTP request is used to call the external work order system. Finally, the `Message` component outputs the result.

## Creation & Management
### Access Agent Page
After logging into RAGFlow, click **Agent** in the top navigation bar to enter the Agent page. Created Agents are displayed as cards. Click an existing card to continue editing; click the creation entry to create a new Agent.

### Create from Template
RAGFlow provides Agent templates for different business scenarios. When creating from a template, the system presets common components and connections. Users only need to modify models, knowledge bases, prompts, interface addresses or output content.

Steps:
1. Enter the Agent page.
2. Click **Create agent**.
3. Select an appropriate template on the template page, such as Deep Research, Knowledge Base Q&A, Data Analysis or E-commerce Customer Service template.
4. Enter the Agent name.
5. Click **OK**.
6. After entering the canvas, check the configuration of each component and save.

### Create from Blank Agent
When creating a blank Agent, the canvas contains a default `Begin` component. Users can click the plus sign next to the Begin component or other components to add downstream components.

Steps:
1. Enter the Agent page.
2. Click **Create agent**.
3. Select blank creation.
4. Enter the Agent name and select the agent type.
5. Enter the canvas.
6. Click the plus sign next to the `Begin` component, and add components according to business processes.
7. Configure each component.
8. Click **Save**.

> [!NOTE]
> The `Begin` component is the start of the workflow. Each Agent can only have one Begin component and it cannot be deleted. After creation, configure Begin first, then configure subsequent components.

### Search, Copy and Delete Agent
In the Agent list, you can search for target Agents by name. If copy, delete or other operation entries are available on the interface, confirm the Agent name and permission scope before operation. Deletion is usually irreversible. Please confirm whether the Agent is referenced by embedded web pages, API calls or other workflows.

### Save Agent
Click **Save** in time after editing the canvas. Saving only records the current configuration. Whether it immediately affects published or embedded Agents depends on the version publishing mechanism and deployment strategy. Re-run tests before external usage.

### Import & Export Agent
#### Import Agent
- First-time import: Upload the JSON file, fill in corresponding information, drag or click to upload the file, then save.
- Non-first-time import: Upload the JSON file, fill in corresponding information, drag or click to upload the file, then save.

#### Export Agent
Open an Agent, click **Manage** in the upper right corner, then click export.

## Embed into Web Pages
### Embed Agent via Webpage
You can embed the Agent into third-party web pages using iframe.

Prerequisite: You must obtain an API Key. In Enterprise Edition, only Admin accounts can obtain the API Key.

Steps:
1. On the Agent page, click the target Agent to open its editing page.
2. Click **Manage > Embed Webpage** in the upper right corner of the canvas to open the iframe window.
3. Copy the iframe code and embed it into your webpage.

## Understand the Canvas
### Add Components
Click the plus sign next to any component on the canvas to select the next component. Common components include:
`Begin`, `Agent`, `Retrieval`, `Message`, `Await response`, `Switch`, `Iteration`, `Categorize`, `Code`, `Text processing`, `Execute SQL`, `HTTP Request`, and pipeline-related components: `Parser`, `Title chunker`, `Token chunker`, `Transformer`, `Indexer`.

After adding a component, click the component itself to open the configuration panel on the right. Fields in the configuration panel define input data, processing logic, output variables, and references for subsequent steps.

### Use Variables
Components on the Agent canvas support variable references to implement data transmission between components. Variable sources include system variables, global variables defined in the Begin component, and outputs from upstream components.

In input boxes supporting variable references, type `/` or click the variable button next to the input box to open the variable selector.

Common variables:
| Variable | Description |
| ---- | ---- |
| `sys.query` | Current user input question |
| `formalized_content` | Sorted text results from Retrieval, SQL or tool components |
| `chunks` | Fragment set output by document parsing or chunking components |
| `content` | Main text output from Agent, Code or other components |

Operation Steps:
1. Enter the Agent page and open the target canvas for editing.
2. Select a component supporting variable references, e.g. Agent, Retrieval, Message, Code, HTTP Request, SQL.
3. On the right configuration panel, click the input box that needs variable reference, such as System Prompt, User Prompt, Query, etc.
4. Open the variable selector via either method:
   - Type `/` inside the input box
   - Click the variable icon next to the input box (may display as `{}`, `+` or other icons in different versions)
5. Select required variables in the selector. Available variables include:
   - System variables such as `sys.query`
   - Global variables defined in the Begin component
   - Output variables from upstream components
6. After selection, the variable will be automatically inserted into the input box.
7. Save component configuration and run the Agent to verify data transmission.

### Save & Run
After configuration, click **Save** to save the Agent. During debugging, click **Run** at the top of the canvas, enter test questions and observe the execution result of each component. If a component returns no output, check input variables, model configuration, knowledge base permissions, external interface addresses or tool configuration.

### Component Connection Rules
Connections define component execution order. Sequential components run along a single path. Branch components such as `Switch` and `Categorize` route workflows to different exits according to conditions. `Iteration` executes sub-processes in loops. Components not connected to the execution path will not run. Before deleting a component, check upstream/downstream connections and variable references to avoid missing inputs for subsequent nodes.

### Configuration Panel
Click any canvas component to open the right-side configuration panel, which displays fields, input variables, output variables and runtime parameters. Recommended workflow: confirm which variable the component reads → configure processing logic → verify downstream nodes reference correct outputs.

## Agent Workflow
### Basic Component Configuration
#### Begin Component
`Begin` is the start of the Agent workflow, used to set trigger mode, opening greeting and global input variables. Every Agent must contain exactly one Begin component.

##### Trigger Modes
- **Conversational**: Triggered via dialogue, suitable for regular chat Agents.
- **Task**: Started as a task, suitable for non-dialog automated workflows.
- **Webhook**: Triggered by external HTTP requests, suitable for system integration, automation tasks and third-party callbacks.
When Webhook mode is selected, the system generates the Agent Webhook URL. You can further configure request method, security authentication, request Schema and response mode.

##### Opening Greeting
In Conversational mode, set the first message sent by the Agent in Opening greeting. The greeting should clearly describe supported capabilities instead of lengthy introductions.

Example:
> Hello, I can help you query product materials, compare models and generate installation suggestions.
> Please describe your question or upload files for analysis.

##### Input Variables
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

> [!NOTE]
> Files uploaded through the Begin component are only used as workflow input. They will not be automatically saved to knowledge bases, nor use knowledge base parsing, OCR or chunking capabilities. File content can be passed to subsequent components as variables and limited by model context length.

#### Agent Component
The Agent component invokes LLMs for reasoning, content generation, task planning and tool calling. It can process user questions independently or cooperate with retrieval, HTTP requests, code, databases and sub-agents to complete multi-step tasks.

Capabilities:
- Reason, reflect and adjust logic based on context and execution results
- Call tools or sub-agents to complete tasks
- Control reply style, task boundaries and output format via system & user prompts

##### Basic Configuration Steps
1. Click the Agent component to open the right configuration panel.
2. Select a chat model in Model.
3. Adjust Creativity or keep Precise by default.
4. Define role, constraints and output format in System prompt.
5. Write task instructions in User prompt and insert variables by typing `/`.
6. Add Tools or sub-Agents if retrieval, SQL, HTTP, MCP or nested agents are required.
7. Set output variable name.
8. Save and run tests.

##### Prompt Configuration
- **System Prompt**: Define model role and behavior boundaries.
- **User Prompt**: Define current task and input data.

If the Agent component follows a Retrieval component, usually reference `formalized_content` in User Prompt to make the model answer based on retrieved documents.

Example:
> Please answer `/Retrieval_0.formalized_content` according to `/sys.query`. If retrieval results are insufficient, clearly state that confirmation cannot be obtained from the knowledge base and do not fabricate answers.

##### Tools & Sub-Agents
When tools or sub-Agents are attached under an Agent component, the current Agent acts as a planner to judge when to invoke these capabilities. Tools include Retrieval, Execute SQL, HTTP Request, MCP Server and other available components. Sub-Agents split complex tasks for multi-role collaboration.

It is recommended to specify trigger conditions in system prompts:
> Retrieve knowledge base when questions involve product documents; call HTTP interface when querying order status.

> [!NOTE]
> Tool calling, sub-agents, reflection rounds and larger message window size will increase response latency. Prioritize simple pipelines for regular Q&A; enable planning logic only when necessary.

##### Advanced Settings
Advanced settings control context management, exception handling and output format during runtime. Keep default values in most cases; adjust when optimizing effect or adapting to business requirements.

| Parameter | Description | Suggestion |
| ---- | ---- | ---- |
| Message window size | Number of historical messages retained during LLM reasoning. Larger windows provide more context but consume more tokens. | Keep default; increase for multi-turn dialogue, decrease for single-turn tasks |
| Citation | Whether to return source citations in answers when using knowledge bases. | Enable for knowledge Q&A scenarios |
| Max retries | Maximum retry attempts after Agent execution failure. | Keep default; increase under unstable network |
| Delay after error | Waiting time (seconds) before each retry to avoid continuous failed requests. | Keep default |
| Exception handling method | Execution policy when exceptions occur. | Adjust based on business requirements |

##### Output Configuration
Two output types are supported:
1. **content**: Default natural language text returned by Agent
2. **structured**: Structured data output. After enabling Structured output, define the format with JSON Schema to standardize returned fields for Code, HTTP Request, SQL and conditional judgment nodes.

#### Retrieval Component
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

#### Message Component
The Message component outputs static or dynamic content to users, usually as the final component of a workflow. It supports fixed text and variable insertion. Multiple message entries can be configured; the system randomly selects one to send.

When Begin uses Webhook mode with Final response as response method, the Message component can set HTTP status codes (200 ~ 399).

**Save to Memory**: Enable this option to store dialogue sessions into specified memory. Bind User ID to associate conversations with users; subsequent retrieval can query memory filtered by user ID.

Applicable scenarios: Output final answers, branch hints, fallback replies or display intermediate processing results. Output content will be sent to dialogue windows, webhook responses or embedded pages.

### Flow Control Components
#### Await Response Component
Await Response pauses the workflow and waits for users to supplement information. Suitable for multi-turn dialogue, form collection, confirmation operations or file upload requirements.

Configuration:
Define prompt messages to guide users. Input supports the same variable types as Begin: single-line text, paragraph text, dropdown options, file upload, number and boolean.

Recommendations:
- Dropdown options: Select business categories
- Paragraph text: Collect detailed requirement descriptions
- File upload: Receive contracts, reports or screenshots
- Boolean: Confirm continue/cancel operations

#### Switch (Conditional) Component
Switch executes rule-based judgment and routes workflows to different downstream paths according to results.

Configuration:
At least one Case must be defined. Each Case can contain multiple conditions combined by AND / OR.
Supported operators: Equals, Not equal, Greater than, Greater equal, Less than, Less equal, Contains, Not contains, Starts with, Ends with, Is empty, Not empty.

> [!NOTE]
> Switch is rule-based judgment for structured data and clear conditions. Categorize uses LLM-based classification for natural language intent recognition.

#### Iteration Component
Iteration splits text into fragments and executes the same set of internal components for each fragment. Suitable for long-text translation, paragraph-wise summarization, batch generation and item-by-item list processing.

Internal Workflow:
Iteration contains built-in `Loop Item`. Components dragged inside Iteration can only be accessed within the loop. Reference `Loop Item` to obtain current fragment data.

Configuration Parameters:
- **Loop variables**: Variables used during iteration; support read and update inside the loop
- **Loop termination condition**: Exit condition to stop iteration
- **Maximum loop count**: Prevent infinite iteration

> [!NOTE]
> Configure both termination condition and maximum loop count to avoid long-running infinite loops.

#### Categorize Component
Categorize uses LLM to judge user intent or input category and branch the workflow based on classification results.

Configuration Steps:
1. Select content to classify in Query variable / Input.
2. Select model and Creativity.
3. Configure message window size (keep default for single-turn classification).
4. Add at least two Categories.
5. Fill clear Name, Description and Examples for each category.
6. Connect downstream components for each classification result on the canvas.

Classification Recommendations:
Use easy-to-understand category names, e.g. Product Consultation, Installation Reservation, After-sales Fault, Other Questions.
Examples improve classification stability; provide 2~3 typical samples for each category.

### Data Manipulation Components
#### Code Component
The Code component executes Python or JavaScript code for complex data processing, format conversion, calculation, file generation and custom logic.

Prerequisite: The Code component depends on a secure sandbox environment. The deployment environment needs to install and enable gVisor, RAGFlow sandbox and related environment variables. Restart the service after dependency changes.

Configuration:
1. **Input**: Define parameters passed into code; variables can be directly referenced inside scripts.
2. **Code**: Select Python or JavaScript and write business logic.
3. **Return Value**: Define output data returned to downstream components.

#### Text Processing Component
Text Processing splits or merges text. Used to split long upstream text by separators or combine multiple variables into one template.

Processing Modes:
- **Merge**: Concatenate content sequentially
- **Split**: Split text by specified delimiters (comma, line break, space etc.)

Configure script content with variables inserted via `/`. Output results can be referenced by subsequent nodes.

#### Data Operation Component
Data Operation processes structured objects returned by upstream tools, code or database nodes to clean data for downstream usage.

Configuration Steps:
1. Add and select the Data Operation component on canvas.
2. Select target data variables in Query variables.
3. Click `+` to add multiple input variables. Query variables are mandatory.
4. Select processing operation in Operations and fill corresponding configurations.
5. Save and run tests.

Output: Processed data stored in variable `result`.

Supported Operations:
| Operation | Function | Scenario |
| ---- | ---- | ---- |
| Select keys | Keep only specified fields | Extract required fields for downstream nodes |
| Literal eval | Convert string-formatted list/dict/bool/number into actual data types | Parse serialized structured strings |
| Combine | Merge multiple objects into one | Aggregate outputs from multiple upstream nodes |
| Filter values | Filter data matching conditions | Filter array/object collections |
| Append or update | Add new fields or overwrite existing field values | Supplement or modify object attributes |
| Remove keys | Delete specified fields | Remove unnecessary sensitive or unused fields |
| Rename keys | Rename object field keys | Unify field naming standards |

#### Variable Assignor Component
Variable Assignor writes or updates variables during workflow execution. It can save upstream results to target variables and support overwrite, clear, append and arithmetic operations for numbers, arrays and objects.

Configuration:
Add variable rules. Select target variable, choose operation type, and fill constant or reference variable as value if required. Multiple rules execute sequentially.

Supported Operations:
| Operation | Requires Value | Description |
| ---- | ---- | ---- |
| Overwritten by | Yes | Overwrite target variable with another variable's value |
| Set | Yes | Assign fixed constant value to target variable |
| Clear | No | Empty the target variable |

#### List Operation Component
List Operation processes array data, supporting element extraction, head/tail fetching, filtering, sorting and deduplication. Suitable for array outputs from Begin, HTTP Request, Code and SQL.

Configuration:
1. Select array variable in Query variables.
2. Choose operation in Operations and fill parameters.
3. Optional: Enable Strict mode.

Strict Mode:
- Enabled: Return error when input data format is abnormal
- Disabled: Handle abnormal data with default tolerance

Supported Operations:
- Nth: Get element at specified index (starting from 0)
- Head: Get first N elements
- Tail: Get last N elements
- Filter: Filter array by conditions
- Sort: Sort array by specified field (asc/desc)
- Drop duplicates: Remove duplicate entries

#### Variable Aggregator Component
Variable Aggregator combines multiple independent variables into one output group for unified reference by downstream nodes. Widely used in multi-branch conditional workflows to collect data from different branches.

Configuration:
1. Select variables and add into variable groups (default Group0).
2. Click `+` to create new variable groups; delete groups via remove button.
3. Save configuration and run tests.

Output: Variables inside each group can be referenced via group name such as `Group0`.