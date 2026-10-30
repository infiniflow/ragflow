---
sidebar_position: 4
title: Import and Export Workflows
sidebar_label: Import and Export Workflows
slug: /import_and_export_agents
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Import and Export Workflows

Use JSON files to back up a workflow configuration or move it to another account or environment. This chapter covers **Workflow**; pipeline import and export are described in the [pipeline manual](./ingestion_pipeline/create_ingestion_pipeline.md#import-and-export-a-pipeline).

## Import Workflow

1. Open **Agent** and select **Workflow**.
2. Open the creation menu and choose **Import JSON file**. On an empty list, use the **Import JSON file** entry directly.
3. Enter **Name** and upload one workflow JSON file under **DSL**.
4. Click **Save**, then open the imported workflow card.
5. Check its components and [imported dependencies](#check-imported-dependencies), click **Save draft**, and test with **Run** before publishing.

## Export Workflow

1. Open **Agent > Workflow** and click the workflow card you want to export.
2. Check that the canvas contains the configuration you want to keep. The canvas export uses the current components and connections.
3. In the upper-right corner, open **Management > Export**.
4. The browser downloads a JSON file named after the workflow. Keep this file as a configuration backup or import it into the target environment.

The exported file contains the workflow configuration. It does not transfer execution logs, conversation records, or the external resources referenced by the workflow.

## Check Imported Dependencies

After importing, reselect models, datasets, memories, and MCP servers that are available to the importing account. Importing a JSON configuration does not create or transfer those resources. If **Checklist** appears, resolve its reported issues, then save and test the workflow before use.

The exporter clears API keys for selected tool types. Re-enter the required keys after import. It does not guarantee that every password, token, header, or secret in a configuration has been removed; inspect the exported JSON before sharing it.
