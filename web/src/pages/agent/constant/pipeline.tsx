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
  TextMarkdown = 'markdown',
  Code = 'text&code',
  Html = 'html',
  Doc = 'doc',
  Docx = 'docx',
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
  Text = 'json',
}

export enum TextJsonOutputFormat {
  Text = 'text',
  Json = 'json',
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
  [FileType.Code]: TextJsonOutputFormat,
  [FileType.Html]: TextJsonOutputFormat,
  [FileType.Doc]: DocxOutputFormat,
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
  [FileType.Code]: TextJsonOutputFormat.Json,
  [FileType.Html]: TextJsonOutputFormat.Json,
  [FileType.Doc]: DocxOutputFormat.Json,
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
  TableOfContents = 'toc',
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

export enum PreprocessValue {
  main_content = 'main_content',
  section_title = 'title',
  abstract = 'abstract',
  author = 'author',
}

export const MAIN_CONTENT_PREPROCESS_VALUE: PreprocessValue =
  PreprocessValue.main_content;

export const PreprocessLabelKeyMap: Record<PreprocessValue, string> = {
  main_content: 'mainContent',
  title: 'sectionTitle',
  abstract: 'abstract',
  author: 'author',
};
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
      preprocess: PreprocessValue.main_content,
      flatten_media_to_text: false,
      remove_header_footer: false,
    },
    {
      fileFormat: FileType.Spreadsheet,
      output_format: SpreadsheetOutputFormat.Html,
      parse_method: ParseDocumentType.DeepDOC,
      preprocess: PreprocessValue.main_content,
      flatten_media_to_text: false,
    },
    {
      fileFormat: FileType.Image,
      output_format: ImageOutputFormat.Text,
      parse_method: ImageParseMethod.OCR,
      preprocess: PreprocessValue.main_content,
      system_prompt: '',
    },
    {
      fileFormat: FileType.Email,
      fields: Object.values(ParserFields),
      output_format: EmailOutputFormat.Text,
      preprocess: PreprocessValue.main_content,
    },
    {
      fileFormat: FileType.TextMarkdown,
      output_format: TextMarkdownOutputFormat.Text,
      preprocess: PreprocessValue.main_content,
      flatten_media_to_text: false,
    },
    {
      fileFormat: FileType.Code,
      output_format: TextJsonOutputFormat.Json,
      preprocess: PreprocessValue.main_content,
    },
    {
      fileFormat: FileType.Html,
      output_format: TextJsonOutputFormat.Json,
      preprocess: PreprocessValue.main_content,
      remove_header_footer: false,
    },
    {
      fileFormat: FileType.Doc,
      output_format: DocxOutputFormat.Json,
      preprocess: PreprocessValue.main_content,
    },
    {
      fileFormat: FileType.Docx,
      output_format: DocxOutputFormat.Json,
      preprocess: PreprocessValue.main_content,
      flatten_media_to_text: false,
      remove_header_footer: false,
    },
    {
      fileFormat: FileType.PowerPoint,
      output_format: PptOutputFormat.Json,
      parse_method: ParseDocumentType.DeepDOC,
      preprocess: PreprocessValue.main_content,
    },
  ],
};

export const initialTokenChunkerValues = {
  outputs: {
    chunks: { type: 'Array<Object>', value: [] },
  },
  delimiter_mode: 'token_size',
  chunk_token_size: 512,
  overlapped_percent: 0,
  delimiters: [{ value: '\n' }],
  image_table_context_window: 0,
};

export enum Hierarchy {
  H1 = '1',
  H2 = '2',
  H3 = '3',
  H4 = '4',
  H5 = '5',
}
const rules = [
  {
    // levels: [
    //   { expression: '^#[^#]' },
    //   { expression: '^##[^#]' },
    //   { expression: '^###[^#]' },
    //   { expression: '^####[^#]' },
    // ],
    levels: [
      { expression: '^#[^#]' },
      { expression: '^##[^#]' },
      { expression: '^###[^#]' },
      { expression: '^####[^#]' },
    ],
  },
  {
    levels: [
      { expression: '第[零一二三四五六七八九十百0-9]+(分?编|部分)' },
      { expression: '第[零一二三四五六七八九十百0-9]+章' },
      { expression: '第[零一二三四五六七八九十百0-9]+节' },
      { expression: '第[零一二三四五六七八九十百0-9]+条' },
      { expression: '[\\(（][零一二三四五六七八九十百]+[\\)）]' },
    ],
  },
  {
    levels: [
      { expression: '第[0-9]+章' },
      { expression: '第[0-9]+节' },
      { expression: '[0-9]{1,2}[\\. 、]' },
      { expression: '[0-9]{1,2}\\.[0-9]{1,2}($|[^a-zA-Z/%~.-])' },
      { expression: '[0-9]{1,2}\\.[0-9]{1,2}\\.[0-9]{1,2}' },
    ],
  },
  {
    levels: [
      { expression: '第[零一二三四五六七八九十百0-9]+章' },
      { expression: '第[零一二三四五六七八九十百0-9]+节' },
      { expression: '[零一二三四五六七八九十百]+[ 、]' },
      { expression: '[\\(（][零一二三四五六七八九十百]+[\\)）]' },
      { expression: '[\\(（][0-9]{,2}[\\)）]' },
    ],
  },
  {
    levels: [
      {
        expression: 'PART (ONE|TWO|THREE|FOUR|FIVE|SIX|SEVEN|EIGHT|NINE|TEN)',
      },
      { expression: 'Chapter (I+V?|VI*|XI|IX|X)' },
      { expression: 'Section [0-9]+' },
      { expression: 'Article [0-9]+' },
    ],
  },
];
export const initialTitleChunkerValues = {
  outputs: {
    chunks: { type: 'Array<Object>', value: [] },
  },
  method: 'hierarchy',
  hierarchy: Hierarchy.H3,
  include_heading_content: false,
  root_chunk_as_heading: false,
  rules: rules,
};

export const initialGroupValues = {
  method: 'group',
  hierarchy: '0',
  include_heading_content: false,
  root_chunk_as_heading: false,
  rules: rules,
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
  [FileType.TextMarkdown]: ['md', 'markdown', 'mdx'],
  [FileType.Code]: [
    'txt',
    'py',
    'js',
    'java',
    'c',
    'cpp',
    'h',
    'php',
    'go',
    'ts',
    'sh',
    'cs',
    'kt',
    'sql',
  ],
  [FileType.Html]: ['htm', 'html'],
  [FileType.Doc]: ['doc'],
  [FileType.Docx]: ['docx'],
  [FileType.PowerPoint]: ['pptx', 'ppt'],
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
  Operator.TokenChunker,
  Operator.TitleChunker,
  Operator.Parser,
];
