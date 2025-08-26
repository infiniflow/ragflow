import {
  initialKeywordsSimilarityWeightValue,
  initialSimilarityThresholdValue,
} from '@/components/similarity-slider';
import {
  AgentGlobals,
  CodeTemplateStrMap,
  ProgrammingLanguage,
} from '@/constants/agent';

export enum AgentDialogueMode {
  Conversational = 'conversational',
  Task = 'task',
}

import {
  ChatVariableEnabledField,
  variableEnabledFieldMap,
} from '@/constants/chat';
import { ModelVariableType } from '@/constants/knowledge';
import i18n from '@/locales/config';
import { setInitialChatVariableEnabledFieldValue } from '@/utils/chat';
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
  Circle,
  CircleSlash2,
  CloudUpload,
  ListOrdered,
  OptionIcon,
  TextCursorInput,
  ToggleLeft,
  WrapText,
} from 'lucide-react';

export const BeginId = 'begin';

export enum Operator {
  Begin = 'Begin',
  Retrieval = 'Retrieval',
  Categorize = 'Categorize',
  Message = 'Message',
  Relevant = 'Relevant',
  RewriteQuestion = 'RewriteQuestion',
  KeywordExtract = 'KeywordExtract',
  Baidu = 'Baidu',
  DuckDuckGo = 'DuckDuckGo',
  Wikipedia = 'Wikipedia',
  PubMed = 'PubMed',
  ArXiv = 'ArXiv',
  Google = 'Google',
  Bing = 'Bing',
  GoogleScholar = 'GoogleScholar',
  DeepL = 'DeepL',
  GitHub = 'GitHub',
  BaiduFanyi = 'BaiduFanyi',
  QWeather = 'QWeather',
  ExeSQL = 'ExeSQL',
  Switch = 'Switch',
  WenCai = 'WenCai',
  AkShare = 'AkShare',
  YahooFinance = 'YahooFinance',
  Jin10 = 'Jin10',
  Concentrator = 'Concentrator',
  TuShare = 'TuShare',
  Note = 'Note',
  Crawler = 'Crawler',
  Invoke = 'Invoke',
  Email = 'Email',
  Iteration = 'Iteration',
  IterationStart = 'IterationItem',
  Code = 'CodeExec',
  WaitingDialogue = 'WaitingDialogue',
  Agent = 'Agent',
  Tool = 'Tool',
  TavilySearch = 'TavilySearch',
  TavilyExtract = 'TavilyExtract',
  UserFillUp = 'UserFillUp',
  StringTransform = 'StringTransform',
}

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
  Operator.Concentrator,
  Operator.Iteration,
  Operator.WaitingDialogue,
  Operator.Note,
  Operator.Agent,
];

export const componentMenuList = [
  {
    name: Operator.Retrieval,
  },
  {
    name: Operator.Categorize,
  },
  {
    name: Operator.Message,
  },

  {
    name: Operator.RewriteQuestion,
  },
  {
    name: Operator.KeywordExtract,
  },
  {
    name: Operator.Switch,
  },
  {
    name: Operator.Concentrator,
  },
  {
    name: Operator.Iteration,
  },
  {
    name: Operator.Code,
  },
  {
    name: Operator.WaitingDialogue,
  },
  {
    name: Operator.Agent,
  },
  {
    name: Operator.Note,
  },
  {
    name: Operator.DuckDuckGo,
  },
  {
    name: Operator.Baidu,
  },
  {
    name: Operator.Wikipedia,
  },
  {
    name: Operator.PubMed,
  },
  {
    name: Operator.ArXiv,
  },
  {
    name: Operator.Google,
  },
  {
    name: Operator.Bing,
  },
  {
    name: Operator.GoogleScholar,
  },
  {
    name: Operator.DeepL,
  },
  {
    name: Operator.GitHub,
  },
  {
    name: Operator.BaiduFanyi,
  },
  {
    name: Operator.QWeather,
  },
  {
    name: Operator.ExeSQL,
  },
  {
    name: Operator.WenCai,
  },
  {
    name: Operator.AkShare,
  },
  {
    name: Operator.YahooFinance,
  },
  {
    name: Operator.Jin10,
  },
  {
    name: Operator.TuShare,
  },
  {
    name: Operator.Crawler,
  },
  {
    name: Operator.Invoke,
  },
  {
    name: Operator.Email,
  },
];

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

const initialQueryBaseValues = {
  query: [],
};

