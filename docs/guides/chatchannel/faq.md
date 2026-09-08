---
sidebar_position: 4
title: FAQ
sidebar_label: FAQ
slug: /chat_channels_faq
sidebar_custom_props: {
  categoryIcon: LucideMessagesSquare
}
---

# FAQ

## The Channel Is Saved Successfully but Does Not Reply

First confirm that the channel has been connected to a Chat. If it is not connected to a Chat, RAGFlow ignores external messages and does not generate replies.

## The Bot Cannot Start

Check whether the third-party platform credentials are correct, whether the bot has been enabled, and whether message event permissions have been granted. After credentials are modified, RAGFlow automatically restarts the corresponding channel connection.

## Messages Can Be Received but Replies Fail

Check whether the bot has permission to send messages, and confirm that the bound Chat can answer normally on the RAGFlow page. For `WeCom Webhook`, also confirm that `CorpID`, `AgentID`, and `Secret` can obtain `access_token` normally.

## The WhatsApp QR Code Is Not Displayed

First confirm that the channel has been saved, then wait a few seconds to see whether the page generates a QR code. If the QR code is still not displayed, confirm that `WhatsAppGateway` has started and that the RAGFlow backend can access the gateway service. If the login state has expired or been cleared, scan the QR code again for pairing.
