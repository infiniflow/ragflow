---
sidebar_position: 7
title: Build a Customer Support Agent
slug: /build_customer_support_agent
---

# Build a Customer Support Agent

Build a workflow that answers product questions through retrieval and collects customer details for an installation visit.

## Prepare

- Configure a chat model.
- Create a knowledge base containing a short product guide, and finish parsing it. For a repeatable test, include: “Model A supports Wi-Fi. To reset it, hold the reset button for five seconds.”

## Build the workflow

1. [Create a blank Workflow Agent](../creation_and_management.md#create-from-blank-agent) named `Customer support demo`.
2. After **Begin**, add **Categorize**. Use `sys.query` as the query variable and select the chat model.
3. Add two categories:

| Name | Description | Example |
| --- | --- | --- |
| Product help | Product features, setup, and troubleshooting. | How do I reset Model A? |
| Appointment | Requests to arrange a service appointment. | I want to arrange an installation. |

### Product help branch

1. Connect **Product help → Retrieval → Agent → Message**.
2. In Retrieval, select the product knowledge base and use `sys.query` as the query.
3. In Agent, select the chat model and use this **System prompt**:

```text
Answer product questions using the provided product guide.
If the guide does not contain the answer, say so and ask for clarification.
```

4. In **User prompt**, enter `Question: ` and insert `sys.query`. On a new line, enter `Product guide: ` and insert Retrieval's `formalized_content`. Open the variable selector with `/` and select the components on your canvas. Names in this example are illustrative; do not type them as variable references.
5. In Message, insert the Agent's `content` output.

### Appointment branch

Connect **Appointment → Await Response → Message**. Follow the [form example](../agent_workflow/dialogue_component.md#example-collect-appointment-details) to collect the customer's name and preferred installation time, then display them in Message. This branch collects installation preferences; it does not book a service visit.

## Test

Save, then click **Run** and test each path:

| Input | Expected result |
| --- | --- |
| How do I reset Model A? | Product help branch; the answer mentions holding reset for five seconds. |
| Does Model A support Bluetooth? | Product help branch; the answer says the guide does not establish this. |
| I want to arrange an installation. | Appointment branch; a form appears and the submitted name and time are displayed. |

If routing is wrong, refine the category descriptions and examples. If retrieval is empty, check parsing status, knowledge-base selection, and the query. If the answer ignores retrieved text, check the Agent's variable references. Publish only after both branches work as intended.
