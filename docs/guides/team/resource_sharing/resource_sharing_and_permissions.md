---
sidebar_position: 1
sidebar_label: Resource Sharing and Permissions
title: Resource Sharing and Permissions
---

# Resource Sharing and Permissions

This section describes sharing scope, access conditions, and member permissions for shared resources.

## Sharing Scope

Resources that support sharing have two scopes:

| Scope | Who can access the resource |
| --- | --- |
| Only me | Only the owner. |
| Team | The owner and members who have joined the owner's team. |

Workflow and ingestion pipeline settings display **me** for **Only me**; it is the same private scope.

### Find the Sharing Setting

| Resource | How to configure sharing | Team usage guide |
| --- | --- | --- |
| Dataset | [Dataset sharing settings](../../dataset/configuration.md#sharing-scope) | [Use Datasets with Your Team](use_datasets_with_your_team.md) |
| Workflow | [Workflow sharing settings](../../agent/understand_workflow_canvas.md#sharing-scope) | [Workflow team usage](use_agent_with_your_team.md#workflow-member-permissions) |
| Ingestion pipeline | [Ingestion pipeline sharing settings](../../agent/ingestion_pipeline/create_ingestion_pipeline.md#sharing-scope) | [Ingestion pipeline team usage](use_agent_with_your_team.md#ingestion-pipeline-member-permissions) |
| Memory | [Memory sharing settings](../../memory/configure_memory.md#sharing-scope) | [Use Memories with Your Team](use_memories_with_your_team.md) |

## Access Rules

Members can access a resource when both conditions are met:

- They have accepted the invitation to the resource owner's team.
- The owner has set the resource's sharing scope to **Team** and saved the setting.

An invitation awaiting acceptance does not grant access. Private resources remain available only to their owner.

### When Access Ends

Members lose team-based access when the owner makes the resource private, they leave the team, or the owner removes them.

Document creators retain permission to read, update, and delete their own documents, even after team-based access ends. This does not grant access to the entire Dataset or documents created by other members.

Joining another team does not share resources from a member's own workspace with that team.

## Member Permissions

The following tables summarize member permissions for shared resources. Available actions vary by resource type.

### Dataset Permissions

| Action | Workspace owner | Joined team member |
| --- | --- | --- |
| View files and use the Dataset for retrieval | Yes | Yes |
| Upload and parse files | Yes | Yes |
| Edit or delete files in the Dataset | Yes | Yes |
| Update Dataset configuration, including parsing settings | Yes | Yes |
| Change the Dataset's sharing scope | Yes | No |
| Delete the Dataset itself | Yes | No |

Deleting files does not delete the Dataset itself; only the workspace owner can delete the Dataset.

### Other Resource Permissions

The following table lists the permissions of members who have joined the resource owner's team.

| Shared resource | View | Edit | Run or use | Delete resource | Change sharing |
| --- | --- | --- | --- | --- | --- |
| Workflow | Yes | Yes | Yes | Owner only | Owner only |
| Ingestion pipeline | Yes | Yes | Use a published version in a Dataset | Owner only | Owner only |
| Memory | Yes | Yes | Yes | Yes | Owner only |

For Memory message controls and the effect of deletion, see [Use Memories with Your Team](use_memories_with_your_team.md).

Chat has no team sharing setting. Members can create their own Chat using a shared Dataset; the owner's Chat and conversations remain private.

For model use, see [Dataset parsing](use_datasets_with_your_team.md#parse-files-as-a-team-member), [Chat with a shared Dataset](use_datasets_with_your_team.md#use-a-shared-dataset-in-chat), [shared Workflows](use_agent_with_your_team.md#models-used-by-shared-workflows), or [shared Memories](use_memories_with_your_team.md).

#### Why Can I See a Button but Not Complete the Action?

Some owner-only controls remain visible to members. For example, a member may see **Delete** for a shared workflow but cannot delete it. Ask the owner to perform these actions.
