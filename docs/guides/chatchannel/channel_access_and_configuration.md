---
sidebar_position: 2
title: Channel Access and Configuration
sidebar_label: Channel Access and Configuration
slug: /channel_access_and_configuration
sidebar_custom_props: {
  categoryIcon: LucideMessagesSquare
}
---

# Channel Access and Configuration

## Discord

Users can chat with a Chat directly in Discord. This is suitable for community support, product Q&A, user feedback collection, and similar scenarios.

Configuration prerequisites:

- Create an application and bot in the Discord Developer Portal.
- Obtain the `BotToken` on the **Bot** page.
- Generate an invitation link on the **OAuth2** page and select the `bot` scope.
- Grant the bot permissions to send messages and read message history.
- Invite the bot to the target server.

Configuration parameters:

- **Name**: The channel name displayed in RAGFlow, used to distinguish different Discord bots.
- **BotToken**: The Discord bot authentication credential. Obtain it from **Discord Developer Portal > Bot**. An incorrect value causes the bot to fail to connect to Discord.
- **ApplicationID** (optional): The Discord application identifier. Obtain it from **Discord Developer Portal > General Information**. Leaving it empty does not affect basic message receiving and replying.

Usage:

When adding a Discord channel in RAGFlow, fill in the `BotToken` obtained from the Discord Developer Portal and connect the channel to a Chat after saving it. The bot must join the target server and have permissions to read and send messages. If `MessageContentIntent` is not enabled, the bot may appear online but fail to read the body text of user messages.

Connection verification:

After completing configuration, send a test message to the bot in Discord to verify whether replies are received normally.

## DingTalk

The DingTalk channel is suitable for organizational collaboration scenarios. Members can complete knowledge Q&A, process consultation, and business support in DingTalk conversations.

Configuration prerequisites:

- Create an application in the DingTalk Open Platform.
- Add bot capabilities to the application.
- Enable permissions related to bot message sending and cards, such as `qyapi_robot_sendmsg`, `Card.Streaming.Write`, and `Card.Instance.Write`.
- Publish the application version to make the configuration take effect.
- Obtain the `ClientID` and `ClientSecret`.

Configuration parameters:

- **Name**: The channel name displayed in RAGFlow, used to distinguish different DingTalk bots.
- **ClientID**: The DingTalk application identity identifier. Obtain it from **DingTalk Open Platform > Application Credentials**. An incorrect value causes message connection establishment to fail.
- **ClientSecret**: The DingTalk application secret. Obtain it from **DingTalk Open Platform > Application Credentials**. An incorrect value causes DingTalk platform authentication to fail.

Usage:

When adding a DingTalk channel in RAGFlow, fill in the `ClientID` and `ClientSecret` from the DingTalk Open Platform and connect the channel to a Chat after saving it. DingTalk replies depend on the conversation `Webhook` returned in message events. Therefore, the user must first send a message to the bot in DingTalk before RAGFlow can return a reply in the corresponding conversation.

Connection verification:

After completing configuration, send a test message to the bot in DingTalk to verify whether replies are received normally.

## Feishu / Lark

The Feishu / Lark channel helps organization members use knowledge assistants in daily chat windows. It is suitable for enterprise knowledge Q&A, customer service collaboration, and internal business assistants.

Configuration prerequisites:

- Create an application in the Feishu Open Platform or Lark Developer Console.
- Enable bot capabilities.
- Configure long connections or event subscriptions.
- Enable permissions related to user IDs, messages, and groups.
- Obtain the `AppID` and `AppSecret`.

Configuration parameters:

- **Name**: The channel name displayed in RAGFlow, used to distinguish different Feishu / Lark bots.
- **AppID**: The Feishu / Lark application identifier. Obtain it from **Feishu Open Platform / Lark Developer Console > Application Credentials**. An incorrect value causes the bot to fail to establish a connection.
- **AppSecret**: The Feishu / Lark application secret. Obtain it from **Feishu Open Platform / Lark Developer Console > Application Credentials**. An incorrect value causes the bot to fail to receive or send messages.
- **Domain**: The platform region setting. Select **Feishu(mainland)** for mainland China Feishu, and select **Lark(international)** for the international Lark version. An incorrect selection causes authentication or API calls to fail.

Usage:

When adding a Feishu / Lark channel in RAGFlow, fill in the `AppID` and `AppSecret` from the open platform and select the `Domain` based on the platform where the application is located. After saving the configuration, connect the channel to a Chat and add the bot to the Feishu / Lark conversation where it will be used.

Connection verification:

After completing configuration, send a test message to the bot in Feishu or Lark to verify whether replies are received normally.

## QQ Bot

