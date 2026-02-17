---
sidebar_position: 35
slug: /docs_generator
---

# Docs Generator component

A component that generates downloadable PDF, DOCX, or TXT documents from markdown-style content with full Unicode support.

---

The **Docs Generator** component enables you to create professional documents directly within your agent workflow. It accepts markdown-formatted text and converts it into downloadable files, making it ideal for generating reports, summaries, or any structured document output.

## Key features

- **Multiple output formats**: PDF, DOCX, and TXT
- **Full Unicode support**: Automatic font switching for CJK (Chinese, Japanese, Korean), Arabic, Hebrew, and other non-Latin scripts
- **Rich formatting**: Headers, lists, tables, code blocks, and more
- **Customizable styling**: Fonts, margins, page size, and orientation
- **Document extras**: Logo, watermark, page numbers, and timestamps
- **Direct download**: Generates a download button for the chat interface

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

The main text content to include in the document. Supports Markdown formatting:

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

### Logo image

Optional. A logo image to display at the top of the document. You can either:

- Upload an image file using the file picker
- Paste an image path, URL, or base64-encoded data

### Logo position

The horizontal position of the logo:

- **left** (default)
- **center**
- **right**

### Logo dimensions

- **Logo width**: Width in inches (default: `2.0`)
- **Logo height**: Height in inches (default: `1.0`)

### Font family

The font used throughout the document:

- **Helvetica** (default)
- **Times-Roman**
- **Courier**
- **Helvetica-Bold**
- **Times-Bold**

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

### Output directory

The server directory where generated documents are saved. Defaults to `/tmp/pdf_outputs`.

### Add page numbers

When enabled, page numbers are added to the footer of each page. Defaults to `true`.

### Add timestamp

When enabled, a generation timestamp is added to the document footer. Defaults to `true`.

### Watermark text

Optional. Text to display as a diagonal watermark across each page. Useful for marking documents as "Draft", "Confidential", etc.

## Output

The **Docs Generator** component provides the following output variables:

| Variable name | Type      | Description                                                  |
|---------------|-----------|--------------------------------------------------------------|
| `file_path`   | `string`  | The server path where the generated document is saved.       |
| `pdf_base64`  | `string`  | The document content encoded in base64 format.               |
| `download`    | `string`  | JSON containing download information for the chat interface. |
| `success`     | `boolean` | Indicates whether the document was generated successfully.   |

### Displaying the download button

To display a download button in the chat, add a **Message** component after the **Docs Generator** and reference the `download` variable:

1. Connect the **Docs Generator** output to a **Message** component.
2. In the **Message** component's content field, type `/` and select `{Docs Generator_0@download}`.
3. When the agent runs, a download button will appear in the chat, allowing users to download the generated document.

The download button automatically handles:
- File type detection (PDF, DOCX, TXT)
- Proper MIME type for browser downloads
- Base64 decoding for direct file delivery

## Unicode and multi-language support

The **Docs Generator** includes intelligent font handling for international content:

### How it works

1. **Content analysis**: The component scans the text for non-Latin characters.
2. **Automatic font switching**: When CJK or other complex scripts are detected, the system automatically switches to a compatible CID font (STSong-Light for Chinese, HeiseiMin-W3 for Japanese, HYSMyeongJo-Medium for Korean).
3. **Latin content**: For documents containing only Latin characters (including extended Latin, Cyrillic, and Greek), the user-selected font family is used.

### Supported scripts

| Script                       | Unicode Range | Font Used          |
|------------------------------|---------------|--------------------|
| Chinese (CJK)                | U+4E00–U+9FFF | STSong-Light       |
| Japanese (Hiragana/Katakana) | U+3040–U+30FF | HeiseiMin-W3       |
| Korean (Hangul)              | U+AC00–U+D7AF | HYSMyeongJo-Medium |
| Arabic                       | U+0600–U+06FF | CID font fallback  |
| Hebrew                       | U+0590–U+05FF | CID font fallback  |
| Devanagari (Hindi)           | U+0900–U+097F | CID font fallback  |
| Thai                         | U+0E00–U+0E7F | CID font fallback  |

### Font installation

For full multi-language support in self-hosted deployments, ensure Unicode fonts are installed:

**Linux (Debian/Ubuntu):**
```bash
apt-get install fonts-freefont-ttf fonts-noto-cjk
```

**Docker:** The official RAGFlow Docker image includes these fonts. For custom images, add the font packages to your Dockerfile:
```dockerfile
RUN apt-get update && apt-get install -y fonts-freefont-ttf fonts-noto-cjk
```

:::tip NOTE
CID fonts (STSong-Light, HeiseiMin-W3, etc.) are built into ReportLab and do not require additional installation. They are used automatically when CJK content is detected.
:::

## Troubleshooting

### Characters appear as boxes or question marks

This indicates missing font support. Ensure:
1. The content contains supported Unicode characters.
2. For self-hosted deployments, Unicode fonts are installed on the server.
3. The document is being viewed in a PDF reader that supports embedded fonts.

### Download button not appearing

Ensure:
1. The **Message** component is connected after the **Docs Generator**.
2. The `download` variable is correctly referenced using `/` (which appears as `{Docs Generator_0@download}` when copied).
3. The document generation completed successfully (check `success` output).

### Large tables not rendering correctly

For tables with many columns or large cell content:
- The component automatically converts wide tables to a definition list format for better readability.
- Consider splitting large tables into multiple smaller tables.
- Use landscape orientation for wide tables.
