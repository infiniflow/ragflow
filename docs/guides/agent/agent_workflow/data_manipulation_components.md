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

Prerequisite: The Code component depends on a secure sandbox environment. The deployment environment needs to install and enable gVisor, RAGFlow sandbox and related environment variables. Restart the service after dependency changes.

Configuration:
1. **Input**: Define parameters passed into code; variables can be directly referenced inside scripts.
2. **Code**: Select Python or JavaScript and write business logic.
3. **Return Value**: Define output data returned to downstream components.

![Code Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/code_component.jpg)


## Text Processing Component
Text Processing splits or merges text. Used to split long upstream text by separators or combine multiple variables into one template.

Processing Modes:
- **Merge**: Concatenate content sequentially
- **Split**: Split text by specified delimiters (comma, line break, space etc.)

Configure script content with variables inserted via `/`. Output results can be referenced by subsequent nodes.

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




## Variable Assignor Component
Variable Assignor writes or updates variables during workflow execution. It can save upstream results to target variables and support overwrite, clear, append and arithmetic operations for numbers, arrays and objects.

Configuration Steps:
1. Click the **Variable Assigner** component and add a new variable rule under **Variables**.
2. Select the target variable to be updated.
3. Select an assignment operation, such as **Overwrite**, **Set**, **Append**, or **Add**.
4. If the selected operation requires a value, select a variable from the right panel or enter a fixed value.
5. To update multiple variables at once, continue adding variable rules. The system will execute them sequentially in the order they are added.

![Variable Assigner Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/variable_assigner_component.jpg)

Parameter Description：

| Parameter | Type | Required | Description |
|---|---|---|---|
| Target Variable | Variable | Yes | Select the variable to be written or updated. Supports component output variables, system variables, environment variables, session variables, and other variable types. |
| Operation | Enum | Yes | Select the operation to be performed on the target variable. The system will automatically display available operations based on the variable type. |
| Value | Variable / Constant | Conditional | The value used for the operation. You can select another variable or enter a fixed value (depending on the supported operation type). Some operations do not require a value. |

Supported Operations:
| Operation | Requires Value | Description |
| ---- | ---- | ---- |
| Overwritten by | Yes | Overwrite target variable with another variable's value |
| Set | Yes | Assign fixed constant value to target variable |
| Clear | No | Empty the target variable |



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
| Nth | Retrieves an element at a specified position from the list. | Configure **N** to specify the element index (starting from 0). |
| Head | Retrieves elements from the beginning of the list. | Configure the return count **N** to return the first N elements. |
| Tail | Retrieves elements from the end of the list. | Configure the return count **N** to return the last N elements. |
| Filter | Filters list elements based on conditions. | Configure filter conditions and retain only elements that meet the requirements. |
| Sort | Sorts elements in the list. | Configure the sorting field and order (ascending or descending). |
| Drop duplicates | Removes duplicate elements from the list. | No additional configuration required. Returns the deduplicated list. |

Strict Mode:
- Enabled: Return error when input data format is abnormal
- Disabled: Handle abnormal data with default tolerance


## Variable Aggregator Component
Variable Aggregator combines multiple independent variables into one output group for unified reference by downstream nodes. Widely used in multi-branch conditional workflows to collect data from different branches.

Configuration Steps：
1. Add the **Variable Aggregator** component to the canvas and select it.
2. In the **Variable Group** (default: **Group0**), click **Select value** and choose the variables to be aggregated.
3. Click **Add** to add additional variables to aggregate.
4. To create a new variable group, click the **+** icon. To delete a variable group, click the delete button.
5. Save the configuration, then click **Run** at the top of the page to test the execution result.
6. The aggregated variables will be output through the corresponding variable group name (for example, **Group0**) and can be referenced by subsequent nodes.

> **Note**
>
> A variable group can contain multiple variables. Multiple variable groups can also be created based on different business requirements for separate aggregation.

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
| Group0 (or other variable group name) | Outputs the variables after aggregation in the current variable group. The output can be directly referenced by subsequent nodes. |

Usage Instructions：
| Operation | Description |
|---|---|
| Add Variable | Select variables in **Select value**, then click **Add** to continue adding multiple variables. |
| Create Variable Group | Click the **+** icon to create a new variable group. |
| Delete Variable Group | Click the delete button for the variable group to remove the current group. |
| Reference Output | Subsequent nodes can directly reference the variable group (for example, **Group0**) as an input. |

![Variable Aggregation Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/variable_aggregation_component.jpg)