The QQ Bot channel is suitable for providing automatic Q&A services to QQ users. It can cover one-on-one chats, group chats, channels, private messages, and other conversation scenarios.

Configuration prerequisites:

- Create a bot in the QQ Open Platform or QQ Bot Open Platform.
- Obtain the bot `AppID`.
- Obtain the bot `ClientSecret`.
- Enable the message event permissions required for use.
- Add the bot to the QQ conversation scope it needs to serve.

Configuration parameters:

- **Name**: The channel name displayed in RAGFlow, used to distinguish different QQ bots.
- **AppID**: The QQ bot application identifier. Obtain it from **QQ Bot Platform > Application Information**. An incorrect value causes the bot to fail to connect to the gateway.
- **ClientSecret**: The QQ bot application secret. Obtain it from **QQ Bot Platform > Application Credentials**. An incorrect value causes authentication to fail.
- **BaseURL** (optional): The base address of the QQ Bot API. Keep the default value in ordinary environments. Fill it in only when using a proxy or a specified API address. An incorrect value causes gateway address retrieval or message sending to fail.

Usage:

When adding a QQ Bot channel in RAGFlow, fill in the `AppID` and `ClientSecret` obtained from the QQ platform and connect the channel to a Chat after saving it. In ordinary public network environments, keep `BaseURL` empty and use the system default address. Fill in this parameter only when the platform requires another API domain name, a proxy gateway, or a private forwarding address.

Connection verification:

After completing configuration, send a test message to the bot in QQ to verify whether replies are received normally.

## Telegram

The Telegram channel supports real-time conversations with users through a bot. It is suitable for overseas user services, community operations, and lightweight message bot scenarios.

Configuration prerequisites:

- Use BotFather to create a Telegram bot.
- Obtain the `BotToken`.
- If using the bot in a group, add it to the target group.
- Check the bot privacy mode and group permissions.

Configuration parameters:

- **Name**: The channel name displayed in RAGFlow, used to distinguish different Telegram bots.
- **BotToken**: The Telegram bot authentication credential. Obtain it from **Telegram > BotFather**. An incorrect value causes the bot to fail to start.

Usage:

If you need to use the bot in a group, add it to the target group and confirm that it can receive the messages that need to be processed. Group privacy mode may affect whether the bot can read ordinary messages.

Connection verification:

After completing configuration, send a test message to the bot in Telegram to verify whether replies are received normally.

## WeCom

The WeCom channel is suitable for providing knowledge Q&A, employee services, and business bot capabilities inside an enterprise. You can select `Webhook` or `WebSocket` access based on the deployment environment.

Configuration prerequisites:

- Create a WeCom application or smart bot based on the access method.
- When using `Webhook`, prepare the enterprise ID, application ID, application secret, and callback configuration.
- When using `WebSocket`, create a smart bot through the WeCom API.
- Obtain the `BotID` and `Secret` required by the `WebSocket` method.
- Confirm that the bot has been enabled in WeCom.

Configuration parameters:

WeCom supports two connection methods: `Webhook` and `WebSocket`. Different parameters are required for different methods.

Parameters required by the `Webhook` method:

- **Name**: The channel name displayed in RAGFlow, used to distinguish different WeCom bots.
- **ConnectionType**: Select `Webhook`. After selecting it, you need to fill in `CorpID`, `AgentID`, `Secret`, `Token`, and `AESKey`.
- **Secret**: The WeCom application secret. Obtain it from **WeCom Admin Console > Application Management**. An incorrect value causes WeCom messages to fail to send.
- **CorpID**: The enterprise subject identifier. Obtain it from **WeCom Admin Console > Enterprise Information**. An incorrect value causes callback verification or API authentication to fail.
- **AgentID**: The WeCom application identifier. Obtain it from **WeCom Admin Console > Application Management**. An incorrect value causes application message sending to fail.
- **Token**: The callback signature verification credential. Obtain it from **WeCom Admin Console > Callback Configuration**. An incorrect value causes callback verification to fail.
- **AESKey**: The message encryption and decryption key. Obtain it from **WeCom Admin Console > Callback Configuration**. An incorrect value causes messages to fail to decrypt.

Parameters required by the `WebSocket` method:

- **Name**: The channel name displayed in RAGFlow, used to distinguish different WeCom bots.
- **ConnectionType**: Select `WebSocket`. After selecting it, you only need to fill in `BotID` and `Secret`.
- **BotID**: The WeCom smart bot identifier. Obtain it from **WeCom Admin Console > Smart Bot**. An incorrect value causes long-connection subscription to fail.
- **Secret**: The long-connection subscription secret. Obtain it from **WeCom Admin Console > Smart Bot**. An incorrect value causes `WebSocket` authentication to fail.

