import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import { initialLlmBaseValues } from '@/constants/agent';
import {
  ChatVariableEnabledField,
  variableEnabledFieldMap,
} from '@/constants/chat';
import { setInitialChatVariableEnabledFieldValue } from '@/utils/chat';

import {
  Circle,
  CircleSlash2,
  CloudUpload,
  ListOrdered,
  OptionIcon,
  TextCursorInput,
  ToggleLeft,
  WrapText,
} from 'lucide-react';

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

export enum ContextGeneratorFieldName {
  Summary = 'summary',
  Keywords = 'keywords',
  Questions = 'questions',
  Metadata = 'metadata',
}

export enum PromptRole {
  User = 'user',
  Assistant = 'assistant',
}

export enum AgentDialogueMode {
  Conversational = 'conversational',
  Task = 'task',
}

export const BeginId = 'File';

export enum Operator {
  Begin = 'File',
  Note = 'Note',
  Parser = 'Parser',
  Tokenizer = 'Tokenizer',
  Splitter = 'Splitter',
  HierarchicalMerger = 'HierarchicalMerger',
  Extractor = 'Extractor',
}

export const SwitchLogicOperatorOptions = ['and', 'or'];

export const CommonOperatorList = Object.values(Operator).filter(
  (x) => x !== Operator.Note,
);

export const SwitchOperatorOptions = [
  { value: '=', label: 'equal', icon: 'equal' },
  { value: '≠', label: 'notEqual', icon: 'not-equals' },
  { value: '>', label: 'gt', icon: 'Less' },
  { value: '≥', label: 'ge', icon: 'Greater-or-equal' },
  { value: '<', label: 'lt', icon: 'Less' },
  { value: '≤', label: 'le', icon: 'less-or-equal' },
  { value: 'contains', label: 'contains', icon: 'Contains' },
  { value: 'not contains', label: 'notContains', icon: 'not-contains' },
  { value: 'start with', label: 'startWith', icon: 'list-start' },
  { value: 'end with', label: 'endWith', icon: 'list-end' },
  {
    value: 'empty',
    label: 'empty',
    icon: <Circle className="size-4" />,
  },
  {
    value: 'not empty',
    label: 'notEmpty',
    icon: <CircleSlash2 className="size-4" />,
  },
];

export const SwitchElseTo = 'end_cpn_ids';

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

export const initialBeginValues = {
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

export const variableCheckBoxFieldMap = Object.keys(
  variableEnabledFieldMap,
).reduce<Record<string, boolean>>((pre, cur) => {
  pre[cur] = setInitialChatVariableEnabledFieldValue(
    cur as ChatVariableEnabledField,
  );
  return pre;
}, {});

export const initialNoteValues = {
  text: '',
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
  outputs: {},
};

export const CategorizeAnchorPointPositions = [
  { top: 1, right: 34 },
  { top: 8, right: 18 },
  { top: 15, right: 10 },
  { top: 24, right: 4 },
  { top: 31, right: 1 },
  { top: 38, right: -2 },
  { top: 62, right: -2 }, //bottom
  { top: 71, right: 1 },
  { top: 79, right: 6 },
  { top: 86, right: 12 },
  { top: 91, right: 20 },
  { top: 98, right: 34 },
];

// key is the source of the edge, value is the target of the edge
// no connection lines are allowed between key and value
export const RestrictedUpstreamMap = {
  [Operator.Begin]: [],
  [Operator.Parser]: [Operator.Begin],
  [Operator.Splitter]: [Operator.Begin],
  [Operator.HierarchicalMerger]: [Operator.Begin],
  [Operator.Tokenizer]: [Operator.Begin],
  [Operator.Extractor]: [Operator.Begin],
};

export const NodeMap = {
  [Operator.Begin]: 'beginNode',
  [Operator.Note]: 'noteNode',
  [Operator.Parser]: 'parserNode',
  [Operator.Tokenizer]: 'tokenizerNode',
  [Operator.Splitter]: 'splitterNode',
  [Operator.HierarchicalMerger]: 'hierarchicalMergerNode',
  [Operator.Extractor]: 'contextNode',
};

export enum BeginQueryType {
  Line = 'line',
  Paragraph = 'paragraph',
  Options = 'options',
  File = 'file',
  Integer = 'integer',
  Boolean = 'boolean',
}

export const BeginQueryTypeIconMap = {
  [BeginQueryType.Line]: TextCursorInput,
  [BeginQueryType.Paragraph]: WrapText,
  [BeginQueryType.Options]: OptionIcon,
  [BeginQueryType.File]: CloudUpload,
  [BeginQueryType.Integer]: ListOrdered,
  [BeginQueryType.Boolean]: ToggleLeft,
};

export const NoDebugOperatorsList = [Operator.Begin];

export enum NodeHandleId {
  Start = 'start',
  End = 'end',
  Tool = 'tool',
  AgentTop = 'agentTop',
  AgentBottom = 'agentBottom',
  AgentException = 'agentException',
}

export enum VariableType {
  String = 'string',
  Array = 'array',
  File = 'file',
}

export enum AgentExceptionMethod {
  Comment = 'comment',
  Goto = 'goto',
}

export const FileTypeSuffixMap = {
  [FileType.PDF]: ['pdf'],
  [FileType.Spreadsheet]: ['xls', 'xlsx', 'csv'],
  [FileType.Image]: ['jpg', 'jpeg', 'png', 'gif'],
  [FileType.Email]: ['eml', 'msg'],
  [FileType.TextMarkdown]: ['md', 'markdown', 'mdx', 'txt'],
  [FileType.Docx]: ['doc', 'docx'],
  [FileType.PowerPoint]: ['pptx'],
  [FileType.Video]: [],
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
