---
sidebar_position: 3
slug: /configure_memory
title: Configure Memory
sidebar_label: Configure Memory
sidebar_custom_props: {
  categoryIcon: LucideBox
}
---

# Configure Memory

The configuration page is used to maintain a memory's basic information, model configuration, capacity configuration, and advanced settings. After making changes, click **Confirm** to save them, or click **Cancel** to discard unsaved content.

On the memory page, click the target memory **> Configurations** to view and update its settings.

### Basic Information

The name, avatar, and description are mainly used to distinguish memories. Fill them in according to the page prompts. The avatar image must not exceed 4 MB.

The embedding model and large language model are the basic configuration for memory extraction and retrieval. The embedding model is used to convert memory content into vectors and supports subsequent similarity retrieval. The large language model is used to analyze conversation content and extract structured memories. After messages have already been written to a memory, frequent model changes are not recommended.

### Memory Type

Multiple memory types can be selected, but **Raw (`raw`)** is required and cannot be removed. When multiple types are selected, the system saves or extracts memories at different levels according to the selected types.

- **Raw (`raw`)**: Saves the original conversation content between the user and the Agent. It is suitable for scenarios where the complete context needs to be traced, and it is also the basis for extracting other types of memory.
- **Semantic (`semantic`)**: Extracts relatively stable facts, preferences, attributes, or background information, such as user preferences, customer profiles, and common requirements. It is suitable for long-term reuse.
- **Episodic (`episodic`)**: Extracts experience records with time, conversation, or event context, such as a communication, a task execution result, or a staged event. It is suitable for tracing historical processes.
- **Procedural (`procedural`)**: Extracts processes, steps, operating habits, or handling rules, such as fixed workflows and common handling methods. It is suitable for allowing Agents to reuse approaches in subsequent tasks.

### Memory Size

Memory size is used to limit the capacity that the memory can occupy. The value range is `(0, 5242880]` bytes. After the capacity reaches the upper limit, the system cleans up old content according to the forgetting policy.

### Advanced Settings

Advanced settings are used to control the memory's visibility scope, storage method, and extraction effect. If there are no special requirements, you can keep the default configuration.

- **Permissions**: Select **Only me** or **Team**. **Only me** means that only the current user can see the memory. **Team** means that team members can see and use the memory.
- **Storage type**: Currently, **Table (`table`)** can be selected. It is suitable for general messages, field queries, and regular retrieval.
- **Forgetting policy**: Currently, **First in, first out (FIFO)** can be selected. When the capacity reaches the upper limit, the system preferentially cleans up content written earlier.
- **Temperature**: The value range is `0-1`. It is used to control the randomness of memory extraction. A lower value makes the extraction result more stable, while a higher value makes the expression more divergent.
- **System prompt**: Used to define the role, output format, and structure constraints for memory extraction. Adjust it only when you need to unify the extraction format or strengthen extraction rules.
- **User prompt**: Used to supplement user-side extraction requirements. If there are no special requirements, it can be left empty.
