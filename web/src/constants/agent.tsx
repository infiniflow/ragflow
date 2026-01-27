import { setInitialChatVariableEnabledFieldValue } from '@/utils/chat';
import { Circle, CircleSlash2 } from 'lucide-react';
import { ChatVariableEnabledField, variableEnabledFieldMap } from './chat';

export enum ProgrammingLanguage {
  Python = 'python',
  Javascript = 'javascript',
}

export const CodeTemplateStrMap = {
  [ProgrammingLanguage.Python]: `def main(arg1: str, arg2: str) -> str:
    return f"result: {arg1 + arg2}"
`,
  [ProgrammingLanguage.Javascript]: `const axios = require('axios');
async function main({}) {
  try {
    const response = await axios.get('https://github.com/infiniflow/ragflow');
    return 'Body:' + response.data;
  } catch (error) {
    return 'Error:' + error.message;
  }
}`,
};

export enum AgentGlobals {
  SysQuery = 'sys.query',
  SysUserId = 'sys.user_id',
  SysConversationTurns = 'sys.conversation_turns',
  SysFiles = 'sys.files',
  SysHistory = 'sys.history',
}

export const AgentGlobalsSysQueryWithBrace = `{${AgentGlobals.SysQuery}}`;

export const variableCheckBoxFieldMap = Object.keys(
  variableEnabledFieldMap,
).reduce<Record<string, boolean>>((pre, cur) => {
  pre[cur] = setInitialChatVariableEnabledFieldValue(
    cur as ChatVariableEnabledField,
  );
  return pre;
}, {});

export const initialLlmBaseValues = {
  ...variableCheckBoxFieldMap,
  temperature: 0.1,
  top_p: 0.3,
  frequency_penalty: 0.7,
  presence_penalty: 0.4,
  max_tokens: 256,
};

export enum AgentCategory {
  AgentCanvas = 'agent_canvas',
  DataflowCanvas = 'dataflow_canvas',
}

export enum AgentQuery {
  Category = 'category',
}

export enum DataflowOperator {
  Begin = 'File',
  Note = 'Note',
  Parser = 'Parser',
  Tokenizer = 'Tokenizer',
  Splitter = 'Splitter',
  HierarchicalMerger = 'HierarchicalMerger',
  Extractor = 'Extractor',
}

export enum Operator {
  Begin = 'Begin',
  Retrieval = 'Retrieval',
  Categorize = 'Categorize',
  Message = 'Message',
  RewriteQuestion = 'RewriteQuestion',
  DuckDuckGo = 'DuckDuckGo',
  Wikipedia = 'Wikipedia',
  PubMed = 'PubMed',
  ArXiv = 'ArXiv',
  Google = 'Google',
  Bing = 'Bing',
  GoogleScholar = 'GoogleScholar',
  GitHub = 'GitHub',
  ExeSQL = 'ExeSQL',
  Switch = 'Switch',
  WenCai = 'WenCai',
  YahooFinance = 'YahooFinance',
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
  SearXNG = 'SearXNG',
  PDFGenerator = 'PDFGenerator',
  Placeholder = 'Placeholder',
  DataOperations = 'DataOperations',
  ListOperations = 'ListOperations',
  VariableAssigner = 'VariableAssigner',
  VariableAggregator = 'VariableAggregator',
  File = 'File', // pipeline
  Parser = 'Parser',
  Tokenizer = 'Tokenizer',
  Splitter = 'Splitter',
  HierarchicalMerger = 'HierarchicalMerger',
  Extractor = 'Extractor',
  Loop = 'Loop',
  LoopStart = 'LoopItem',
  ExitLoop = 'ExitLoop',
  ExcelProcessor = 'ExcelProcessor',
}

export enum ComparisonOperator {
  Equal = '=',
  NotEqual = '≠',
  GreatThan = '>',
  GreatEqual = '≥',
  LessThan = '<',
  LessEqual = '≤',
  Contains = 'contains',
  NotContains = 'not contains',
  StartWith = 'start with',
  EndWith = 'end with',
  Empty = 'empty',
  NotEmpty = 'not empty',
  In = 'in',
  NotIn = 'not in',
}

export const SwitchOperatorOptions = [
  { value: ComparisonOperator.Equal, label: 'equal', icon: 'equal' },
  { value: ComparisonOperator.NotEqual, label: 'notEqual', icon: 'not-equals' },
  { value: ComparisonOperator.GreatThan, label: 'gt', icon: 'Less' },
  {
    value: ComparisonOperator.GreatEqual,
    label: 'ge',
    icon: 'Greater-or-equal',
  },
  { value: ComparisonOperator.LessThan, label: 'lt', icon: 'Less' },
  { value: ComparisonOperator.LessEqual, label: 'le', icon: 'less-or-equal' },
  { value: ComparisonOperator.Contains, label: 'contains', icon: 'Contains' },
  {
    value: ComparisonOperator.NotContains,
    label: 'notContains',
    icon: 'not-contains',
  },
  {
    value: ComparisonOperator.StartWith,
    label: 'startWith',
    icon: 'list-start',
  },
  { value: ComparisonOperator.EndWith, label: 'endWith', icon: 'list-end' },
  {
    value: ComparisonOperator.Empty,
    label: 'empty',
    icon: <Circle className="size-4" />,
  },
  {
    value: ComparisonOperator.NotEmpty,
    label: 'notEmpty',
    icon: <CircleSlash2 className="size-4" />,
  },
  {
    value: ComparisonOperator.In,
    label: 'in',
    icon: <CircleSlash2 className="size-4" />,
  },
  {
    value: ComparisonOperator.NotIn,
    label: 'notIn',
    icon: <CircleSlash2 className="size-4" />,
  },
];

export const AgentStructuredOutputField = 'structured';

export enum JsonSchemaDataType {
  String = 'string',
  Number = 'number',
  Boolean = 'boolean',
  Array = 'array',
  Object = 'object',
}

export enum SwitchLogicOperator {
  And = 'and',
  Or = 'or',
}

export const WebhookJWTAlgorithmList = [
  'hs256',
  'hs384',
  'hs512',
  'rs256',
  'rs384',
  'rs512',
  'es256',
  'es384',
  'es512',
  'ps256',
  'ps384',
  'ps512',
  'none',
] as const;

export enum AgentDialogueMode {
  Conversational = 'conversational',
  Task = 'task',
  Webhook = 'Webhook',
}

export const initialBeginValues = {
  mode: AgentDialogueMode.Conversational,
  prologue: `Hi! I'm your assistant. What can I do for you?`,
};
