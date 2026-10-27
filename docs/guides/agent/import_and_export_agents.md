---
sidebar_position: 4
title: Import and Export Agents
sidebar_label: Import and Export Agents
slug: /import_and_export_agents
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Import and Export Agents

## Import Agent

1. On the **Agent** page, open **Create agent > Import JSON file**. On an empty list, click **Import JSON file** directly.
2. Enter **Name** and upload one Agent JSON file under **DSL**.
3. Click **Save**, then open the imported Agent to check its configuration.

![Upload Agent JSON file](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/import_an_agent_3.jpg)

## Export Agent
Open an Agent, click **Management** in the upper right corner, then select **Export**.

![Export an Agent](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/export_an_agent.jpg)

## Check imported dependencies

After importing, reselect models, datasets, memories, MCP servers, and compilation template groups that are available to the importing account. Importing a JSON configuration does not create or transfer those resources. Check **Checklist**, save, and validate the workflow or pipeline before use.

The exporter clears API keys for selected tool types. Re-enter the required keys after import. It does not guarantee that every password, token, header, or secret in a configuration has been removed; inspect the exported JSON before sharing it.
