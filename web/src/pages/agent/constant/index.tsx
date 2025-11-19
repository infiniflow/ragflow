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
  SwitchOperatorOptions,
  initialLlmBaseValues,
} from '@/constants/agent';
export {
  AgentStructuredOutputField,
  JsonSchemaDataType,
  Operator,
} from '@/constants/agent';

export * from './pipeline';

export enum AgentDialogueMode {
  Conversational = 'conversational',
  Task = 'task',
}

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

export const SwitchLogicOperatorOptions = ['and', 'or'];

export const CommonOperatorList = Object.values(Operator).filter(
  (x) => x !== Operator.Note,
);

export const AgentOperatorList = [
  Operator.Retrieval,
  Operator.Categorize,
  Operator.Message,
  Operator.RewriteQuestion,
  Operator.KeywordExtract,
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

const initialQueryBaseValues = {
  query: [],
};

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

export const initialBeginValues = {
  mode: AgentDialogueMode.Conversational,
  prologue: `Hi! I'm your assistant. What can I do for you?`,
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

export const initialKeywordExtractValues = {
  ...initialLlmBaseValues,
  top_n: 3,
  ...initialQueryBaseValues,
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

export const initialQWeatherValues = {
  web_apikey: 'xxx',
  type: 'weather',
  user_type: 'free',
  time_period: 'now',
  ...initialQueryBaseValues,
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
      logical_operator: SwitchLogicOperatorOptions[0],
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

export const initialAkShareValues = { top_n: 10, ...initialQueryBaseValues };

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

export const initialJin10Values = {
  type: 'flash',
  secret_key: 'xxx',
  flash_type: '1',
  contain: '',
  filter: '',
  ...initialQueryBaseValues,
};

export const initialTuShareValues = {
  token: 'xxx',
  src: 'eastmoney',
  start_date: '2024-01-01 09:00:00',
  ...initialQueryBaseValues,
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
  [Operator.Begin]: [Operator.Relevant],
  [Operator.Categorize]: [Operator.Begin, Operator.Categorize],
  [Operator.Retrieval]: [Operator.Begin, Operator.Retrieval],
  [Operator.Message]: [
    Operator.Begin,
    Operator.Message,
    Operator.Retrieval,
    Operator.RewriteQuestion,
    Operator.Categorize,
  ],
  [Operator.Relevant]: [Operator.Begin],
  [Operator.RewriteQuestion]: [
    Operator.Begin,
    Operator.Message,
    Operator.RewriteQuestion,
    Operator.Relevant,
  ],
  [Operator.KeywordExtract]: [
    Operator.Begin,
    Operator.Message,
    Operator.Relevant,
  ],
  [Operator.DuckDuckGo]: [Operator.Begin, Operator.Retrieval],
  [Operator.Wikipedia]: [Operator.Begin, Operator.Retrieval],
  [Operator.PubMed]: [Operator.Begin, Operator.Retrieval],
  [Operator.ArXiv]: [Operator.Begin, Operator.Retrieval],
  [Operator.Google]: [Operator.Begin, Operator.Retrieval],
  [Operator.Bing]: [Operator.Begin, Operator.Retrieval],
  [Operator.GoogleScholar]: [Operator.Begin, Operator.Retrieval],
  [Operator.GitHub]: [Operator.Begin, Operator.Retrieval],
  [Operator.QWeather]: [Operator.Begin, Operator.Retrieval],
  [Operator.SearXNG]: [Operator.Begin, Operator.Retrieval],
  [Operator.ExeSQL]: [Operator.Begin],
  [Operator.Switch]: [Operator.Begin],
  [Operator.WenCai]: [Operator.Begin],
  [Operator.AkShare]: [Operator.Begin],
  [Operator.YahooFinance]: [Operator.Begin],
  [Operator.Jin10]: [Operator.Begin],
  [Operator.TuShare]: [Operator.Begin],
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
};

export const NodeMap = {
  [Operator.Begin]: 'beginNode',
  [Operator.Categorize]: 'categorizeNode',
  [Operator.Retrieval]: 'retrievalNode',
  [Operator.Message]: 'messageNode',
  [Operator.Relevant]: 'relevantNode',
  [Operator.RewriteQuestion]: 'rewriteNode',
  [Operator.KeywordExtract]: 'keywordNode',
  [Operator.DuckDuckGo]: 'ragNode',
  [Operator.Wikipedia]: 'ragNode',
  [Operator.PubMed]: 'ragNode',
  [Operator.ArXiv]: 'ragNode',
  [Operator.Google]: 'ragNode',
  [Operator.Bing]: 'ragNode',
  [Operator.GoogleScholar]: 'ragNode',
  [Operator.GitHub]: 'ragNode',
  [Operator.QWeather]: 'ragNode',
  [Operator.SearXNG]: 'ragNode',
  [Operator.ExeSQL]: 'ragNode',
  [Operator.Switch]: 'switchNode',
  [Operator.WenCai]: 'ragNode',
  [Operator.AkShare]: 'ragNode',
  [Operator.YahooFinance]: 'ragNode',
  [Operator.Jin10]: 'ragNode',
  [Operator.TuShare]: 'ragNode',
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
