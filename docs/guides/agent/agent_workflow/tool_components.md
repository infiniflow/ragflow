---
sidebar_position: 5
title: Tool Components
sidebar_label: Tool Components
slug: /tool_components
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Tool Components

Tool components connect external search, databases, HTTP APIs, email sending, document generation, financial queries, and browser automation capabilities to Agents. When building Agents, first understand each tool's purpose and security boundaries.

## Tool Selection Suggestions

| Tool Category | Typical Components | Use Case |
| --- | --- | --- |
| Web search | Tavily, Google, DuckDuckGo, SearXNG, Keenable | Retrieve web pages, news, public information, or content from specified sites. |
| Academic search | Google Scholar, ArXiv, PubMed, BGPT | Retrieve papers, medical literature, and research materials. |
| Data and financial queries | Execute SQL, Yahoo Finance, WenCai | Query databases, market data, or financial screening results. |
| Content output | Email, Document Generator | Send emails or generate downloadable documents. |
| Browser automation | Browser | Access web pages, read page content, or perform multi-step browser tasks. |

## Web Page and Information Retrieval

### Tavily Search 

Tavily is a web search service for LLMs. It is suitable for retrieving general web information, news, and content that needs to be limited to specific domains. Keep queries focused on a single topic and avoid overly long natural-language questions.

#### Parameter Description

| Parameter | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| Query | string | Yes | Current user input | Search keywords. |
| Topic | string | No | general | Search type. Options are `general` or `news`. |
| Include Domains | array[string] | No | Empty list | Keep only results from these domains, such as `www.nasa.gov`. |
| Exclude Domains | array[string] | No | Empty list | Exclude results from these domains. |
| API Key | string | No | Empty | Tavily API key. |
| Search Depth | string | No | basic | Search depth. Options are `basic` or `advanced`. |
| Max Results | integer | No | 6 | Maximum number of results. |
| Days | integer | No | 14 | Time range in days for news retrieval. |
| Include Answer | boolean | No | false | Whether to request Tavily's answer field. |
| Include Raw Content | boolean | No | false | Whether to request raw page content. |
| Include Images | boolean | No | false | Whether to request images. |
| Include Image Descriptions | boolean | No | false | Whether to request image descriptions. |

#### Supported Parameter Values

| Parameter | Supported Value | Description |
| --- | --- | --- |
| Topic | general | General web search. |
| Topic | news | News retrieval. |
| Search Depth | basic | Basic search. |
| Search Depth | advanced | Deep search. |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| URLs | `["https://www.ragflow.io/docs/dev/"]` |
| Extract Depth | basic |
| Format | markdown |
| API Key |  |
| Include Images | false |

#### Output Result

The output usually contains search result summaries, titles, links, snippets, and optional image information. `formalized_content` is commonly passed to the Agent to generate answers, while JSON is used by subsequent nodes to read structured fields.

![Tavily Search](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/tavily_search.jpg)

### Tavily Extract 

Tavily Extract reads the body content of one or more known URLs. A common workflow is to use Tavily Search to obtain links, then pass those links to this component to extract page content.

#### Parameter Description

| Parameter | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| URLs | array[string] | Yes | Empty list | URL list to extract. If input is a string, it is split by English commas. |
| Extract Depth | string | No | basic | `advanced` can extract more tables and embedded content, with higher cost and latency. |
| Format | string | No | markdown | Extraction result format. Options are `markdown` or `text`. |
| API Key | string | No | Empty | Tavily API key. |
| Include Images | boolean | No | false | Whether to include images. |

#### Supported Parameter Values

| Parameter | Supported Value | Description |
| --- | --- | --- |
| Extract Depth | basic | Basic extraction. |
| Extract Depth | advanced | Extracts more tables and embedded content, with higher latency and cost. |
| Format | markdown | Markdown format. |
| Format | text | Plain text format. |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| URLs | `["https://www.ragflow.io/docs/dev/"]` |
| Extract Depth | basic |
| Format | markdown |
| API Key |  |
| Include Images | false |

#### Output Result

The output contains the page body, title, URL, and extraction status. `formalized_content` is commonly used as Agent context, while JSON preserves the structured extraction result for each URL.

![Tavily Extract](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/tavily_extract.jpg)

### Google 

Google Search obtains Google organic search results through SerpApi. It is suitable for web retrieval that requires country and language targeting.

