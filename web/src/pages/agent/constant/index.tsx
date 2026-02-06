import {
  initialKeywordsSimilarityWeightValue,
  initialSimilarityThresholdValue,
} from '@/components/similarity-slider';
import {
  AgentGlobals,
  AgentGlobalsSysQueryWithBrace,
  CodeTemplateStrMap,
  ComparisonOperator,
  JsonSchemaDataType,
  Operator,
  ProgrammingLanguage,
  SwitchLogicOperator,
  SwitchOperatorOptions,
  initialLlmBaseValues,
} from '@/constants/agent';
export {
  AgentDialogueMode,
  AgentStructuredOutputField,
  JsonSchemaDataType,
  Operator,
  initialBeginValues,
} from '@/constants/agent';

export * from './pipeline';

import { ModelVariableType } from '@/constants/knowledge';
import { t } from 'i18next';

// DuckDuckGo's channel options
export enum Channel {
  Text = 'text',
  News = 'news',
}

export enum PromptRole {
  User = 'user',
  Assistant = 'assistant',
}

import {
  CloudUpload,
  ListOrdered,
  OptionIcon,
  TextCursorInput,
  ToggleLeft,
  WrapText,
} from 'lucide-react';

export const BeginId = 'begin';

export const CommonOperatorList = Object.values(Operator).filter(
  (x) => x !== Operator.Note,
);

export const AgentOperatorList = [
  Operator.Retrieval,
  Operator.Categorize,
  Operator.Message,
  Operator.RewriteQuestion,
  Operator.Switch,
  Operator.Iteration,
  Operator.WaitingDialogue,
  Operator.Note,
  Operator.Agent,
];

export const DataOperationsOperatorOptions = [
  ComparisonOperator.Equal,
  ComparisonOperator.NotEqual,
  ComparisonOperator.Contains,
  ComparisonOperator.StartWith,
  ComparisonOperator.EndWith,
];

export const SwitchElseTo = 'end_cpn_ids';

export enum RetrievalFrom {
  Dataset = 'dataset',
  Memory = 'memory',
}

export const initialRetrievalValues = {
  query: AgentGlobalsSysQueryWithBrace,
  top_n: 8,
  top_k: 1024,
  kb_ids: [],
  rerank_id: '',
  empty_response: '',
  ...initialSimilarityThresholdValue,
  ...initialKeywordsSimilarityWeightValue,
  use_kg: false,
  toc_enhance: false,
  cross_languages: [],
  retrieval_from: RetrievalFrom.Dataset,
  outputs: {
    formalized_content: {
      type: 'string',
      value: '',
    },
    json: {
      type: 'Array<Object>',
      value: [],
    },
  },
};

export const initialRewriteQuestionValues = {
  ...initialLlmBaseValues,
  language: '',
  message_history_window_size: 6,
};

export const initialRelevantValues = {
  ...initialLlmBaseValues,
};

export const initialCategorizeValues = {
  ...initialLlmBaseValues,
  query: AgentGlobals.SysQuery,
  parameter: ModelVariableType.Precise,
  message_history_window_size: 1,
  items: [],
  outputs: {
    category_name: {
      type: 'string',
    },
  },
};

export const initialMessageValues = {
  content: [''],
};

export const initialExcelProcessorValues = {
  input_files: [],
  operation: 'read',
  sheet_selection: 'all',
  merge_strategy: 'concat',
  join_on: '',
  transform_data: '',
  output_format: 'xlsx',
  output_filename: 'output',
  outputs: {
    data: {
      type: 'object',
      value: {},
    },
    summary: {
      type: 'string',
      value: '',
    },
    markdown: {
      type: 'string',
      value: '',
    },
  },
};

export const initialDuckValues = {
  top_n: 10,
  channel: Channel.Text,
  query: AgentGlobals.SysQuery,
  outputs: {
    formalized_content: {
      value: '',
      type: 'string',
    },
    json: {
      value: [],
      type: 'Array<Object>',
    },
  },
};

