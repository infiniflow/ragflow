---
sidebar_position: 9
slug: /add_rss
sidebar_custom_props: {
  categoryIcon: SiGoogledrive
}
---

# Add RSS

Integrate an RSS feed as a data source.

---

This guide explains how to add an RSS feed as a data source to your dataset in RAGFlow.

RSS (Really Simple Syndication) is a standardized web feed format used to publish frequently updated content—such as blog entries, news headlines, and podcasts. By connecting an RSS feed to RAGFlow, you can automatically ingest new content from a website as soon as it is published.

## Benefits

Integrating an RSS data source provides the following advantages:

- **Automated ingestion**: Automatically fetch and process the latest articles, news, and updates from any website or blog that supports RSS or Atom feeds.
- **Dynamic dataset**: Keeps your Retrieval-Augmented Generation (RAG) system up to date with continuous, hands-free content delivery.
- **Deleted-file synchronization**: RAGFlow tracks the state of the RSS feed in the background. If an item is removed from the upstream feed, the system automatically synchronizes this change and deletes the corresponding parsed file from your dataset. This prevents stale or outdated information from polluting your RAG context.

## Prerequisites

- A valid RSS feed URL.
- An existing dataset in RAGFlow.

## Find an RSS feed URL

Before adding the data source, you need the direct URL of the RSS feed you want to monitor. You can typically find this in a few different ways:

- **Look for the RSS icon**: Many blogs and news sites display the standard orange RSS icon, often located in the site's header or footer. For example, tech sites like the **AWS News Blog** or **Smashing Magazine** display this icon prominently. Clicking it usually takes you directly to the feed URL.
- **Try common URL paths**: Often, you can find the feed by appending common RSS paths to the website's main URL (e.g., `https://example.com/rss`, `https://example.com/feed`, or `https://example.com/atom.xml`).
- **Check the page source**: Right-click on the webpage, select **View Page Source**, and press `Ctrl+F` (or `Cmd+F`) to search for `rss` or `application/rss+xml`. The URL will be listed in the `href` attribute of that tag.

## Add an RSS data source

To add an RSS feed to your dataset, follow these steps:

1. Log in to RAGFlow.
2. Navigate to the **Datasets** page and select the dataset you want to populate.
3. Go to the **Dataset** tab and click **+ Add data source**.
4. Select **RSS** from the list of available integrations.
5. In the configuration dialog, configure the following settings:
   - **Name**: Enter a descriptive name to identify this RSS feed.
   - **Feed URL**: Enter the complete URL of the RSS feed (e.g., `https://news.ycombinator.com/rss`).
   - **Batch size**: Specify the maximum number of new articles or items RAGFlow should fetch and process during a single background sync cycle. The default is 2. This setting helps manage the ingestion rate and prevents system overload, especially when connecting to highly active feeds or performing the initial fetch.
   - **Sync deleted files**: Toggle this option on to automatically remove parsed files from your dataset if the corresponding items are deleted from the upstream RSS feed. If disabled, RAGFlow retains all historically ingested content, even if it is no longer available in the source feed.
6. Click **OK** to save the configuration.

*Once configured, RAGFlow's background task executors will automatically poll the RSS feed. The system continuously downloads new entries for parsing and chunking, while concurrently running the deleted-file sync to remove files that are no longer present in the source feed, requiring no manual scheduling on your part.*
