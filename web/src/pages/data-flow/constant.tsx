import {
  initialKeywordsSimilarityWeightValue,
  initialSimilarityThresholdValue,
} from '@/components/similarity-slider';
import {
  AgentGlobals,
  CodeTemplateStrMap,
  ProgrammingLanguage,
} from '@/constants/agent';

import {
  ChatVariableEnabledField,
  variableEnabledFieldMap,
} from '@/constants/chat';
import { ModelVariableType } from '@/constants/knowledge';
import i18n from '@/locales/config';
import { setInitialChatVariableEnabledFieldValue } from '@/utils/chat';
import { t } from 'i18next';

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

export enum PromptRole {
  User = 'user',
  Assistant = 'assistant',
}

export enum AgentDialogueMode {
  Conversational = 'conversational',
  Task = 'task',
}

export const BeginId = 'begin';

export enum Operator {
  Begin = 'Begin',
  Retrieval = 'Retrieval',
  Categorize = 'Categorize',
  Message = 'Message',
  Relevant = 'Relevant',
  RewriteQuestion = 'RewriteQuestion',
  KeywordExtract = 'KeywordExtract',
  ExeSQL = 'ExeSQL',
  Switch = 'Switch',
  Concentrator = 'Concentrator',
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
  UserFillUp = 'UserFillUp',
  StringTransform = 'StringTransform',
  Parser = 'Parser',
  Chunker = 'Chunker',
  Tokenizer = 'Tokenizer',
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

export const initialConcentratorValues = {};

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

export const initialChunkerValues = { outputs: {} };

export const initialTokenizerValues = {};

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

export const initialParserValues = { outputs: {} };

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
  [Operator.ExeSQL]: [Operator.Begin],
  [Operator.Switch]: [Operator.Begin],
  [Operator.Concentrator]: [Operator.Begin],
  [Operator.Crawler]: [Operator.Begin],
  [Operator.Note]: [],
  [Operator.Invoke]: [Operator.Begin],
  [Operator.Email]: [Operator.Begin],
  [Operator.Iteration]: [Operator.Begin],
  [Operator.IterationStart]: [Operator.Begin],
  [Operator.Code]: [Operator.Begin],
  [Operator.WaitingDialogue]: [Operator.Begin],
  [Operator.Agent]: [Operator.Begin],
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
  [Operator.ExeSQL]: 'ragNode',
  [Operator.Switch]: 'switchNode',
  [Operator.Concentrator]: 'logicNode',
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
  [Operator.UserFillUp]: 'ragNode',
  [Operator.StringTransform]: 'ragNode',
  [Operator.Parser]: 'parserNode',
  [Operator.Chunker]: 'chunkerNode',
  [Operator.Tokenizer]: 'tokenizerNode',
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