export const initialSearXNGValues = {
  top_n: '10',
  searxng_url: '',
  query: AgentGlobals.SysQuery,
  outputs: {
    formalized_content: {
      value: '',
      type: 'string',
    },
    json: {
      value: [],
      type: 'Array<Object>',
    },
  },
};

export const initialWikipediaValues = {
  top_n: 10,
  language: 'en',
  query: AgentGlobals.SysQuery,
  outputs: {
    formalized_content: {
      value: '',
      type: 'string',
    },
  },
};

export const initialPubMedValues = {
  top_n: 12,
  email: '',
  query: AgentGlobals.SysQuery,
  outputs: {
    formalized_content: {
      value: '',
      type: 'string',
    },
  },
};

export const initialArXivValues = {
  top_n: 12,
  sort_by: 'relevance',
  query: AgentGlobals.SysQuery,
  outputs: {
    formalized_content: {
      value: '',
      type: 'string',
    },
  },
};

export const initialGoogleValues = {
  q: AgentGlobals.SysQuery,
  start: 0,
  num: 12,
  api_key: '',
  country: 'us',
  language: 'en',
  outputs: {
    formalized_content: {
      value: '',
      type: 'string',
    },
    json: {
      value: [],
      type: 'Array<Object>',
    },
  },
};

export const initialBingValues = {
  top_n: 10,
  channel: 'Webpages',
  api_key:
    'YOUR_API_KEY (obtained from https://www.microsoft.com/en-us/bing/apis/bing-web-search-api)',
  country: 'CH',
  language: 'en',
  query: '',
};

export const initialGoogleScholarValues = {
  top_n: 12,
  sort_by: 'relevance',
  patents: true,
  query: AgentGlobals.SysQuery,
  year_low: undefined,
  year_high: undefined,
  outputs: {
    formalized_content: {
      value: '',
      type: 'string',
    },
    json: {
      value: [],
      type: 'Array<Object>',
    },
  },
};

export const initialGithubValues = {
  top_n: 5,
  query: AgentGlobals.SysQuery,
  outputs: {
    formalized_content: {
      value: '',
      type: 'string',
    },
    json: {
      value: [],
      type: 'Array<Object>',
    },
  },
};

export const initialExeSqlValues = {
  sql: '',
  db_type: 'mysql',
  database: '',
  username: '',
  host: '',
  port: 3306,
  password: '',
  max_records: 1024,
  outputs: {
    formalized_content: {
      value: '',
      type: 'string',
    },
    json: {
      value: [],
      type: 'Array<Object>',
    },
  },
};

export const initialSwitchValues = {
  conditions: [
    {
      logical_operator: SwitchLogicOperator.And,
      items: [
        {
          operator: SwitchOperatorOptions[0].value,
        },
      ],
      to: [],
    },
  ],
  [SwitchElseTo]: [],
};

export const initialWenCaiValues = {
  top_n: 20,
  query_type: 'stock',
  query: AgentGlobals.SysQuery,
  outputs: {
    report: {
      value: '',
      type: 'string',
    },
  },
};

export const initialYahooFinanceValues = {
  stock_code: '',
  info: true,
  history: false,
  financials: false,
  balance_sheet: false,
  cash_flow_statement: false,
  news: true,
  outputs: {
    report: {
      value: '',
      type: 'string',
    },
  },
};

export const initialNoteValues = {
  text: '',
};

export const initialCrawlerValues = {
  extract_type: 'markdown',
  query: '',
};

export const initialInvokeValues = {
  url: '',
  method: 'GET',
  timeout: 60,
  headers: `{
  "Accept": "*/*",
  "Cache-Control": "no-cache",
  "Connection": "keep-alive"
}`,
  proxy: '',
  clean_html: false,
  variables: [],
  outputs: {
    result: {
      value: '',
      type: 'string',
    },
  },
};

export const initialTemplateValues = {
  content: '',
  parameters: [],
};

export const initialEmailValues = {
  smtp_server: '',
  smtp_port: 465,
  email: '',
  password: '',
  sender_name: '',
  to_email: '',
  cc_email: '',
  subject: '',
  content: '',
  outputs: {
    success: {
      value: true,
      type: 'boolean',
    },
  },
};

