import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import {
  initialLlmBaseValues,
  DataflowOperator as Operator,
} from '@/constants/agent';

export enum FileType {
  PDF = 'pdf',
  Spreadsheet = 'spreadsheet',
  Image = 'image',
  Email = 'email',
  TextMarkdown = 'text&markdown',
  Docx = 'word',
  PowerPoint = 'slides',
  Video = 'video',
  Audio = 'audio',
}

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
  Text = 'text',
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
  [FileType.Video]: VideoOutputFormat.Text,
  [FileType.Audio]: AudioOutputFormat.Text,
};

export enum ContextGeneratorFieldName {
  Summary = 'summary',
  Keywords = 'keywords',
  Questions = 'questions',
  Metadata = 'metadata',
}

export const FileId = 'File'; // BeginId

export enum TokenizerSearchMethod {
  Embedding = 'embedding',
  FullText = 'full_text',
}

export enum ImageParseMethod {
  OCR = 'ocr',
}

export enum TokenizerFields {
  Text = 'text',
  Questions = 'questions',
  Summary = 'summary',
}

export enum ParserFields {
  From = 'from',
  To = 'to',
  Cc = 'cc',
  Bcc = 'bcc',
  Date = 'date',
  Subject = 'subject',
  Body = 'body',
  Attachments = 'attachments',
}

// initialBeginValues
export const initialFileValues = {
  outputs: {
    name: {
      type: 'string',
      value: '',
    },
    file: {
      type: 'Object',
      value: {},
    },
  },
};

export const initialTokenizerValues = {
  search_method: [
    TokenizerSearchMethod.Embedding,
    TokenizerSearchMethod.FullText,
  ],
  filename_embd_weight: 0.1,
  fields: TokenizerFields.Text,
  outputs: {},
};

export enum StringTransformMethod {
  Merge = 'merge',
  Split = 'split',
}

export enum StringTransformDelimiter {
  Comma = ',',
  Semicolon = ';',
  Period = '.',
  LineBreak = '\n',
  Tab = '\t',
  Space = ' ',
}

export const initialParserValues = {
  outputs: {
    markdown: { type: 'string', value: '' },
    text: { type: 'string', value: '' },
    html: { type: 'string', value: '' },
    json: { type: 'Array<object>', value: [] },
  },
  setups: [
    {
      fileFormat: FileType.PDF,
      output_format: PdfOutputFormat.Json,
      parse_method: ParseDocumentType.DeepDOC,
    },
    {
      fileFormat: FileType.Spreadsheet,
      output_format: SpreadsheetOutputFormat.Html,
    },
    {
      fileFormat: FileType.Image,
      output_format: ImageOutputFormat.Text,
      parse_method: ImageParseMethod.OCR,
      system_prompt: '',
    },
    {
      fileFormat: FileType.Email,
      fields: Object.values(ParserFields),
      output_format: EmailOutputFormat.Text,
    },
    {
      fileFormat: FileType.TextMarkdown,
      output_format: TextMarkdownOutputFormat.Text,
    },
    {
      fileFormat: FileType.Docx,
      output_format: DocxOutputFormat.Json,
    },
    {
      fileFormat: FileType.PowerPoint,
      output_format: PptOutputFormat.Json,
    },
  ],
};

export const initialSplitterValues = {
  outputs: {
    chunks: { type: 'Array<Object>', value: [] },
  },
  chunk_token_size: 512,
  overlapped_percent: 0,
  delimiters: [{ value: '\n' }],
};

export enum Hierarchy {
  H1 = '1',
  H2 = '2',
  H3 = '3',
  H4 = '4',
  H5 = '5',
}

export const initialHierarchicalMergerValues = {
  outputs: {
    chunks: { type: 'Array<Object>', value: [] },
  },
  hierarchy: Hierarchy.H3,
  levels: [
    { expressions: [{ expression: '^#[^#]' }] },
    { expressions: [{ expression: '^##[^#]' }] },
    { expressions: [{ expression: '^###[^#]' }] },
    { expressions: [{ expression: '^####[^#]' }] },
  ],
};

export const initialExtractorValues = {
  ...initialLlmBaseValues,
  field_name: ContextGeneratorFieldName.Summary,
  outputs: {
    chunks: { type: 'Array<Object>', value: [] },
  },
};

export const NoDebugOperatorsList = [Operator.Begin];

export const FileTypeSuffixMap = {
  [FileType.PDF]: ['pdf'],
  [FileType.Spreadsheet]: ['xls', 'xlsx', 'csv'],
  [FileType.Image]: ['jpg', 'jpeg', 'png', 'gif'],
  [FileType.Email]: ['eml', 'msg'],
  [FileType.TextMarkdown]: ['md', 'markdown', 'mdx', 'txt'],
  [FileType.Docx]: ['doc', 'docx'],
  [FileType.PowerPoint]: ['pptx'],
  [FileType.Video]: ['mp4', 'avi', 'mkv'],
  [FileType.Audio]: [
    'da',
    'wave',
    'wav',
    'mp3',
    'aac',
    'flac',
    'ogg',
    'aiff',
    'au',
    'midi',
    'wma',
    'realaudio',
    'vqf',
    'oggvorbis',
    'ape',
  ],
};

export const SingleOperators = [
  Operator.Tokenizer,
  Operator.Splitter,
  Operator.HierarchicalMerger,
  Operator.Parser,
];
