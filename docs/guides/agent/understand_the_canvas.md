---
sidebar_position: 6
title: Understand the Canvas
sidebar_label: Understand the Canvas
slug: /understand_the_canvas
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Understand the Canvas

## Add Components
Click the plus sign next to any component on the canvas to select the next component. Common components include:
`Begin`, `Agent`, `Retrieval`, `Message`, `Await response`, `Switch`, `Iteration`, `Categorize`, `Code`, `Text processing`, `Execute SQL`, `HTTP Request`, and pipeline-related components: `Parser`, `Title chunker`, `Token chunker`, `Transformer`, `Indexer`.

After adding a component, click the component itself to open the configuration panel on the right. Fields in the configuration panel define input data, processing logic, output variables, and references for subsequent steps.

![Add component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/add_component.jpg)

## Use Variables
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

![Use variables](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/use_variables.jpg)
## Save & Run
After configuration, click **Save** to save the Agent. During debugging, click **Run** at the top of the canvas, enter test questions and observe the execution result of each component. If a component returns no output, check input variables, model configuration, knowledge base permissions, external interface addresses or tool configuration.

![Save and run](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/save_and_run.jpg)
## Component Connection Rules
Connections define component execution order. Sequential components run along a single path. Branch components such as `Switch` and `Categorize` route workflows to different exits according to conditions. `Iteration` executes sub-processes in loops. Components not connected to the execution path will not run. Before deleting a component, check upstream/downstream connections and variable references to avoid missing inputs for subsequent nodes.

## Configuration Panel
Click any canvas component to open the right-side configuration panel, which displays fields, input variables, output variables and runtime parameters. Recommended workflow: confirm which variable the component reads → configure processing logic → verify downstream nodes reference correct outputs.
