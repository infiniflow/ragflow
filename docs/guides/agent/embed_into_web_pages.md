---
sidebar_position: 5
title: Embed into Web Pages
sidebar_label: Embed into Web Pages
slug: /embed_into_web_pages
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Embed into Web Pages

## Embed Agent Via Webpage
Embed a **Workflow** Agent in a website as a fullscreen chat or floating widget.

Follow these steps to embed an Agent from a self-managed deployment. Obtain an API key under **User settings > API** before configuring the embed; see [Acquire a RAGFlow API key](../../develop/acquire_ragflow_api_key.md).

Steps:
1. On the Agent page, click the target Agent to open its editing page.
2. Click **Management > Embed into webpage** in the upper right corner of the canvas to open the embedding settings.

![Embed Agent into webpage menu](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/embed_agent_into_webpage.jpg)
3. In **Embed Setup**, choose **Fullscreen chat (traditional iframe)** or **Floating widget (Intercom-style)**.
4. Set **Language**, **User ID**, and display options as needed. Enable **Published** to use the published version of the Agent.
5. For a floating widget, use **Widget Customization** to adjust its title, text, and colors. Click **Save widget settings** to save the settings.
6. Copy **Embed code** into your webpage. Use **Open in new tab** to preview the result.

![Agent embedding settings](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/website/blog/2026-04-23-ragflow-0.25-ingestion-pipeline-agent-sandbox-and-user-level-memory/embed_into_webpage.png)
