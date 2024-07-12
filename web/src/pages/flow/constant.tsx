import { ReactComponent as BaiduIcon } from '@/assets/svg/baidu.svg';
import { ReactComponent as DuckIcon } from '@/assets/svg/duck.svg';
import { ReactComponent as KeywordIcon } from '@/assets/svg/keyword.svg';
import { variableEnabledFieldMap } from '@/constants/chat';
import i18n from '@/locales/config';

import {
  BranchesOutlined,
  DatabaseOutlined,
  FormOutlined,
  MergeCellsOutlined,
  MessageOutlined,
  RocketOutlined,
  SendOutlined,
  SlidersOutlined,
} from '@ant-design/icons';

export enum Operator {
  Begin = 'Begin',
  Retrieval = 'Retrieval',
  Generate = 'Generate',
  Answer = 'Answer',
  Categorize = 'Categorize',
  Message = 'Message',
  Relevant = 'Relevant',
  RewriteQuestion = 'RewriteQuestion',
  KeywordExtract = 'KeywordExtract',
  Baidu = 'Baidu',
  DuckDuckGo = 'DuckDuckGo',
}

export const operatorIconMap = {
  [Operator.Retrieval]: RocketOutlined,
  [Operator.Generate]: MergeCellsOutlined,
  [Operator.Answer]: SendOutlined,
  [Operator.Begin]: SlidersOutlined,
  [Operator.Categorize]: DatabaseOutlined,
  [Operator.Message]: MessageOutlined,
  [Operator.Relevant]: BranchesOutlined,
  [Operator.RewriteQuestion]: FormOutlined,
  [Operator.KeywordExtract]: KeywordIcon,
  [Operator.DuckDuckGo]: DuckIcon,
  [Operator.Baidu]: BaiduIcon,
};

export const operatorMap = {
  [Operator.Retrieval]: {
    backgroundColor: '#cad6e0',
    color: '#385974',
  },
  [Operator.Generate]: {
    backgroundColor: '#ebd6d6',
    width: 150,
    height: 150,
    fontSize: 20,
    iconFontSize: 30,
    color: '#996464',
  },
  [Operator.Answer]: {
    backgroundColor: '#f4816d',
    color: 'white',
  },
  [Operator.Begin]: {
    backgroundColor: '#4f51d6',
  },
  [Operator.Categorize]: {
    backgroundColor: '#ffebcd',
    color: '#cc8a26',
  },
  [Operator.Message]: {
    backgroundColor: '#c5ddc7',
    color: 'green',
  },
  [Operator.Relevant]: {
    backgroundColor: '#9fd94d',
    color: 'white',
    width: 70,
    height: 70,
    fontSize: 12,
    iconFontSize: 16,
  },
  [Operator.RewriteQuestion]: {
    backgroundColor: '#f8c7f8',
    color: 'white',
    width: 70,
    height: 70,
    fontSize: 12,
    iconFontSize: 16,
  },
  [Operator.KeywordExtract]: {
    width: 70,
    height: 70,
    backgroundColor: '#0f0e0f',
    color: '#e1dcdc',
    fontSize: 12,
    iconWidth: 16,
    // iconFontSize: 16,
  },
  [Operator.DuckDuckGo]: {
    backgroundColor: '#e7e389',
    color: '#aea00c',
  },
  [Operator.Baidu]: {},
};

export const componentMenuList = [
  {
    name: Operator.Retrieval,
  },
  {
    name: Operator.Generate,
  },
  {
    name: Operator.Answer,
  },
  {
    name: Operator.Categorize,
  },
  {
    name: Operator.Message,
  },
  {
    name: Operator.Relevant,
  },
  {
    name: Operator.RewriteQuestion,
  },
  {
    name: Operator.KeywordExtract,
  },
  {
    name: Operator.DuckDuckGo,
  },
  {
    name: Operator.Baidu,
  },
];

export const initialRetrievalValues = {
  similarity_threshold: 0.2,
  keywords_similarity_weight: 0.3,
  top_n: 8,
};

export const initialBeginValues = {
  prologue: `Hi! I'm your assistant, what can I do for you?`,
};

export const variableCheckBoxFieldMap = Object.keys(
  variableEnabledFieldMap,
).reduce<Record<string, boolean>>((pre, cur) => {
  pre[cur] = true;
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
  loop: 1,
};

export const initialRelevantValues = {
  ...initialLlmBaseValues,
};

export const initialCategorizeValues = {
  ...initialLlmBaseValues,
  category_description: {},
};

export const initialMessageValues = {
  messages: [],
};

export const initialKeywordExtractValues = {
  ...initialLlmBaseValues,
  top_n: 1,
};

export const initialFormValuesMap = {
  [Operator.Begin]: initialBeginValues,
  [Operator.Retrieval]: initialRetrievalValues,
  [Operator.Generate]: initialGenerateValues,
  [Operator.Answer]: {},
  [Operator.Categorize]: initialCategorizeValues,
  [Operator.Relevant]: initialRelevantValues,
  [Operator.RewriteQuestion]: initialRewriteQuestionValues,
  [Operator.Message]: initialMessageValues,
  [Operator.KeywordExtract]: initialKeywordExtractValues,
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
  [Operator.Categorize]: [
    Operator.Begin,
    Operator.Categorize,
    Operator.Answer,
    Operator.Relevant,
  ],
  [Operator.Answer]: [
    Operator.Begin,
    Operator.Answer,
    Operator.Message,
    Operator.Relevant,
  ],
  [Operator.Retrieval]: [Operator.Begin, Operator.Retrieval],
  [Operator.Generate]: [Operator.Begin, Operator.Relevant],
  [Operator.Message]: [
    Operator.Begin,
    Operator.Message,
    Operator.Generate,
    Operator.Retrieval,
    Operator.RewriteQuestion,
    Operator.Categorize,
    Operator.Relevant,
  ],
  [Operator.Relevant]: [Operator.Begin, Operator.Answer, Operator.Relevant],
  [Operator.RewriteQuestion]: [
    Operator.Begin,
    Operator.Message,
    Operator.Generate,
    Operator.RewriteQuestion,
    Operator.Categorize,
    Operator.Relevant,
  ],
  [Operator.KeywordExtract]: [
    Operator.Begin,
    Operator.Message,
    Operator.Relevant,
  ],
  [Operator.Baidu]: [Operator.Begin, Operator.Retrieval],
  [Operator.DuckDuckGo]: [Operator.Begin, Operator.Retrieval],
};

export const NodeMap = {
  [Operator.Begin]: 'beginNode',
  [Operator.Categorize]: 'categorizeNode',
  [Operator.Retrieval]: 'ragNode',
  [Operator.Generate]: 'ragNode',
  [Operator.Answer]: 'ragNode',
  [Operator.Message]: 'ragNode',
  [Operator.Relevant]: 'relevantNode',
  [Operator.RewriteQuestion]: 'ragNode',
  [Operator.KeywordExtract]: 'ragNode',
  [Operator.DuckDuckGo]: 'ragNode',
  [Operator.Baidu]: 'ragNode',
};
