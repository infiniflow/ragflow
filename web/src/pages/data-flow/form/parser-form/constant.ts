import { FileType } from '../../constant';

export enum PdfOutputFormat {
  Json = 'json',
  Markdown = 'markdown',
}

export enum SpreadsheetOutputFormat {
  Json = 'json',
  Html = 'html',
}

export enum ImageOutputFormat {
  Text = 'text',
}

export enum EmailOutputFormat {
  Json = 'json',
  Text = 'text',
}

export enum TextMarkdownOutputFormat {
  Text = 'text',
}

export enum DocxOutputFormat {
  Markdown = 'markdown',
  Json = 'json',
}

export enum PptOutputFormat {
  Json = 'json',
}

export enum VideoOutputFormat {
  Json = 'json',
}

export enum AudioOutputFormat {
  Text = 'text',
}

export const OutputFormatMap = {
  [FileType.PDF]: PdfOutputFormat,
  [FileType.Spreadsheet]: SpreadsheetOutputFormat,
  [FileType.Image]: ImageOutputFormat,
  [FileType.Email]: EmailOutputFormat,
  [FileType.TextMarkdown]: TextMarkdownOutputFormat,
  [FileType.Docx]: DocxOutputFormat,
  [FileType.PowerPoint]: PptOutputFormat,
  [FileType.Video]: VideoOutputFormat,
  [FileType.Audio]: AudioOutputFormat,
};

export const InitialOutputFormatMap = {
  [FileType.PDF]: PdfOutputFormat.Json,
  [FileType.Spreadsheet]: SpreadsheetOutputFormat.Html,
  [FileType.Image]: ImageOutputFormat.Text,
  [FileType.Email]: EmailOutputFormat.Text,
  [FileType.TextMarkdown]: TextMarkdownOutputFormat.Text,
  [FileType.Docx]: DocxOutputFormat.Json,
  [FileType.PowerPoint]: PptOutputFormat.Json,
  [FileType.Video]: VideoOutputFormat.Json,
  [FileType.Audio]: AudioOutputFormat.Text,
};
