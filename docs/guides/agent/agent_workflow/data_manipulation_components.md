---
sidebar_position: 4
title: Data Manipulation Components
sidebar_label: Data Manipulation Components
slug: /data_manipulation_components
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Data Manipulation Components
## Code Component
The Code component executes Python or JavaScript code for complex data processing, format conversion, calculation, file generation and custom logic.

Prerequisite: An administrator must configure a provider in **Admin > Sandbox Settings**. See [Code Execution Sandbox](../../../administrator/sandbox/overview.md). For a private deployment, follow [Quickstart with Self-Managed](../../../administrator/sandbox/quickstart_with_self_managed.md).

Configuration:
1. **Input**: Define parameters passed into code; variables can be directly referenced inside scripts.
2. **Code**: Select Python or JavaScript and write business logic.
3. **Return Value**: Define output data returned to downstream components.

### Minimal example

Add two **Input** parameters, `first` and `second`, and bind them to text variables. Define one **Return Value** named `result` with type string. Use either example:

```python
def main(first: str, second: str):
    return first + second
```

```javascript
async function main(args) {
  return args.first + args.second;
}
```

In **Test run**, enter `Hello, ` for `first` and `world!` for `second`. The expected business output is `result: "Hello, world!"`. In a downstream **Message**, use the variable selector to insert this Code component's `result`.

The returned value is assigned to one business output. To return several fields, return an object and configure that business output as an object rather than defining several business outputs.

### Read the output

| Output | Meaning | Use |
| --- | --- | --- |
| Business output, such as `result` | The value returned by `main`, validated against the configured type. | Calculations and downstream data processing. |
| `content` | Text representation of the returned value. | Prompts and text display. |
| `actual_type` | The detected return type. | Diagnose a type mismatch. |
| `attachments` | Files collected from execution, when present. | File output and download handling. |

### If the test fails

- Check that Input names match the Python function arguments or JavaScript `args` properties.
- Check that the returned value matches the Return Value type. Printing a value does not replace returning it.
- For missing packages, unavailable runners, or connection errors, ask an administrator to check the active provider. See the [Sandbox FAQ](../../../administrator/sandbox/faq.md).

![Code Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/code_component.jpg)


## Text Processing Component
Text Processing splits or merges text. Used to split long upstream text by separators or combine multiple variables into one template.

Processing Modes:
- **Merge**: Concatenate content sequentially
- **Split**: Split text by specified delimiters (comma, line break, space etc.)

Configure script content with variables inserted via `/`. Output results can be referenced by subsequent nodes.

| Mode | Configure | Output |
| --- | --- | --- |
| Split | Select the input variable and delimiters. | `result`: an array of text segments. |
| Merge | Enter **Script** and insert variables with `/`. | `result`: the completed text. |

For example, splitting `red,blue` with a comma produces `["red", "blue"]`. A downstream **Iteration** can process that array. In Merge mode, a Script containing `Hello, ` followed by an inserted name variable produces a single greeting.

![Text Processing Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/text_processing_component.jpg)


## Data Operation Component
Data Operation processes structured objects returned by upstream tools, code or database nodes to clean data for downstream usage.

Configuration Steps:
1. Add and select the Data Operation component on canvas.
2. Select target data variables in Query variables.
3. Click `+` to add multiple input variables. Query variables are mandatory.
4. Select processing operation in Operations and fill corresponding configurations.
5. Save and run tests.

![Data Operation Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/data_operation_component.jpg)

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




## Variable Assigner Component
Variable Assigner updates a target variable during workflow execution. Available operations depend on the selected variable type.

Configuration Steps:
1. Click the **Variable Assigner** component and add a new variable rule under **Variables**.
2. Select the target variable to be updated.
3. Select an operation from the list available for that variable type.
4. If the selected operation requires a value, select a variable from the right panel or enter a fixed value.
5. To update multiple variables at once, continue adding variable rules. The system will execute them sequentially in the order they are added.

![Variable Assigner Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/variable_assigner_component.jpg)

Parameter Description：

| Parameter | Type | Required | Description |
|---|---|---|---|
| Target Variable | Variable | Yes | Select a target from the variable selector, such as a Begin input or Conversation variable. The selected type determines the available operations. |
| Operation | Enum | Yes | Select the operation to be performed on the target variable. The system will automatically display available operations based on the variable type. |
| Value | Variable / Constant | Conditional | The value used for the operation. You can select another variable or enter a fixed value (depending on the supported operation type). Some operations do not require a value. |

Supported operations by target type:

| Target type | Available operations |
| --- | --- |
| Text | **Overwritten by**, **Clear**, **Set** |
| Number | **Overwritten by**, **Clear**, **Set**, **Add**, **Subtract**, **Multiply**, **Divide** |
| Array | **Overwritten by**, **Clear**, **Append**, **Extend**, **Remove first**, **Remove last** |

**Overwritten by** uses another variable; **Set** assigns a constant. **Append** adds one element, while **Extend** adds elements from another array. **Clear**, **Remove first**, and **Remove last** do not require a value. Fill the value or variable selector shown for other operations. Arrays do not provide **Set** in the current UI.