#### Parameter Description

| Parameter | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| Query | string | Yes | Current user input | Search keywords. |
| Start | integer | No | 0 | Result offset. In document semantics, this is the pagination starting point. |
| Num | integer | No | 6 | Number of requested results. |
| API Key | string | Yes | Empty | SerpApi API key. |
| Country | string | No | cn | Google region code, such as `cn`, `us`, or `jp`. |
| Language | string | No | en | Google interface/result language code, such as `zh-CN` or `en`. |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| Query | RAGFlow official documentation |
| Start | 0 |
| Num | 6 |
| API Key |  |
| Country | cn |
| Language | en |

#### Output Result

The output contains search result titles, links, and summaries. The organized text can be passed to the Agent for summarization, or subsequent nodes can read the link list from JSON.

![Google Search](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/google_search.jpg)

### DuckDuckGo

DuckDuckGo is a privacy-focused search engine component. It does not require a separate API key and can be used for general web and news retrieval.

#### Parameter Description

| Parameter | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| Query | string | Yes | Current user input | Search keywords. |
| Channel | string | No | general | Message channel: `general` or `news`. |
| Top N | integer | Node configuration | 10 | Maximum number of returned results. |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| Query | RAGFlow open source |
| Channel | general |
| Top N | 10 |

#### Output Result

The output contains titles, links, and summaries returned by DuckDuckGo. It can be used for web material summarization, news lead organization, or subsequent page extraction.

![Duckduckgo](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/duckduckgo_1.jpg)

![Duckduckgo](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/duckduckgo_2.jpg)

### SearXNG 

SearXNG is a self-hostable privacy-oriented meta-search engine. This component calls a user-provided SearXNG instance and is suitable for scenarios that need control over retrieval sources or internal search deployment.

#### Parameter Description

| Parameter | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| Query | string | Yes | Current user input | Search keywords. |
| SearXNG URL | string | Yes | Empty | SearXNG instance base URL, such as `https://searxng.example.com`. |
| Top N | integer | No | 10 | Maximum number of results. |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| Query | RAGFlow documentation |
| SearXNG URL | `https://<your-searxng-host>` |
| Top N | 10 |

#### Output Result

The output contains titles, links, summaries, and source information returned by SearXNG. Before use, configure a reachable SearXNG service address and pass the system security checks.

### Keenable 

Keenable is a web search API for AI Agents. By default, it supports a public free path without a key. After configuring a key, you can increase the limit and enable low-latency realtime mode.

#### Parameter Description

| Field | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| query | string | Yes | `{sys.query}` | Search keywords. |
| site | string | No | Empty | Limit results to a single domain, such as `techcrunch.com`. |
| api_key | string | Node configuration | Empty | Optional Keenable API key. |
| mode | string | Node configuration | pro | `pro` for deeper retrieval or `realtime` for low latency. `realtime` requires an API key. |
| top_n | integer | Node configuration | 10 | Maximum number of results. |
| KEENABLE_API_URL | Environment variable | Deployment configuration | `https://api.keenable.ai` | Service API base URL. Production environments must use HTTPS. Local loopback addresses can use HTTP. |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| Query | RAGFlow Agent workflow |
| Site | github.com |
| API Key |  |
| Mode | pro |
| Top N | 10 |
| Keenable API URL | `https://api.keenable.ai` |

#### Output Result

The output contains search entries, summaries, and links returned by Keenable. It can be summarized by the Agent or passed to subsequent retrieval or extraction nodes. Do not set `realtime` for keyless trial runs.

![Keenable Search](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/keenable_search.jpg)

### Wikipedia
The Wikipedia component searches encyclopedia entries and extracts entry summaries. It is suitable for querying clear entities, concepts, and historical events. Query terms should be as close as possible to the entry title.

#### Parameter Description

| Field | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| query | string | Yes | `{sys.query}` | Specific entry topic or keyword. |
| top_n | integer | Node configuration | 10 | Maximum number of search entries. |
| language | string | Node configuration | en | Wikipedia language code, such as `zh`, `en`, or `ja`. |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| Query | culture |
| Top N | 10 |
| Language | en |

#### Output Result

The output contains the titles, summaries, and page links of matching entries. It is suitable for generating concept explanations or background descriptions with the Agent.

