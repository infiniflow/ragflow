---
sidebar_position: 3
title: Creation and Management
sidebar_label: Creation and Management
slug: /creation_and_management
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Creation and Management

## Access Agent Page
After logging into RAGFlow, click **Agent** in the top navigation bar to enter the Agent page. Created Agents are displayed as cards. Click an existing card to continue editing; click the creation entry to create a new Agent.

![Agent list](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/agent_list.jpg)

## Create from Template
RAGFlow provides Agent templates for different business scenarios. When creating from a template, the system presets common components and connections. Users only need to modify models, knowledge bases, prompts, interface addresses or output content.

Steps:
1. Enter the Agent page.
2. Open **Create agent > Create from template**.

![Create from a template entry](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_from_a_template_2.jpg)
3. Select an appropriate template on the template page, such as Deep Research, Knowledge Base Q&A, Data Analysis or E-commerce Customer Service template.

![Select an Agent template](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_from_a_template_3.jpg)
4. Enter the Agent name.
5. Confirm the creation.
6. After entering the canvas, check the configuration of each component and save.

![Template Agent canvas](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_from_a_template_4.jpg)
## Create from Blank Agent
A blank Workflow starts with **Begin**. Use a component's outgoing connection point to add downstream components. An Ingestion pipeline starts with **Begin → Parser**; see [Create an Ingestion Pipeline](./ingestion_pipeline/create_ingestion_pipeline.md) for its setup.

Steps:
1. Enter the Agent page.
2. Open **Create agent**.
3. Select **Create from blank**.

![Create from blank entry](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_an_agent_from_blank_1.jpg)
4. Select **Workflow**, **Ingestion pipeline**, or **Compilation operator**, enter **Name**, and click **Confirm**. Choose Workflow for a conversational Agent.
5. Open the created card to enter its canvas. For **Compilation operator**, configure the template group and model as described in [Configure the Compiler Component](./ingestion_pipeline/understand_core_ingestion_pipeline_components.md#configure-the-compiler-component).
6. For a Workflow, click the outgoing connection point on **Begin** and add the next component.

![Add components to a blank Agent](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_an_agent_from_blank_3.jpg)
7. Configure each component.
8. Click **Save**.

:::tip NOTE
The `Begin` component is the start of the workflow. Each Agent can only have one Begin component and it cannot be deleted. After creation, configure Begin first, then configure subsequent components.
:::
## Search, Copy and Delete Agent
- **Search**: Enter the Agent name in the list's search box.
- **Copy**: Open the Agent, choose **Management > Export**, then [import the JSON](./import_and_export_agents.md#import-agent) with a different name.
- **Delete**: Open the card's **…** menu, select **Delete**, and confirm the Agent name. Deletion removes the Agent and its saved versions. Export a copy first if you need to keep its configuration.

## Save and Publish

The editor shows the latest autosave time.

- **Save**: Save the editing configuration.
- **Run**: Test the workflow in the editor.
- **Launch**: Open the conversation page in Conversational mode.
- **Publish**: Create a published version after checking the test results.

## Management

The Workflow canvas provides these entries under **Management**:

| Entry | Purpose |
| --- | --- |
| Conversation variable | Configure variables used in a conversation. |
| Version history | Browse recorded versions and inspect their canvas configurations. A history entry alone does not indicate a published version. |
| Log | Search execution records by **ID/Title** and date. View state, version, and other record details, or use **Export current page**. |
| Export | Export the Agent configuration. |
| Settings | Open Agent settings. |
| Embed into webpage | Open the website embedding options. |

Available controls differ for other Agent types. Ingestion pipeline creation and validation are covered in the [pipeline guide](./ingestion_pipeline/create_ingestion_pipeline.md).