export const initialIterationValues = {
  items_ref: '',
  outputs: {},
};

export const initialIterationStartValues = {
  outputs: {
    item: {
      type: 'unkown',
    },
    index: {
      type: 'integer',
    },
  },
};

export const initialCodeValues = {
  lang: ProgrammingLanguage.Python,
  script: CodeTemplateStrMap[ProgrammingLanguage.Python],
  arguments: {
    arg1: '',
    arg2: '',
  },
  outputs: {},
};

export const initialWaitingDialogueValues = {};

export const initialAgentValues = {
  ...initialLlmBaseValues,
  description: '',
  user_prompt: '',
  sys_prompt: t('flow.sysPromptDefaultValue'),
  prompts: [{ role: PromptRole.User, content: `{${AgentGlobals.SysQuery}}` }],
  message_history_window_size: 12,
  max_retries: 3,
  delay_after_error: 1,
  visual_files_var: '',
  max_rounds: 1,
  exception_method: '',
  exception_goto: [],
  exception_default_value: '',
  tools: [],
  mcp: [],
  cite: true,
  showStructuredOutput: false,
  outputs: {
    content: {
      type: 'string',
      value: '',
    },
    // [AgentStructuredOutputField]: {},
  },
};

export const initialUserFillUpValues = {
  enable_tips: true,
  tips: '',
  inputs: [],
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

export const initialStringTransformValues = {
  method: StringTransformMethod.Merge,
  split_ref: '',
  script: '',
  delimiters: [StringTransformDelimiter.Comma],
  outputs: {
    result: {
      type: 'string',
    },
  },
};

export enum TavilySearchDepth {
  Basic = 'basic',
  Advanced = 'advanced',
}

export enum TavilyTopic {
  News = 'news',
  General = 'general',
}

export const initialTavilyValues = {
  api_key: '',
  query: AgentGlobals.SysQuery,
  search_depth: TavilySearchDepth.Basic,
  topic: TavilyTopic.General,
  max_results: 5,
  days: 7,
  include_answer: false,
  include_raw_content: true,
  include_images: false,
  include_image_descriptions: false,
  include_domains: [],
  exclude_domains: [],
  outputs: {
    formalized_content: {
      value: '',
      type: 'string',
    },
    json: {
      value: [],
      type: 'Array<Object>',
    },
  },
};

export enum TavilyExtractDepth {
  Basic = 'basic',
  Advanced = 'advanced',
}

export enum TavilyExtractFormat {
  Text = 'text',
  Markdown = 'markdown',
}

export const initialTavilyExtractValues = {
  urls: '',
  extract_depth: TavilyExtractDepth.Basic,
  format: TavilyExtractFormat.Markdown,
  outputs: {
    formalized_content: {
      value: '',
      type: 'string',
    },
    json: {
      value: [],
      type: 'Array<Object>',
    },
  },
};

export const initialPlaceholderValues = {
  // Placeholder node doesn't need any specific form values
  // It's just a visual placeholder
};

export enum Operations {
  SelectKeys = 'select_keys',
  LiteralEval = 'literal_eval',
  Combine = 'combine',
  FilterValues = 'filter_values',
  AppendOrUpdate = 'append_or_update',
  RemoveKeys = 'remove_keys',
  RenameKeys = 'rename_keys',
}

export const initialDataOperationsValues = {
  query: [],
  operations: Operations.SelectKeys,
  outputs: {
    result: {
      type: 'Array<Object>',
    },
  },
};
export enum SortMethod {
  Asc = 'asc',
  Desc = 'desc',
}

export enum ListOperations {
  TopN = 'topN',
  Head = 'head',
  Tail = 'tail',
  Filter = 'filter',
  Sort = 'sort',
  DropDuplicates = 'drop_duplicates',
}

export const initialListOperationsValues = {
  query: '',
  operations: ListOperations.TopN,
  outputs: {
    // result: {
    //   type: 'Array<?>',
    // },
    // first: {
    //   type: '?',
    // },
    // last: {
    //   type: '?',
    // },
  },
};

export const initialVariableAssignerValues = {};

export const initialVariableAggregatorValues = { outputs: {}, groups: [] };

export const initialLoopValues = {
  loop_variables: [],
  loop_termination_condition: [],
  maximum_loop_count: 10,
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
  [Operator.Begin]: [Operator.Begin],
  [Operator.Categorize]: [Operator.Begin],
  [Operator.Retrieval]: [Operator.Begin, Operator.Retrieval],
  [Operator.Message]: [
    Operator.Begin,
    Operator.Message,
    Operator.Retrieval,
    Operator.RewriteQuestion,
    Operator.Categorize,
  ],
  [Operator.RewriteQuestion]: [
    Operator.Begin,
    Operator.Message,
    Operator.RewriteQuestion,
  ],
  [Operator.DuckDuckGo]: [Operator.Begin, Operator.Retrieval],
  [Operator.Wikipedia]: [Operator.Begin, Operator.Retrieval],
  [Operator.PubMed]: [Operator.Begin, Operator.Retrieval],
  [Operator.ArXiv]: [Operator.Begin, Operator.Retrieval],
  [Operator.Google]: [Operator.Begin, Operator.Retrieval],
  [Operator.Bing]: [Operator.Begin, Operator.Retrieval],
  [Operator.GoogleScholar]: [Operator.Begin, Operator.Retrieval],
  [Operator.GitHub]: [Operator.Begin, Operator.Retrieval],
  [Operator.SearXNG]: [Operator.Begin, Operator.Retrieval],
  [Operator.ExeSQL]: [Operator.Begin],
  [Operator.Switch]: [Operator.Begin],
  [Operator.WenCai]: [Operator.Begin],
  [Operator.YahooFinance]: [Operator.Begin],
  [Operator.Crawler]: [Operator.Begin],
  [Operator.Note]: [],
  [Operator.Invoke]: [Operator.Begin],
  [Operator.Email]: [Operator.Begin],
  [Operator.Iteration]: [Operator.Begin],
  [Operator.IterationStart]: [Operator.Begin],
  [Operator.Code]: [Operator.Begin],
  [Operator.WaitingDialogue]: [Operator.Begin],
  [Operator.Agent]: [Operator.Begin],
  [Operator.TavilySearch]: [Operator.Begin],
  [Operator.TavilyExtract]: [Operator.Begin],
  [Operator.StringTransform]: [Operator.Begin],
  [Operator.UserFillUp]: [Operator.Begin],
  [Operator.Tool]: [Operator.Begin],
  [Operator.Placeholder]: [Operator.Begin],
  [Operator.DataOperations]: [Operator.Begin],
  [Operator.ListOperations]: [Operator.Begin],
  [Operator.VariableAssigner]: [Operator.Begin],
  [Operator.VariableAggregator]: [Operator.Begin],
  [Operator.Parser]: [Operator.Begin], // pipeline
  [Operator.Splitter]: [Operator.Begin],
  [Operator.HierarchicalMerger]: [Operator.Begin],
  [Operator.Tokenizer]: [Operator.Begin],
  [Operator.Extractor]: [Operator.Begin],
  [Operator.File]: [Operator.Begin],
  [Operator.Loop]: [Operator.Begin],
  [Operator.LoopStart]: [Operator.Begin],
  [Operator.ExitLoop]: [Operator.Begin],
};

export const NodeMap = {
  [Operator.Begin]: 'beginNode',
  [Operator.Categorize]: 'categorizeNode',
  [Operator.Retrieval]: 'retrievalNode',
  [Operator.Message]: 'messageNode',
  [Operator.RewriteQuestion]: 'rewriteNode',
  [Operator.DuckDuckGo]: 'ragNode',
  [Operator.Wikipedia]: 'ragNode',
  [Operator.PubMed]: 'ragNode',
  [Operator.ArXiv]: 'ragNode',
  [Operator.Google]: 'ragNode',
  [Operator.Bing]: 'ragNode',
  [Operator.GoogleScholar]: 'ragNode',
  [Operator.GitHub]: 'ragNode',
  [Operator.SearXNG]: 'ragNode',
  [Operator.ExeSQL]: 'ragNode',
  [Operator.Switch]: 'switchNode',
  [Operator.WenCai]: 'ragNode',
  [Operator.YahooFinance]: 'ragNode',
  [Operator.Note]: 'noteNode',
  [Operator.Crawler]: 'ragNode',
  [Operator.Invoke]: 'ragNode',
  [Operator.Email]: 'ragNode',
  [Operator.Iteration]: 'group',
  [Operator.IterationStart]: 'iterationStartNode',
  [Operator.Code]: 'ragNode',
  [Operator.WaitingDialogue]: 'ragNode',
  [Operator.Agent]: 'agentNode',
  [Operator.Tool]: 'toolNode',
  [Operator.TavilySearch]: 'ragNode',
  [Operator.UserFillUp]: 'ragNode',
  [Operator.StringTransform]: 'ragNode',
  [Operator.TavilyExtract]: 'ragNode',
  [Operator.Placeholder]: 'placeholderNode',
  [Operator.File]: 'fileNode',
  [Operator.Parser]: 'parserNode',
  [Operator.Tokenizer]: 'tokenizerNode',
  [Operator.Splitter]: 'splitterNode',
  [Operator.HierarchicalMerger]: 'splitterNode',
  [Operator.Extractor]: 'contextNode',
  [Operator.DataOperations]: 'dataOperationsNode',
  [Operator.ListOperations]: 'listOperationsNode',
  [Operator.VariableAssigner]: 'variableAssignerNode',
  [Operator.VariableAggregator]: 'variableAggregatorNode',
  [Operator.Loop]: 'loopNode',
  [Operator.LoopStart]: 'loopStartNode',
  [Operator.ExitLoop]: 'exitLoopNode',
  [Operator.ExcelProcessor]: 'ragNode',
  [Operator.PDFGenerator]: 'ragNode',
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

export const NoDebugOperatorsList = [
  Operator.Begin,
  Operator.Message,
  Operator.RewriteQuestion,
  Operator.Switch,
  Operator.Iteration,
  Operator.UserFillUp,
  Operator.IterationStart,
  Operator.File,
  Operator.Parser,
  Operator.Tokenizer,
  Operator.Splitter,
  Operator.HierarchicalMerger,
  Operator.Extractor,
  Operator.Tool,
];

export const NoCopyOperatorsList = [
  Operator.File,
  Operator.Parser,
  Operator.Tokenizer,
  Operator.Splitter,
  Operator.HierarchicalMerger,
  Operator.Extractor,
];

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

export const PLACEHOLDER_NODE_WIDTH = 200;
export const PLACEHOLDER_NODE_HEIGHT = 60;
export const DROPDOWN_SPACING = 25;
export const DROPDOWN_ADDITIONAL_OFFSET = 50;
export const HALF_PLACEHOLDER_NODE_WIDTH = PLACEHOLDER_NODE_WIDTH / 2;
export const HALF_PLACEHOLDER_NODE_HEIGHT =
  PLACEHOLDER_NODE_HEIGHT + DROPDOWN_SPACING + DROPDOWN_ADDITIONAL_OFFSET;
export const DROPDOWN_HORIZONTAL_OFFSET = 28;
export const DROPDOWN_VERTICAL_OFFSET = 74;
export const PREVENT_CLOSE_DELAY = 300;

export enum VariableAssignerLogicalOperator {
  Overwrite = 'overwrite',
  Clear = 'clear',
  Set = 'set',
}

export enum VariableAssignerLogicalNumberOperator {
  Overwrite = VariableAssignerLogicalOperator.Overwrite,
  Clear = VariableAssignerLogicalOperator.Clear,
  Set = VariableAssignerLogicalOperator.Set,
  Add = '+=',
  Subtract = '-=',
  Multiply = '*=',
  Divide = '/=',
}

export const VariableAssignerLogicalNumberOperatorLabelMap = {
  [VariableAssignerLogicalNumberOperator.Add]: 'add',
  [VariableAssignerLogicalNumberOperator.Subtract]: 'subtract',
  [VariableAssignerLogicalNumberOperator.Multiply]: 'multiply',
  [VariableAssignerLogicalNumberOperator.Divide]: 'divide',
};

export enum VariableAssignerLogicalArrayOperator {
  Overwrite = VariableAssignerLogicalOperator.Overwrite,
  Clear = VariableAssignerLogicalOperator.Clear,
  Append = 'append',
  Extend = 'extend',
  RemoveFirst = 'remove_first',
  RemoveLast = 'remove_last',
}

export enum ExportFileType {
  // PDF = 'pdf',
  HTML = 'html',
  Markdown = 'md',
  DOCX = 'docx',
  Excel = 'xlsx',
}

export enum TypesWithArray {
  String = 'string',
  Number = 'number',
  Boolean = 'boolean',
  Object = 'object',
  ArrayString = 'array<string>',
  ArrayNumber = 'array<number>',
  ArrayBoolean = 'array<boolean>',
  ArrayObject = 'array<object>',
}

export const ArrayFields = [
  JsonSchemaDataType.Array,
  TypesWithArray.ArrayBoolean,
  TypesWithArray.ArrayNumber,
  TypesWithArray.ArrayString,
  TypesWithArray.ArrayObject,
];

export enum InputMode {
  Constant = 'constant',
  Variable = 'variable',
}

export enum LoopTerminationComparisonOperator {
  Contains = ComparisonOperator.Contains,
  NotContains = ComparisonOperator.NotContains,
  StartWith = ComparisonOperator.StartWith,
  EndWith = ComparisonOperator.EndWith,
  Is = 'is',
  IsNot = 'is not',
}

export const LoopTerminationStringComparisonOperator = [
  LoopTerminationComparisonOperator.Contains,
  LoopTerminationComparisonOperator.NotContains,
  LoopTerminationComparisonOperator.StartWith,
  LoopTerminationComparisonOperator.EndWith,
  LoopTerminationComparisonOperator.Is,
  LoopTerminationComparisonOperator.IsNot,
  ComparisonOperator.Empty,
  ComparisonOperator.NotEmpty,
];

export const LoopTerminationBooleanComparisonOperator = [
  LoopTerminationComparisonOperator.Is,
  LoopTerminationComparisonOperator.IsNot,
  ComparisonOperator.Empty,
  ComparisonOperator.NotEmpty,
];
// object or object array
export const LoopTerminationObjectComparisonOperator = [
  ComparisonOperator.Empty,
  ComparisonOperator.NotEmpty,
];

// string array or number array
export const LoopTerminationStringArrayComparisonOperator = [
  LoopTerminationComparisonOperator.Contains,
  LoopTerminationComparisonOperator.NotContains,
  ComparisonOperator.Empty,
  ComparisonOperator.NotEmpty,
];

export const LoopTerminationBooleanArrayComparisonOperator = [
  LoopTerminationComparisonOperator.Is,
  LoopTerminationComparisonOperator.IsNot,
  ComparisonOperator.Empty,
  ComparisonOperator.NotEmpty,
];

export const LoopTerminationNumberComparisonOperator = [
  ComparisonOperator.Equal,
  ComparisonOperator.NotEqual,
  ComparisonOperator.GreatThan,
  ComparisonOperator.LessThan,
  ComparisonOperator.GreatEqual,
  ComparisonOperator.LessEqual,
  ComparisonOperator.Empty,
  ComparisonOperator.NotEmpty,
];

export const LoopTerminationStringComparisonOperatorMap = {
  [TypesWithArray.String]: LoopTerminationStringComparisonOperator,
  [TypesWithArray.Number]: LoopTerminationNumberComparisonOperator,
  [TypesWithArray.Boolean]: LoopTerminationBooleanComparisonOperator,
  [TypesWithArray.Object]: LoopTerminationObjectComparisonOperator,
  [TypesWithArray.ArrayString]: LoopTerminationStringArrayComparisonOperator,
  [TypesWithArray.ArrayNumber]: LoopTerminationStringArrayComparisonOperator,
  [TypesWithArray.ArrayBoolean]: LoopTerminationBooleanArrayComparisonOperator,
  [TypesWithArray.ArrayObject]: LoopTerminationObjectComparisonOperator,
};

export enum AgentVariableType {
  Begin = 'begin',
  Conversation = 'conversation',
}

// PDF Generator enums
export enum PDFGeneratorFontFamily {
  Helvetica = 'Helvetica',
  TimesRoman = 'Times-Roman',
  Courier = 'Courier',
  HelveticaBold = 'Helvetica-Bold',
  TimesBold = 'Times-Bold',
}

export enum PDFGeneratorLogoPosition {
  Left = 'left',
  Center = 'center',
  Right = 'right',
}

export enum PDFGeneratorPageSize {
  A4 = 'A4',
  Letter = 'Letter',
}

export enum PDFGeneratorOrientation {
  Portrait = 'portrait',
  Landscape = 'landscape',
}

export const initialPDFGeneratorValues = {
  output_format: 'pdf',
  content: '',
  title: '',
  subtitle: '',
  header_text: '',
  footer_text: '',
  logo_image: '',
  logo_position: PDFGeneratorLogoPosition.Left,
  logo_width: 2.0,
  logo_height: 1.0,
  font_family: PDFGeneratorFontFamily.Helvetica,
  font_size: 12,
  title_font_size: 24,
  heading1_font_size: 18,
  heading2_font_size: 16,
  heading3_font_size: 14,
  text_color: '#000000',
  title_color: '#000000',
  page_size: PDFGeneratorPageSize.A4,
  orientation: PDFGeneratorOrientation.Portrait,
  margin_top: 1.0,
  margin_bottom: 1.0,
  margin_left: 1.0,
  margin_right: 1.0,
  line_spacing: 1.2,
  filename: '',
  output_directory: '/tmp/pdf_outputs',
  add_page_numbers: true,
  add_timestamp: true,
  watermark_text: '',
  enable_toc: false,
  outputs: {
    file_path: { type: 'string' },
    pdf_base64: { type: 'string' },
    download: { type: 'string' },
    success: { type: 'boolean' },
  },
};

export enum WebhookMethod {
  Post = 'POST',
  Get = 'GET',
  Put = 'PUT',
  Patch = 'PATCH',
  Delete = 'DELETE',
  Head = 'HEAD',
}

export enum WebhookContentType {
  ApplicationJson = 'application/json',
  MultipartFormData = 'multipart/form-data',
  ApplicationXWwwFormUrlencoded = 'application/x-www-form-urlencoded',
  TextPlain = 'text/plain',
  ApplicationOctetStream = 'application/octet-stream',
}

export enum WebhookExecutionMode {
  Immediately = 'Immediately',
  Streaming = 'Streaming',
}

export enum WebhookSecurityAuthType {
  None = 'none',
  Token = 'token',
  Basic = 'basic',
  Jwt = 'jwt',
}

export enum WebhookRateLimitPer {
  Second = 'second',
  Minute = 'minute',
  Hour = 'hour',
  Day = 'day',
}

export const RateLimitPerList = Object.values(WebhookRateLimitPer);

export const WebhookMaxBodySize = ['1MB', '5MB', '10MB'];

export enum WebhookRequestParameters {
  File = VariableType.File,
  String = TypesWithArray.String,
  Number = TypesWithArray.Number,
  Boolean = TypesWithArray.Boolean,
}

export enum WebhookStatus {
  Testing = 'testing',
  Live = 'live',
  Stopped = 'stopped',
}

// Map BeginQueryType to TypesWithArray
export const BeginQueryTypeMap = {
  [BeginQueryType.Line]: TypesWithArray.String,
  [BeginQueryType.Paragraph]: TypesWithArray.String,
  [BeginQueryType.Options]: TypesWithArray.ArrayString,
  [BeginQueryType.File]: 'File',
  [BeginQueryType.Integer]: TypesWithArray.Number,
  [BeginQueryType.Boolean]: TypesWithArray.Boolean,
};

export const VariableRegex = /{([^{}]*)}/g;