## List Operation Component
List Operation processes array data, supporting element extraction, head/tail fetching, filtering, sorting and deduplication. Suitable for array outputs from Begin, HTTP Request, Code and SQL.

Configuration Steps：
1. Add the **List** component to the canvas and select it.
2. In the **Query variables** section of the right configuration panel, select the array variable to be processed. Click the dropdown menu and choose an array-type variable from the **Begin** node, upstream nodes, or session variables.
3. Select the list processing method in **Operations**.
4. configure the corresponding parameters based on the selected operation.
5. Enable **Strict mode** if required.
6. Save the configuration, then click **Run** on the page to test and view the execution results.
7. The processed result will be output through the component and can be referenced by subsequent nodes.

Parameter Description：
| Parameter | Required | Description |
|---|---|---|
| Query variables | Yes | Select the array variable to be processed. |
| Operations | Yes | Select the list processing operation. |
| Strict mode | No | When enabled, the system performs strict validation during processing and returns an error if an exception occurs. When disabled, the system processes according to the configured rules. |
| Operation Parameters | Conditional | Configure the corresponding parameters based on the selected operation, such as N, filter conditions, or sorting rules. |

Supported Operations：
| Operation | Description | Use Case |
|---|---|---|
| Nth | Retrieves an element at a specified position from the list. | Get the Nth item from the list. |
| Head | Retrieves one or more elements from the beginning of the list. | Get the first N items from the list. |
| Tail | Retrieves one or more elements from the end of the list. | Get the last N items from the list. |
| Filter | Filters elements in the list based on conditions. | Keep data that meets the specified conditions. |
| Sort | Sorts the elements in the list. | Sort by specified fields or order. |
| Drop duplicates | Removes duplicate elements from the list. | Deduplicate the list data. |

Operation Configuration Description：
| Operation | Description | Configuration |
|---|---|---|
| Nth | Retrieves an element at a specified position from the list. | Set **N** to a positive position starting from `1`: `N=1` selects the first item. The result is an array containing that item. |
| Head | Retrieves elements from the beginning of the list. | Configure the return count **N** to return the first N elements. |
| Tail | Retrieves elements from the end of the list. | Configure the return count **N** to return the last N elements. |
| Filter | Filters list elements based on conditions. | Configure filter conditions and retain only elements that meet the requirements. |
| Sort | Sorts elements in the list. | Configure the sorting field and order (ascending or descending). |
| Drop duplicates | Removes duplicate elements from the list. | No additional configuration required. Returns the deduplicated list. |

For Nth, `N=0` or a positive position beyond the list length returns an empty array when Strict mode is disabled and an error when enabled.

Strict Mode:
- Enabled: Return error when input data format is abnormal
- Disabled: Handle abnormal data with default tolerance


## Variable Aggregator Component
Variable Aggregator selects a value from candidate variables in each group, providing a common output for downstream nodes after conditional branches. It does not combine all candidate values into an array or merge them into an object.

Configuration Steps：
1. Add the **Variable Aggregator** component to the canvas and select it.
2. In the **Variable Group** (default: **Group0**), click **Select value** and choose the variables to be aggregated.
3. Click **Add** to add additional variables to aggregate.
4. To create a new variable group, click the **+** icon. To delete a variable group, click the delete button.
5. Save the configuration, then click **Run** at the top of the page to test the execution result.
6. The selected value is output under the corresponding group name (for example, **Group0**) and can be referenced by subsequent nodes.

> **Note**
>
> Each group checks its candidate variables in configuration order and returns the first resolved value that evaluates to true. Missing values, `false`, numeric `0`, empty strings, and empty collections are skipped. If no candidate qualifies, the group has no output key. Create separate groups when separate outputs are needed; do not use this selection rule when zero or false must be preserved as the result.

Parameter Description：
| Parameter | Required | Description |
|---|---|---|
| Group | Yes | Variable group used to store variables to be aggregated. A default group **Group0** is created automatically, and additional variable groups can be added. |
| Select value | Yes | Select the variables to be aggregated. Supports system variables, **Begin** node inputs, session variables, or output variables from upstream nodes. |
| Add | No | Continue adding variables to the current variable group. |
| + | No | Create a new variable group. |
| Delete | No | Delete the current variable group. |

Output Result：
| Output | Description |
|---|---|
| Group0 (or other variable group name) | Outputs the first qualifying value in the group, not all configured variables. Subsequent nodes can reference this value. |

Usage Instructions：
| Operation | Description |
|---|---|
| Add Variable | Select variables in **Select value**, then click **Add** to continue adding multiple variables. |
| Create Variable Group | Click the **+** icon to create a new variable group. |
| Delete Variable Group | Click the delete button for the variable group to remove the current group. |
| Reference Output | Subsequent nodes can directly reference the variable group (for example, **Group0**) as an input. |

![Variable Aggregation Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/variable_aggregation_component.jpg)
