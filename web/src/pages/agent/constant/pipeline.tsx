import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import { initialLlmBaseValues, Operator } from '@/constants/agent';
import { ModelTypeToField } from '@/constants/llm';
import { pickByBackend } from '@/utils/backend-variant';
import { cloneDeep } from 'lodash';

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
  Json = 'json',
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

// The video parser defaults to the tenant's VLM model and the audio parser to
// the ASR model, keyed by the useFetchDefaultModelDictionary fields. A file
// type without a configured tenant default keeps its empty model id.
export const FileTypeDefaultModelFieldMap: Partial<Record<FileType, string>> = {
  [FileType.Video]: ModelTypeToField.vision,
  [FileType.Audio]: ModelTypeToField.asr,
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
      pages: [{ from: 1, to: 100000 }],
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
      output_format: ImageOutputFormat.Json,
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
      flatten_media_to_text: false,
      remove_header_footer: false,
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
    {
      fileFormat: FileType.Video,
      output_format: VideoOutputFormat.Text,
      vlm: { llm_id: '' },
    },
    {
      fileFormat: FileType.Audio,
      output_format: AudioOutputFormat.Text,
      vlm: { llm_id: '' },
    },
  ],
};

export const initialTokenChunkerValues = {
  outputs: {
    chunks: { type: 'Array<Object>', value: [] },
  },
  delimiter_mode: 'delimiter',
  chunk_token_size: 512,
  overlapped_percent: 0,
  delimiters: [{ value: '\n' }],
  image_table_context_window: 0,
  enable_children: false,
  children_delimiters: [],
};

export enum Hierarchy {
  H1 = '1',
  H2 = '2',
  H3 = '3',
  H4 = '4',
  H5 = '5',
}

export enum TitleChunkerMethod {
  Hierarchy = 'hierarchy',
  Group = 'group',
}
export const originalRules = [
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
  method: TitleChunkerMethod.Hierarchy,
  hierarchyHierarchy: Hierarchy.H3,
  hierarchyGroup: '0',
  include_heading_content: false,
  root_chunk_as_heading: false,
  chunk_token_cap: 512,
  hierarchyRules: cloneDeep(originalRules),
  groupRules: cloneDeep(originalRules),
};

// Defaults for the Python backend extractor (legacy flat fields).
export const initialExtractorValues = {
  ...initialLlmBaseValues,
  field_name: ContextGeneratorFieldName.Summary,
  auto_tags: 1,
  tag_file_id: '',
  outputs: {
    chunks: { type: 'Array<Object>', value: [] },
  },
};

// Defaults for the Go backend extractor: the LLM settings plus the nested
// per-feature groups the Go schema reads (schema.ExtractorParam).
export const initialGoExtractorValues = {
  ...initialLlmBaseValues,
  keywords: {
    top_n: 0,
    system_prompt: '',
  },
  questions: {
    top_n: 0,
    system_prompt: '',
  },
  tags: {
    top_n: 0,
    tag_file_id: '',
  },
  summary: {
    enabled: false,
    system_prompt: '',
  },
  metadata: {
    enabled: false,
    metadata: [],
    built_in_metadata: [],
  },
  outputs: {
    chunks: { type: 'Array<Object>', value: [] },
  },
};

export function getInitialExtractorValues() {
  return pickByBackend<
    typeof initialGoExtractorValues | typeof initialExtractorValues
  >({
    go: initialGoExtractorValues,
    python: initialExtractorValues,
  });
}

export const initialCompilationValues = {
  compilation_template_group_id: '',
  llm_id: '',
  outputs: {
    chunks: { type: 'Array<Object>', value: [] },
  },
};

export const NoDebugOperatorsList = [Operator.File];

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
