---
sidebar_position: 9
title: Search with AnySearch
sidebar_label: Search with AnySearch
slug: /use_anysearch
sidebar_custom_props: {
  categoryIcon: RagHTTP
}
---

# Search with AnySearch

Use **AnySearch** for general web searches. Add **AnySearch Discover** when an Agent needs to choose a vertical search tag and its parameters from the current catalog.

Use [AnySearch Extract](./use_anysearch_extract.md) to read a public page returned by a search or supplied by the user.

AnySearch works anonymously: leave **API Key** blank. To use your account's quota, enter your own key in each tool you configure. Service limits still apply. Keep keys in the tool configuration, outside prompts and model tool arguments.

## Add AnySearch to an Agent

1. Open a workflow and select its **Agent** component. Choose a model that supports tool calls.
2. Under **Tools**, click **Add tools**. In **Built-in**, select **AnySearch** from **Search**. Also select **AnySearch Discover** for vertical searches.
3. Use the edit button beside each selected tool to configure it. Discover only needs the optional **API Key**. Search has these settings:

   | Setting | Default | Use |
   | --- | --- | --- |
   | API Key | Blank | Anonymous access, or your own key for account quota. |
   | Top N | `10` | Maximum results per search, from `1` to `10`. |
   | Zone | **Not set** | Use AnySearch's default, or choose **International** (`intl`) or **China** (`cn`). |
   | Language | Blank | Use AnySearch's default, or enter a preferred language such as `en` or `zh-CN`. |

4. In the Agent prompt, describe when to search. For example: `Search for current information when needed. Before a vertical search, discover the available domains, then inspect the relevant domain's tags and required parameters. Report failed searches instead of treating them as evidence.`
5. Connect the Agent to a **Message** component and insert the Agent's `content` output. Save the draft and click **Run**. Ask a focused question, then inspect the tool calls and results in the execution logs.

The Agent supplies search queries at runtime. Its `max_results`, `zone`, and `language` arguments override the configured search defaults for that call.

### Discover a vertical search

The following JSON examples are model tool arguments, not text to paste into the configuration fields.

1. Call `anysearch_discover` with `{}` to list current domain IDs.
2. Call it again with one to five IDs from that response. For example, use `{"domains":["code"]}` only if `code` appears in the returned list.
3. Read the returned tags and parameter requirements. Call `anysearch_search` with `query`, the exact `tag`, and a `params` object containing the required parameters. Include a required parameter with an empty string when it does not apply.

Omit `tag` and `params` for a general web search. Catalog entries can change; use discovery rather than assuming a tag is available. If discovery reports that its output is too large, request fewer domains.

### Run independent searches together

The Agent can call `anysearch_search` with one `query`, or with a `queries` array containing one to five independent searches. For example:

```json
{
  "queries": [
    {
      "query": "RAGFlow official documentation",
      "zone": "intl",
      "language": "en",
      "max_results": 3
    },
    {
      "query": "RAGFlow release notes",
      "max_results": 2
    }
  ]
}
```

With `queries`, put each search's `tag`, `params`, `zone`, `language`, and `max_results` inside its own item. Do not also supply a top-level `query` or search options. Searches run in parallel and return records in input order, with a zero-based `index`, the query, results, and a `request_id` when available.

A failed item keeps its own `_ERROR`; other searches can still succeed. For example, `{"queries":[{"query":"RAGFlow documentation"},{"tag":"code.doc"}]}` runs the first search and reports the second item's missing query without sending that item to AnySearch. An invalid whole-call shape, such as more than five items, fails the whole call. If all items fail, the response also has a top-level `_ERROR`.

## Use AnySearch as a Canvas component

Use a separate **AnySearch** component when the workflow should always search before generating an answer. The Canvas form runs a single query; discovery and batch selection belong to Agent tool calls.

1. Connect **Begin → AnySearch → Agent → Message**.
2. Select **AnySearch** and set **Query** to `sys.query` using the variable selector. Keep **Top N** at `10`, or reduce it. Configure the optional key, Zone, and Language as needed.
3. In the Agent's user prompt, insert AnySearch's `formalized_content` output using the variable selector and ask the Agent to answer from those sources.
4. In **Message**, insert the Agent's `content` output. Save the draft, click **Run**, and ask a question such as `Where can I find RAGFlow documentation?` Check the AnySearch output before assessing the answer.

AnySearch provides two Canvas outputs:

| Output | Use |
| --- | --- |
| `formalized_content` | Query headings and reference text for a downstream prompt, or an error for a failed query. |
| `json` | An array of query records with structured results and any per-query errors. |

## Read results and handle limits

- Each search returns at most `10` results. Result `content` and `snippet` are each shortened to `1,500` characters; `truncated_fields` identifies shortened fields by result index. Treat shortened text as an excerpt.
- References require an absolute HTTP or HTTPS URL and nonempty text. Reference text uses `content`, falling back to `snippet`. Other results remain available in JSON but do not become references.
- A response larger than `4 MiB`, or query results exceeding the output budget, produces an error. Reduce the result count or narrow the query. Discovery output is limited to `128 KiB`.
- Argument, network, HTTP, and AnySearch service errors appear as `_ERROR`. Inspect each query record in a batch and retain any available `request_id` for troubleshooting. Successful searches can return no results; that alone does not establish that the requested information does not exist.