Usage:

The credentials for `Webhook` and `WebSocket` cannot be mixed. When selecting `WebSocket`, first create a smart bot in WeCom, then fill the obtained `BotID` and `Secret` into RAGFlow. When selecting `Webhook`, fill in `CorpID`, `AgentID`, `Secret`, `Token`, and `AESKey` according to the WeCom application callback configuration.

After saving the channel, connect the WeCom channel to a Chat, or bind it to a custom Agent (see below). After the connection succeeds, users can send messages to the bot in WeCom, and the bot calls the bound Chat or Agent to return replies.

### Binding a custom Agent

Besides a Chat, a WeCom channel can be bound to a custom Flow Agent, so inbound messages are answered by the workflow you designed on the **Agent** canvas instead of a regular Chat.

Prerequisites:

- Design a Flow Agent in RAGFlow (**Agent > Flow agents**) and make sure the models used by its components are configured for the tenant (for example, the LLM node must point to a valid model of the tenant that owns the channel).
- The agent must be owned by, or shared (permission `team`) with, the tenant that owns the WeCom channel.
- Complete the WeCom channel connection first (`Webhook` or `WebSocket`), as described above.

Configuration parameters:

- **Target type**: Decides how inbound messages are answered. Select **Chat assistant** to bind a Chat, or **Agent** to bind a custom Flow Agent. A channel can be bound to exactly one target at a time: switching the target clears the previous binding, and clearing the selection disconnects the channel.
- **Agent**: When **Target type** is `Agent`, select the Flow Agent that should answer the messages. The dropdown lists the Flow Agents (canvas category `agent_canvas`) accessible to the current tenant; the list is loaded from the tenant's agents, so no external credential is required. If the selected agent has components without a configured model, the runtime replies with an error message.

Usage:

1. Open **User settings > Chat channels**, find the WeCom channel, and click the link icon to open the connect dialog.
2. In the connect dialog, set **Target type** to `Agent`.
3. Select the Flow Agent from the **Agent** dropdown and click confirm.
4. Send a test message to the bot in WeCom and verify the reply.

### Thinking placeholder

To keep the end user informed while a completion is still running, the WeCom channel first sends a short `🤔 开始思考...` placeholder for every accepted, non-empty inbound message, and then sends a second message with the final answer once the Chat or Agent has produced a result. Blank messages are ignored, and if a completion produces no result, no final reply is sent. This applies to both Chat and custom Agent bindings.

The placeholder is only sent on the WeCom channel. The final reply is also cleaned up before it is sent: reasoning content (the `<think>` blocks produced by the model) is removed, so end users only see the final answer instead of the internal thinking process.

Connection verification:

After completing configuration, send a test message to the bot in WeCom to verify whether replies are received normally.

## WhatsApp

The WhatsApp channel connects an account through QR pairing. It is suitable for service scenarios that use WhatsApp to stay in contact with external users. This method does not require open platform keys to be filled in on the page.

Configuration prerequisites:

- Prepare a WhatsApp account that can be connected.
- Confirm that the mobile WhatsApp app can scan QR codes.
- Confirm that the mobile app can access **Linked devices**.
- Confirm that the RAGFlow backend `WhatsAppGateway` is running normally.

Configuration parameters:

- **Name**: The channel name displayed in RAGFlow, used to distinguish different WhatsApp connections.

WhatsApp completes account connection through QR pairing. You do not need to fill in open platform keys on the page. After filling in the name and saving the channel, wait a few seconds until the page generates the QR code used for pairing.

Usage:

When adding a WhatsApp channel, fill in the name first and save it. After saving, the page may first display prompt information. Wait a few seconds and use the mobile phone to scan the QR code after it is generated.

Complete the scanning operation in the mobile WhatsApp app: go to settings, click **Linked devices**, select **Link a device**, and then scan the QR code on the RAGFlow page. After successful scanning, subsequent messages sent by users to this WhatsApp account enter RAGFlow.

WhatsApp depends on the `WhatsAppGateway` in the RAGFlow backend. By default, RAGFlow connects to the local gateway service and stores the login state in the gateway data directory. Ordinary users do not need to fill in these gateway parameters on the page. If the administrator has separately configured the gateway address, access token, or session directory for the deployment environment, use the runtime environment provided by the administrator.

Connection verification:

After completing QR scanning and connecting the channel to a Chat, use another WhatsApp account to send a test message to this account and verify whether replies are received normally.

:::caution NOTE

After deleting a WhatsApp channel or clearing the gateway login state, you may need to scan the QR code again for pairing. If the QR code is not displayed for a long time, confirm that `WhatsAppGateway` has started and check network connectivity between the backend service and the gateway.

:::
