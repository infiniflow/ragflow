---
sidebar_position: 35
slug: /docs_generator
---

# Docs Generator component

A component that generates downloadable PDF, DOCX, or TXT documents from markdown-style content.

---

The **Docs Generator** component enables you to create professional documents directly within your agent workflow. It accepts markdown-formatted text and converts it into downloadable files, making it ideal for generating reports, summaries, or any structured document output.

## Prerequisites

- Content to be converted into a document (typically from an **Agent** or other text-generating component).

## Examples

You can pair an **Agent** component with the **Docs Generator** to create dynamic documents based on user queries. The **Agent** generates the content, and the **Docs Generator** converts it into a downloadable file. Connect the output to a **Message** component to display the download button in the chat.

A typical workflow looks like:

```
Begin → Agent → Docs Generator → Message
```

In the **Message** component, reference the `download` output variable from the **Docs Generator** to display a download button in the chat interface.

## Configurations

### Content

The main text content to include in the document. Supports markdown formatting:

- **Bold**: `**text**` or `__text__`
- **Italic**: `*text*` or `_text_`
- **Inline code**: `` `code` ``
- **Headings**: `# Heading 1`, `## Heading 2`, `### Heading 3`
- **Bullet lists**: `- item` or `* item`
- **Numbered lists**: `1. item`
- **Tables**: `| Column 1 | Column 2 |`
- **Horizontal lines**: `---`
- **Code blocks**: ` ``` code ``` `

:::tip NOTE
Click **(x)** or type `/` to insert variables from upstream components.
:::

### Title

Optional. The document title displayed at the top of the generated file.

### Subtitle

Optional. A subtitle displayed below the title.

### Output format

The file format for the generated document:

- **PDF** (default): Portable Document Format with full styling support.
- **DOCX**: Microsoft Word format.
- **TXT**: Plain text format.

### Font family

The font used throughout the document:

- **Helvetica** (default)
- **Times-Roman**
- **Courier**

:::tip NOTE
When the document contains CJK (Chinese, Japanese, Korean) or other non-Latin characters, the system automatically switches to a compatible Unicode font (STSong-Light) to ensure proper rendering. The selected font family is used for Latin-only content.
:::

### Font size

The base font size in points. Defaults to `12`.

### Title font size

The font size for the document title. Defaults to `24`.

### Page size

The paper size for the document:

- **A4** (default)
- **Letter**

### Orientation

The page orientation:

- **Portrait** (default)
- **Landscape**

### Margins

Page margins in inches:

- **Margin top**: Defaults to `1.0`
- **Margin bottom**: Defaults to `1.0`
- **Margin left**: Defaults to `1.0`
- **Margin right**: Defaults to `1.0`

### Filename

Optional. Custom filename for the generated document. If left empty, a filename is auto-generated with a timestamp.

### Add page numbers

When enabled, page numbers are added to the footer of each page. Defaults to `true`.

### Add timestamp

When enabled, a generation timestamp is added to the document footer. Defaults to `true`.

## Output

The **Docs Generator** component provides the following output variables:

| Variable name | Type      | Description                                                                 |
| ------------- | --------- | --------------------------------------------------------------------------- |
| `file_path`   | `string`  | The server path where the generated document is saved.                      |
| `pdf_base64`  | `string`  | The document content encoded in base64 format.                              |
| `download`    | `string`  | JSON containing download information. Reference this in a **Message** component to display a download button. |
| `success`     | `boolean` | Indicates whether the document was generated successfully.                  |

### Displaying the download button

To display a download button in the chat, add a **Message** component after the **Docs Generator** and reference the `download` variable:

1. Connect the **Docs Generator** output to a **Message** component.
2. In the **Message** component's content field, type `/` and select `{Docs Generator_0@download}`.
3. When the agent runs, a download button will appear in the chat, allowing users to download the generated document.

## Multi-language support

The **Docs Generator** automatically detects non-Latin characters (Chinese, Japanese, Korean, Arabic, Hebrew, Cyrillic, etc.) and uses appropriate Unicode fonts when available on the server.

:::tip NOTE
For full multi-language support, ensure Unicode fonts are installed on the RAGFlow server:
- **Linux**: `fonts-freefont-ttf`, `fonts-noto-cjk`, or `fonts-droid-fallback`
- **Docker**: Add font packages to the Dockerfile if needed
:::