![Wikipedia](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/wikipedia.jpg)

### GitHub

The GitHub component searches repositories through the GitHub Repository Search API and sorts by popularity by default. It is suitable for finding open-source projects, reference implementations, and technology ecosystems.

#### Parameter Description

| Field | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| query | string | Yes | `{sys.query}` | GitHub repository search syntax or keywords. |
| top_n | integer | Node configuration | 10 | Maximum number of returned repositories. |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| Query | RAGFlow |
| Top N | 10 |

#### Output Result

The output contains repository, issue, code, or user search entries, usually including names, links, summaries, update times, and other information. The output includes repository names, links, descriptions, and stars.

![GitHub](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/GitHub.jpg)

## Academic Literature Retrieval

### Google Scholar

Google Scholar is used to retrieve papers, dissertations, books, abstracts, and other academic materials. It is suitable for preliminary literature discovery before a research review, but should not replace verification of original texts and citation information.

#### Parameter Description

| Field | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| query | string | Yes | `{sys.query}` | Paper topic or keyword. |
| top_n | integer | Node configuration | 12 | Maximum number of papers. |
| sort_by | string | Node configuration | relevance | Sorting method: `date` or `relevance`. |
| year_low | integer/null | Node configuration | null | Earliest publication year. |
| year_high | integer/null | Node configuration | null | Latest publication year. |
| patents | boolean | Node configuration | true | Whether to include patents. |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| Query | retrieval augmented generation evaluation |
| Top N | 12 |
| Sort By | relevance |
| Year Low | null |
| Year High | null |
| Patents | true |

#### Output Result

The output contains academic retrieval entries such as paper titles, authors, abstracts, source links, and citation information. It is suitable for literature reviews or research lead organization.

![Google Scholar](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/google_scholar.jpg)

### ArXiv 

ArXiv is used to retrieve open preprints across fields such as computer science, mathematics, physics, and quantitative finance. ArXiv papers may not have undergone peer review, so mark their preprint nature when using the results.

#### Parameter Description

| Field | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| query | string | Yes | `{sys.query}` | Retrieval keywords. |
| top_n | integer | Node configuration | 12 | Maximum number of returned papers. |
| sort_by | string | Node configuration | submittedDate | Sorting method: `submittedDate`, `lastUpdatedDate`, or `relevance`. |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| Query | large language model agents |
| Top N | 12 |
| Sort By | submittedDate |

#### Output Result

The output contains paper titles, authors, abstracts, publication times, categories, and paper links. It can be used by the Agent to generate paper summaries or research comparisons.

![ArXiv](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/arxiv.jpg)

### PubMed 

PubMed is used to retrieve life science and biomedical literature. The component queries through NCBI E-utilities and returns titles, authors, journals, DOIs, abstracts, and other information.

#### Parameter Description

| Field | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| query | string | Yes | `{sys.query}` | PubMed search term. Medical subject headings or Boolean retrieval expressions are supported. |
| top_n | integer | Node configuration | 12 | Maximum number of papers. |
| email | string | Node configuration | `A.N.Other@example.com` | NCBI Entrez contact email. Replace it with a real maintainer email in production. |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| Query | COVID-19 vaccine effectiveness |
| Top N | 12 |
| Email | `A.N.Other@example.com` |

#### Output Result

The output contains medical literature titles, authors, journals, abstracts, publication dates, and PubMed links. It is suitable for medical literature retrieval scenarios.

![PubMed](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/pubmed.jpg)

### BGPT 

BGPT retrieves scientific papers and returns structured evidence, including research methods, sample sizes, results, limitations, conflicts of interest, data availability, and falsifiability tips. It is suitable for evaluating scientific claims, not only for finding paper abstracts.

#### Parameter Description

| Field | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| query | string | Yes | `{sys.query}` | Natural-language scientific retrieval question. |
| top_n | integer | Node configuration | 10 | Maximum number of results. |
| api_key | string | Node configuration | Empty | Optional API key. If not configured, the public service path is used. |
| days_back | integer/null | Node configuration | null | Optional. Limits retrieval to content from the most recent number of days. |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| Query | Does sleep deprivation impair working memory in adults? |
| Top N | 10 |
| API Key |  |
| Days Back | null |

#### Output Result

The output contains biomedical knowledge retrieval results and summaries, which can be further summarized, compared, or used to generate research explanations by subsequent Agents.

