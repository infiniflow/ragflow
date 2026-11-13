---
sidebar_position: 3
sidebar_label: Use Agent with Your Team
title: Use Agent with Your Team
sidebar_custom_props: {
  categoryIcon: LucideBot
}
---

# Use Agent with Your Team

Share workflows and ingestion pipelines from the **Agent** page to enable team collaboration.

## Workflow Member Permissions

After joining the team, members can view, edit, and run shared workflows under **Agent > Workflow**. All members edit the same draft.

See [Workflow canvas: Sharing Scope](../../agent/understand_workflow_canvas.md#sharing-scope) for the configuration steps.

Only the workflow owner can delete the workflow or change its sharing scope. Sharing a workflow does not automatically share private Datasets or memories referenced by its components; configure those resources separately.

### Models Used by Shared Workflows

Shared workflows use the models selected in their components and the model provider configuration in the workflow's workspace, rather than the member's personal defaults. If a model is unavailable, ask the owner to check its configuration.

### Drafts and Published Versions

Members edit the same draft. Canvas changes are autosaved; **Save draft** also saves the current draft explicitly. Neither action updates a published version.

- **Run** tests the draft.
- **Publish** creates a published version. Saving a draft does not update a published version.

A selected published version remains in use until another version is selected. Start a new conversation to test updated replies; existing conversations may retain the earlier workflow configuration.

## Ingestion Pipeline Member Permissions

Find shared pipelines under **Agent > Ingestion pipeline**. Members can view and edit them, and use a published version to process Dataset files. Draft edits do not update the version already selected by a Dataset. Only the owner can delete a pipeline or change its sharing scope.

See [Ingestion pipeline sharing settings](../../agent/ingestion_pipeline/create_ingestion_pipeline.md#sharing-scope) for the configuration steps.
