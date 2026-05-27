---
sidebar_position: 7
slug: /add_discord
sidebar_custom_props: {
  categoryIcon: SiGoogledrive
}
---

# Add Discord

Integrate Discord as a data source.

---

This guide outlines how to ingest messages from your Discord servers into RAGFlow by setting up a dedicated bot.

## Prerequisites

- Administrative privileges for the target Discord server.
- Permissions to add data sources within your RAGFlow environment.

## Setting up a Discord bot

You need a bot application to access and read messages from your server securely.

- Go to the Discord Developer Portal.
- Select "New Application" and assign it a descriptive name.
- Navigate to the "Bot" section in the left menu and add a new bot.
- Scroll down to the "Privileged Gateway Intents" section and toggle on "Message Content Intent" so the application can extract message text.
- Click "Reset Token" to generate your bot token. Copy this token immediately and store it safely.

## Invite the bot to your server

The bot must be authorized to view the specific channels you intend to sync.

- In the Developer Portal, open the "OAuth2" menu and select "URL Generator".
- Check the `bot` scope.
- In the permission list, select "View Channels" and "Read Message History".
- Copy the resulting URL generated at the bottom of the screen.
- Open this URL in your browser, select your desired server from the dropdown, and approve the authorization prompt.

## Configure the connection in RAGFlow

With the bot active in your server, you can finalize the integration inside RAGFlow.

- Open RAGFlow and access the data sources configuration module.
- Choose "Discord" from the list of supported external platforms.
- Paste your saved bot token into the designated input field.
- Configure any specific channels or indexing preferences as required by the interface.
- Save your settings to establish the connection.
- Attach this newly created Discord data source to your target dataset to begin syncing your conversations.

### Link to a dataset

1. Navigate to the **Dataset** tab.
2. Select or create the target Dataset.
3. Navigate to the Dataset's **Configuration** page and select **Link data source**.
4. Choose the previously created Discord connector in the popup window.