export const initialRetrievalValues = {
  query: AgentGlobals.SysQuery,
  top_n: 8,
  top_k: 1024,
  kb_ids: [],
  rerank_id: '',
  empty_response: '',
  ...initialSimilarityThresholdValue,
  ...initialKeywordsSimilarityWeightValue,
  use_kg: false,
  cross_languages: [],
  outputs: {
    formalized_content: {
      type: 'string',
      value: '',
    },
  },
};

export const initialBeginValues = {
  mode: AgentDialogueMode.Conversational,
  prologue: `Hi! I'm your assistant. What can I do for you?`,
};

export const variableCheckBoxFieldMap = Object.keys(
  variableEnabledFieldMap,
).reduce<Record<string, boolean>>((pre, cur) => {
  pre[cur] = setInitialChatVariableEnabledFieldValue(
    cur as ChatVariableEnabledField,
  );
  return pre;
}, {});

const initialLlmBaseValues = {
  ...variableCheckBoxFieldMap,
  temperature: 0.1,
  top_p: 0.3,
  frequency_penalty: 0.7,
  presence_penalty: 0.4,
  max_tokens: 256,
};

export const initialGenerateValues = {
  ...initialLlmBaseValues,
  prompt: i18n.t('flow.promptText'),
  cite: true,
  message_history_window_size: 12,
  parameters: [],
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

export const initialBaiduValues = {
  top_n: 10,
  ...initialQueryBaseValues,
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

export const initialDeepLValues = {
  top_n: 5,
  auth_key: 'relevance',
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

export const initialBaiduFanyiValues = {
  appid: 'xxx',
  secret_key: 'xxx',
  trans_type: 'translate',
  ...initialQueryBaseValues,
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

export const initialConcentratorValues = {};

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
  sys_prompt: t('flow.sysPromptDefultValue'),
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
  outputs: {
    // structured_output: {
    //   topic: {
    //     type: 'string',
    //     description:
    //       'default:general. The category of the search.news is useful for retrieving real-time updates, particularly about politics, sports, and major current events covered by mainstream media sources. general is for broader, more general-purpose searches that may include a wide range of sources.',
    //     enum: ['general', 'news'],
    //     default: 'general',
    //   },
    // },
    content: {
      type: 'string',
      value: '',
    },
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
  [Operator.Baidu]: [Operator.Begin, Operator.Retrieval],
  [Operator.DuckDuckGo]: [Operator.Begin, Operator.Retrieval],
  [Operator.Wikipedia]: [Operator.Begin, Operator.Retrieval],
  [Operator.PubMed]: [Operator.Begin, Operator.Retrieval],
  [Operator.ArXiv]: [Operator.Begin, Operator.Retrieval],
  [Operator.Google]: [Operator.Begin, Operator.Retrieval],
  [Operator.Bing]: [Operator.Begin, Operator.Retrieval],
  [Operator.GoogleScholar]: [Operator.Begin, Operator.Retrieval],
  [Operator.DeepL]: [Operator.Begin, Operator.Retrieval],
  [Operator.GitHub]: [Operator.Begin, Operator.Retrieval],
  [Operator.BaiduFanyi]: [Operator.Begin, Operator.Retrieval],
  [Operator.QWeather]: [Operator.Begin, Operator.Retrieval],
  [Operator.ExeSQL]: [Operator.Begin],
  [Operator.Switch]: [Operator.Begin],
  [Operator.WenCai]: [Operator.Begin],
  [Operator.AkShare]: [Operator.Begin],
  [Operator.YahooFinance]: [Operator.Begin],
  [Operator.Jin10]: [Operator.Begin],
  [Operator.Concentrator]: [Operator.Begin],
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
  [Operator.Baidu]: 'ragNode',
  [Operator.Wikipedia]: 'ragNode',
  [Operator.PubMed]: 'ragNode',
  [Operator.ArXiv]: 'ragNode',
  [Operator.Google]: 'ragNode',
  [Operator.Bing]: 'ragNode',
  [Operator.GoogleScholar]: 'ragNode',
  [Operator.DeepL]: 'ragNode',
  [Operator.GitHub]: 'ragNode',
  [Operator.BaiduFanyi]: 'ragNode',
  [Operator.QWeather]: 'ragNode',
  [Operator.ExeSQL]: 'ragNode',
  [Operator.Switch]: 'switchNode',
  [Operator.Concentrator]: 'logicNode',
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
  Operator.Concentrator,
  Operator.Message,
  Operator.RewriteQuestion,
  Operator.Switch,
  Operator.Iteration,
  Operator.UserFillUp,
  Operator.IterationStart,
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