![BGPT](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/bgpt.jpg)

## Data and Financial Queries

### Execute SQL 

Execute SQL connects to an external database and executes SQL statements. The result is formatted as text or table content.

To protect system security, the database address must pass server-side security checks. To connect to a local or internal database, first confirm that the deployment environment allows access.

#### Parameter Description

| Field | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| sql | string | Yes | `{sys.query}` | SQL to execute. Canvas variables can be included. |
| db_type | string | Yes | mysql | Supports `mysql`, `postgres`, `mariadb`, `mssql`, `IBMDB2`, `trino`, and `oceanbase`. |
| database | string | Yes | Empty | Database name. Trino uses `catalog.schema` or `catalog`. |
| username | string | Yes | Empty | Database account. |
| host | string | Yes | Empty | Database address, which must pass server-side security checks. |
| port | integer | Yes | 3306 | Database port. |
| password | string | Conditionally required | Empty | Database password. It can be empty for Trino. |
| max_records | integer | No | 1024 | Maximum number of records returned per statement. |

#### Supported Values

| Parameter | Supported Value | Description |
| --- | --- | --- |
| Database Type | mysql | MySQL |
| Database Type | postgres | PostgreSQL |
| Database Type | mariadb | MariaDB |
| Database Type | mssql | Microsoft SQL Server |
| Database Type | IBMDB2 | IBM DB2 |
| Database Type | trino | Trino |
| Database Type | oceanbase | OceanBase |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| SQL | `SELECT * FROM XXX;` |
| Database Type | mysql |
| Database | demo_sql |
| Username | ragflow_reader |
| Host | `<host>` |
| Port | 3306 |
| Password | `<read-only database account password>` |
| Max Records | 10 |

#### Output Result

The output contains SQL execution results, field names, and record content. You can pass formatted text to the Agent for explanation, or let subsequent nodes read structured results.

![Execute SQL](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/execute_sql.jpg)

### Yahoo Finance 

The Yahoo Finance component queries stock quotes, company profiles, historical market data, financial statements, and news through `yfinance`, and outputs the selected content as a Markdown report.

#### Parameter Description

| Field | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| stock_code | string | Yes | `{sys.query}` | Stock code or company name. Use exchange-standard codes when possible. |
| info | boolean | Node configuration | true | Output company and quote information. |
| history | boolean | Node configuration | false | Output historical market data. |
| count | boolean | Node configuration | false | Share count switch defined by the code. |
| financials | boolean | Node configuration | false | The current implementation outputs calendar information. |
| income_stmt | boolean | Node configuration | false | Income statement switch defined by the code. |
| balance_sheet | boolean | Node configuration | false | Output balance sheet and quarterly balance sheet. |
| cash_flow_statement | boolean | Node configuration | false | Output cash flow statement and quarterly cash flow statement. |
| news | boolean | Node configuration | true | Output related news. |

#### Supported Values

| Parameter | Supported Value | Description |
| --- | --- | --- |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| Stock Code | AAPL |
| Info | true |
| History | false |
| Count | false |
| Financials | false |
| Income Statement | false |
| Balance Sheet | false |
| Cash Flow Statement | false |
| News | true |

#### Output Result

The output contains a financial query report and structured market data, which can be used by subsequent Agents to generate market overviews or indicator explanations.

### WenCai 
WenCai is used to screen financial data such as stocks, indices, funds, Hong Kong stocks, U.S. stocks, futures, and other instruments based on natural-language conditions.

#### Parameter Description

| Field | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| query | string | Yes | `{sys.query}` | Stock selection or financial condition, such as "A-shares with P/E ratio below 20". |
| top_n | integer | Node configuration | 10 | Maximum number of results. The frontend default may be 20. |
| query_type | string | Node configuration | stock | Supports `stock`, `zhishu`, `fund`, `hkstock`, `usstock`, `threeboard`, `conbond`, `insurance`, `futures`, `lccp`, and `foreign_exchange`. |

#### Supported Query Types

| Type | Description |
| --- | --- |
| stock | A-share stocks |
| zhishu | Indices |
| fund | Funds |
| hkstock | Hong Kong stocks |
| usstock | U.S. stocks |
| threeboard | NEEQ |
| conbond | Convertible bonds |
| insurance | Insurance |
| futures | Futures |
| lccp | Wealth management products |
| foreign_exchange | Foreign exchange |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| Query | A-shares with P/E ratio below 20 and net profit YoY growth above 20% |
| Top N | 10 |
| Query Type | stock |

