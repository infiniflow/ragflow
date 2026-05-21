---
sidebar_position: 3
slug: /ecommerce_customer_support_agent
sidebar_custom_props: {
  categoryIcon: LucideStethoscope
}
---

# Build Ecommerce customer support agent

This quickstart guides you through building an intelligent e‑commerce customer support agent. The agent uses RAGFlow’s workflow and Agent framework to automatically handle common customer requests such as product comparisons, usage instructions, and installation bookings—providing fast, accurate, and context-aware responses. In the following sections, we will walk you through the process of building an Ecommerce customer support Agent as shown below: 

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/ecommerce_cs_agent_entirety.PNG)

## Prerequisites

- Sample datasets (available from [Hugging Face](https://huggingface.co/datasets/InfiniFlow/Ecommerce-Customer-Service-Workflow)).

## Procedures

### Prepare datasets

1. Ensure that the above-mentioned sample datasets are downloaded. 
2. Create two datasets:
   - Product Information
   - User Guide
3. Upload the corresponding documents to each dataset.
4. On the configurations page of both datasets, choose **Manual** as chunking method.
   *RAGFlow preserves content integrity by splitting documents at the “smallest heading” level, keeping text and related graphics together.*

### Create an Agent app

1. Navigate to the **Agent** page, create an Agent app to enter the Agent canvas. 
   _A **Begin** component will appear on the canvas._
2. Configure a greeting message in the **Begin** component, for example:

   ```
   Hi! What can I do for you?
   ```
### Add Categorize component

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/add_categorize.png)

This **Categorize** component uses an LLM to recognize user intent and route the conversation to the correct workflow.

### Build a product feature comparison workflow

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/feature_comparison_workflow.png)

1. Add a **Retrieval** component named “Feature Comparison Knowledge Base" and connect it to the “Product Information” dataset.
2. Add an **Agent** component named “Feature Comparison Agent” after the **Retrieval** component.
3. Configure the Agent’s System Prompt:
   ```
   You are a product specification comparison assistant. Help the user compare products by confirming the models and presenting differences clearly in a structured format.
   ```
4. Configure the User Prompt:
   ```
   User's query is /(Begin Input) sys.query
   Schema is /(Feature Comparison Knowledge Base) formalized_content
   ```

### Build a product user guide workflow

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/product_user_guide_workflow.png)

1. Add a **Retrieval** component named “Usage Guide Knowledge Base” and link it to the “User Guide” dataset.
2. Add an Agent component named “Usage Guide Agent.”
3. Set its System Prompt:
   ```
   You are a product usage guide assistant. Provide step‑by‑step instructions for setup, operation, and troubleshooting.
   ```
4. Set the User Prompt:
   ```
   User's query is /(Begin Input) sys.query
   Schema is /(Usage Guide Knowledge Base) formalized_content
   ```

### Build an installation booking assistant

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/installation_booking_assistant.png)

1. Add an **Agent** component named “Installation Booking Agent.”
2. Configure its System Prompt to collect three details:
   - Contact number
   - Preferred installation time
   - Installation address

   *Once all three are collected, the agent should confirm them and notify the user that a technician will call.*

3. Set the User Prompt:
   ```
   User's query is /(Begin Input) sys.query

4. Connect a **Message** component after the three Agent branches.
   *This component displays the final response to the user.*

   ![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/reply_message.png)

5. Click **Save** → **Run** to view execution results and verify that each query is correctly routed and answered.
6. You can test the workflow by asking:
   - Product comparison questions
   - Usage guidance questions
   - Installation booking requests


