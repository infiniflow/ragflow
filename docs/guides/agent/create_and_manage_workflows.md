---
sidebar_position: 3
title: Create and Manage Workflows
sidebar_label: Create and Manage Workflows
slug: /creation_and_management
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Create and Manage Workflows

## Open the Workflow List

Click **Agent** in the top navigation bar, then select **Workflow**. Each card represents a workflow. Click a card to open its canvas and continue editing.

Select the category before creating an application. This chapter covers Workflow. For document processing, see [Create an Ingestion Pipeline](./ingestion_pipeline/create_ingestion_pipeline.md). For compilation templates, see [Knowledge Compilation](../knowledge_compilation/overview.md).

## Create from Blank Workflow

1. Open **Agent > Workflow**.
2. Open the **+** creation menu in the upper-right corner and choose **Create from blank**. On an empty list, you can use the **Create from blank** entry directly.

   ![Workflow category and Create from blank entry](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_an_agent_from_blank_1.jpg)

3. In **Create workflow**, enter **Name** and click **Confirm**.
4. Open the workflow card. A blank workflow starts with **Begin**.
5. Click **Begin** to configure its mode and inputs. Use its outgoing connection point to add the next component, then configure the component in the panel on the right.
6. Connect the remaining steps and click **Save draft**. Use **Run** to test the workflow before publishing it.

Each workflow has one Begin component. Begin is the entry point and cannot be deleted.

## Create from Template

Templates provide components and connections for common tasks. Select one that is close to your task, then replace its models, datasets, prompts, and tool settings with resources available to your account.

1. Open **Agent > Workflow** and choose **Create from template** from the creation menu.

   ![Workflow Create from template entry](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_from_a_template_2.jpg)

2. Browse the template categories and select a suitable template.
3. Use the selected template and complete the name and confirmation prompts.
4. Open the created workflow and check each component. In particular, reselect models, datasets, and tools that the template requires.
5. Click **Save draft**, then **Run** and test with representative inputs.

## Search, Copy and Delete Workflow

- **Search**: Enter the workflow name in the list's search box.
- **Copy**: Open the workflow, select **Management > Export**, then [import the JSON](./import_and_export_workflows.md#import-workflow) with a different name.
- **Delete**: Open the card's **…** menu, select **Delete**, and complete the confirmation. Export a copy first if you need to keep the configuration.

## Save, Test and Publish

The canvas displays the latest **Autosaved** time. **Save draft** saves your current editing configuration; it does not create a published version. Use **Run** to check the workflow, then **Publish** when the configuration is ready for use.

See [Canvas Toolbar](./understand_workflow_canvas.md#canvas-toolbar) for the purpose of each upper-right button, publishing steps, and the entries in **Management**.