#### Output Result

The output contains a financial data list that matches the natural-language screening condition. It can be used for displaying stock selection results, subsequent filtering, or Agent explanation.

:::tip NOTE

Before using the WenCai component, confirm that the WenCai query service is available in the current environment. If the service is not enabled, the component will not return real financial data.

:::

## Communication and External Systems

### Email

The Email component sends HTML emails through SMTP and supports multiple CC addresses. The current version supports recipients, CC recipients, subject, and email body. It does not support adding attachments or BCC recipients through this component.

#### Parameter Description

| Field | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| to_email | string | Yes | `{sys.query}` | Recipient email address. |
| cc_email | string | No | Empty | CC email addresses. Separate multiple addresses with English commas. |
| content | string | No | Empty | Email body, sent as HTML content. |
| subject | string | No | Empty | Email subject. |
| smtp_server | string | Node configuration | Empty | SMTP server address. |
| smtp_port | integer | Node configuration | 465 | SMTP port. Port 465 uses SSL, and other ports use STARTTLS. |
| email | string | Node configuration | Empty | Sender email address. |
| smtp_username | string | Node configuration | Empty | SMTP login account. If empty, the sender email address is used. |
| password | string | Node configuration | Empty | SMTP authorization code or password. |
| sender_name | string | Node configuration | Empty | Sender display name. |

:::tip NOTE

Although some SMTP fields are displayed as optional in the UI, a valid SMTP service address, sender account, and authentication information must be provided before sending email.

:::

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| To Email | receiver@example.com |
| CC Email |  |
| Content | `<p>This is a test email sent by RAGFlow.</p>` |
| Subject | RAGFlow email tool test |
| SMTP Server |  |
| SMTP Port | 465 |
| Email |  |
| SMTP Username |  |
| Password |  |
| Sender Name |  |

#### Output Result

The output contains sending status and error information. `success` being `true` means the email was sent successfully. If sending fails, check the SMTP address, account, password, and recipient.

![Email](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/email.jpg)

### HTTP Request 

The HTTP Request component calls external HTTP APIs, allowing business systems, third-party services, or self-built APIs to be connected to Agent workflows.

#### Parameter Description

| Field | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| url | string | Yes | Empty | API address. Canvas variables can be used.  |
| method | string | Yes | get | Supports `get`, `post`, and `put`. |
| headers | string/object | No | Empty | Request headers in JSON object format. Variables can be used in values. |
| variables | array[object] | No | `[]` | Request parameter list. Each item usually contains `key`, `value`, and `ref`. |
| timeout | integer | No | 60 | Request timeout in seconds. |
| proxy | string | No | Empty | Optional HTTP/HTTPS proxy address. |
| clean_html | boolean | No | false | Whether to clean HTML tags from the response. |
| datatype | string | No | json | Python request body type: `json` or `formdata`. |
| body | string | No | Empty | Raw request body supported by the Go runtime. |
| content_type | string | No | Empty | POST/PUT default to `application/json`. |

#### Supported Values

| Parameter | Supported Value | Description |
| --- | --- | --- |
| Method | GET | GET request. |
| Method | POST | POST request. |
| Method | PUT | PUT request. |
| Proxy | HTTP | HTTP proxy. |
| Proxy | HTTPS | HTTPS proxy address. |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| URL | `https://1.1.1.1/cdn-cgi/trace` |
| Method | GET |
| Headers | `{}` |
| Variables | `[]` |
| Timeout | 30 |
| Proxy |  |
| Clean HTML | false |
| Data Type | json |
| Body |  |
| Content Type |  |

#### Output Result

The output contains response status code, response headers, and response body. Text responses can be passed to the Agent for summarization, while JSON responses can be read by subsequent nodes.

![HTTP Request](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/http_request.jpg)

## Content Generation and Automation

### Document Generator (DocGenerator)

The Document Generator component outputs Markdown content as PDF, DOCX, TXT, Markdown, or HTML files, and stores the generated files in Agent attachment storage.

#### Parameter Description

