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

![Agent template list](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/agent_template_list.jpg)

Steps:
1. Enter the Agent page.
2. Click **Create agent**.

![Create from a template entry](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_from_a_template_2.jpg)
3. Select an appropriate template on the template page, such as Deep Research, Knowledge Base Q&A, Data Analysis or E-commerce Customer Service template.

![Select an Agent template](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_from_a_template_3.jpg)
4. Enter the Agent name.
5. Click **OK**.

![Create from a template settings](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_from_a_template_1.jpg)
6. After entering the canvas, check the configuration of each component and save.

![Template Agent canvas](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_from_a_template_4.jpg)
## Create from Blank Agent
When creating a blank Agent, the canvas contains a default `Begin` component. Users can click the plus sign next to the Begin component or other components to add downstream components.

Steps:
1. Enter the Agent page.
2. Click **Create agent**.
3. Select blank creation.

![Create from blank entry](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_an_agent_from_blank_1.jpg)
4. Enter the Agent name and select the agent type.

![Create blank Agent settings](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_an_agent_from_blank_2.jpg)
5. Enter the canvas.
6. Click the plus sign next to the `Begin` component, and add components according to business processes.

![Add components to a blank Agent](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_an_agent_from_blank_3.jpg)
7. Configure each component.
8. Click **Save**.

:::tip NOTE
The `Begin` component is the start of the workflow. Each Agent can only have one Begin component and it cannot be deleted. After creation, configure Begin first, then configure subsequent components.
:::
## Search, Copy and Delete Agent
In the Agent list, you can search for target Agents by name. If copy, delete or other operation entries are available on the interface, confirm the Agent name and permission scope before operation. Deletion is usually irreversible. Please confirm whether the Agent is referenced by embedded web pages, API calls or other workflows.

## Save Agent
Click **Save** in time after editing the canvas. Saving only records the current configuration. Whether it immediately affects published or embedded Agents depends on the version publishing mechanism and deployment strategy. Re-run tests before external usage.
