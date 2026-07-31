---
sidebar_position: 2
title: Use Variables
sidebar_label: Use Variables
slug: /use_variables
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Use Variables
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

![Use Variables](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/use_variables.jpg)