| Field | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| content | string | Yes | Empty | Markdown content to generate. Upstream outputs can be referenced. |
| output_format | string | Yes | pdf | Output format: `pdf`, `docx`, `txt`, `markdown`, or `html`. |
| filename | string | No | Empty | File name. If empty, it is generated automatically, and illegal file name characters are cleaned. |
| header_text | string | No | Empty | Header text. Applies to PDF/DOCX. |
| footer_text | string | No | Empty | Footer text. Applies to PDF/DOCX. |
| watermark_text | string | No | Empty | Watermark text. Supported by PDF/DOCX/HTML according to the implementation. |
| add_page_numbers | boolean | No | true | Whether to add page numbers. Mainly used for PDF/DOCX. |
| add_timestamp | boolean | No | true | Whether to add generation time. |
| include_download_info_in_content | boolean | No | false | Whether to keep the download information marker in the content. |
| font_size | number | No | 12 | Font size, which must be greater than or equal to 12. |

#### Supported Values

| Parameter | Supported Value | Description |
| --- | --- | --- |
| Output Format | pdf | PDF document. |
| Output Format | docx | Word document. |
| Output Format | html | HTML document. |
| Output Format | txt | Text file. |
| Output Format | markdown | Markdown file. |
| Font Size | >=12 | Font size must be greater than or equal to 12. |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| Content | `# RAGFlow Agent
Test This document is generated by the DocGenerator component.` |
| Output Format | pdf |
| Filename | ragflow-agent-test.pdf |
| Header Text | RAGFlow |
| Footer Text | Internal Test |
| Watermark Text | DRAFT |
| Add Page Numbers | true |
| Add Timestamp | true |
| Include Download Info In Content | false |
| Font Size | 12 |

#### Output Result

The output contains generated file attachment information, download links, and file names. Users can preview, download, or pass the file to subsequent nodes for further processing.

![Document Generator](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/document_generator.jpg)

### Browser (Browser)

Browser is an LLM-driven browser automation component. It can access web pages, perform multi-step operations, read page content, upload source files, and collect downloaded files based on natural-language tasks. It depends on the configured model and the browser automation dependencies in the runtime environment.

#### Parameter Description

| Field | Type | Required | Default Value | Description |
| --- | --- | --- | --- | --- |
| llm_id | string | Yes | Empty | Configured chat model ID used by Browser. The Go path can accept `model_id` as an alias. |
| prompts | string | Yes | `{sys.query}` | Natural-language browser task. Canvas variables are supported. The Go path can accept `prompt` as an alias. |
| max_steps | integer | No | 30 | Maximum number of browser execution steps. Effective in the Python path. The current Go Stagehand path accepts this field but does not use it during execution. |
| headless | boolean | No | true | Whether to run the browser in headless mode. |
| enable_default_extensions | boolean | No | false | Whether to enable default `browser-use` extensions. |
| chromium_sandbox | boolean | No | false | Whether to enable the Chromium sandbox. In Docker root environments, keep this disabled in general. |
| persist_session | boolean | No | true | Whether to reuse the browser user directory for the same node. Effective in the Python path. |
| upload_sources | array/string | No | `[]` | File IDs, URLs, or upstream variable references for the browser task. |
| url | string | No | Empty | Compatibility field accepted by the current Go component. It does not participate in Stagehand execution. |
| timeout | integer | No | 0 | Compatibility field accepted by the current Go component. It does not participate in Stagehand execution. |

#### Supported Values

| Parameter | Supported Value | Description |
| --- | --- | --- |
| Upload Sources | File ID | File ID uploaded to RAGFlow. |
| Upload Sources | URL | Accessible file or web page URL. |
| Upload Sources | Upstream variable reference | File or resource reference from a previous node. |

#### Configuration Example

| Configuration Item | Example Value |
| --- | --- |
| LLM ID | `<configured chat model ID>` |
| Prompt | Open https://www.ragflow.io, identify the main navigation items, and return them as a concise bullet list. |
| Max Steps | 10 |
| Headless | true |
| Enable Default Extensions | false |
| Chromium Sandbox | false |
| Persist Session | true |
| Upload Sources | `[]` |
| URL |  |
| Timeout | 0 |

#### Output Result

The output contains the browser task execution summary, extracted page content, and any generated download file information. Avoid assigning Browser tasks that involve login, payment, or irreversible submissions.
