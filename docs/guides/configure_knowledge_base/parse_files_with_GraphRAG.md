---
sidebar_position: 1
slug: /parse_files_with_GraphRAG
---

# Parse files with GraphRAG

Generate a knowledge base-level graph using GraphRAG

---

On the **Dataset** page of your knowledge base, you can add metadata to any uploaded file. This approach enables you to 'tag' additional information like URL, author, date, and more to an existing file or dataset. In an AI-powered chat, such information will be sent to the LLM with the retrieved chunks for content generation.

## Scenarios

## Prerequisites

The system's default chat model is used to generate knowledge graph. Before proceeding, ensure that you have an chat model properly configured:

![Image](https://github.com/user-attachments/assets/6bc34279-68c3-4d99-8d20-b7bd1dafc1c1)

## Configurations

### Entity types


### Method



### Entity resolution



### Community report generation



## Procedure

1. On the **Configuration** page of your knowledge base, switch on **Extract knowledge graph** or adjust its settings as needed, and click **Save** to confirm your changes.
   
   _The default GraphRAG configurations for your knowlege base are now set and files uploaded afterwards will automatically use them during file parsing._

:::tip NOTE


## Frequently asked questions

### Can I have different GraphRAG settings for different files in my knowledge base?

Yes, you can. Just one graph is generated per knowledge base. The smaller graphs of your files will be *combined* into one big, unified graph at the end of graph extraction process.