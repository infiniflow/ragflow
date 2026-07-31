---
sidebar_position: 4
title: Component Connection Rules
sidebar_label: Component Connection Rules
slug: /component_connection_rules
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Component Connection Rules
Connections define component execution order. Sequential components run along a single path. Branch components such as `Switch` and `Categorize` route workflows to different exits according to conditions. `Iteration` executes sub-processes in loops. Components not connected to the execution path will not run. Before deleting a component, check upstream/downstream connections and variable references to avoid missing inputs for subsequent nodes.
