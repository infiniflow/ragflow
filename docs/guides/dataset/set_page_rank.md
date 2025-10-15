---
sidebar_position: -2
slug: /set_page_rank
---

# Set page rank

Create a step-retrieval strategy using page rank.

---

## Scenario

In an AI-powered chat, you can configure a chat assistant or an agent to respond using knowledge retrieved from multiple specified datasets (datasets), provided that they employ the same embedding model. In situations where you prefer information from certain dataset(s) to take precedence or to be retrieved first, you can use RAGFlow's page rank feature to increase the ranking of chunks from these datasets. For example, if you have configured a chat assistant to draw from two datasets, dataset A for 2024 news and dataset B for 2023 news, but wish to prioritize news from year 2024, this feature is particularly useful.

:::info NOTE
It is important to note that this 'page rank' feature operates at the level of the entire dataset rather than on individual files or documents.
:::

## Configuration

On the **Configuration** page of your dataset, drag the slider under **Page rank** to set the page rank value for your dataset. You are also allowed to input the intended page rank value in the field next to the slider.

:::info NOTE
The page rank value must be an integer. Range: [0,100]

- 0: Disabled (Default)
- A specific value: enabled
:::

:::tip NOTE
If you set the page rank value to a non-integer, say 1.7, it will be rounded down to the nearest integer, which in this case is 1.
:::

## Scoring mechanism

If you configure a chat assistant's **similarity threshold** to 0.2, only chunks with a hybrid score greater than 0.2 x 100 = 20 will be retrieved and sent to the chat model for content generation. This initial filtering step is crucial for narrowing down relevant information.

If you have assigned a page rank of 1 to dataset A (2024 news) and 0 to dataset B (2023 news), the final hybrid scores of the retrieved chunks will be adjusted accordingly. A chunk retrieved from dataset A with an initial score of 50 will receive a boost of 1 x 100 = 100 points, resulting in a final score of 50 + 1 x 100 = 150. In this way, chunks retrieved from dataset A will always precede chunks from dataset B.