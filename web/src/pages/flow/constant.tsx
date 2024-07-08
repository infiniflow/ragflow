import { ReactComponent as BaiduIcon } from '@/assets/svg/baidu.svg';
import { ReactComponent as DuckIcon } from '@/assets/svg/duck.svg';
import { ReactComponent as KeywordIcon } from '@/assets/svg/keyword.svg';
import { variableEnabledFieldMap } from '@/constants/chat';
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
    description: 'This is where the flow begin',
    backgroundColor: '#cad6e0',
    color: '#385974',
  },
  [Operator.Generate]: {
    description: 'Generate description',
    backgroundColor: '#ebd6d6',
    width: 150,
    height: 150,
    fontSize: 20,
    iconFontSize: 30,
    color: '#996464',
  },
  [Operator.Answer]: {
    description:
      'This component is used as an interface between bot and human. It receives input of user and display the result of the computation of the bot.',
    backgroundColor: '#f4816d',
    color: 'white',
  },
  [Operator.Begin]: {
    description: 'Begin description',
    backgroundColor: '#4f51d6',
  },
  [Operator.Categorize]: {
    description: 'Categorize description',
    backgroundColor: '#ffebcd',
    color: '#cc8a26',
  },
  [Operator.Message]: {
    description: 'Message description',
    backgroundColor: '#c5ddc7',
    color: 'green',
  },
  [Operator.Relevant]: {
    description: 'BranchesOutlined description',
    backgroundColor: '#9fd94d',
    color: 'white',
    width: 70,
    height: 70,
    fontSize: 12,
    iconFontSize: 16,
  },
  [Operator.RewriteQuestion]: {
    description: 'RewriteQuestion description',
    backgroundColor: '#f8c7f8',
    color: 'white',
    width: 70,
    height: 70,
    fontSize: 12,
    iconFontSize: 16,
  },
};

export const componentMenuList = [
  {
    name: Operator.Retrieval,
    description: operatorMap[Operator.Retrieval].description,
  },
  {
    name: Operator.Generate,
    description: operatorMap[Operator.Generate].description,
  },
  {
    name: Operator.Answer,
    description: operatorMap[Operator.Answer].description,
  },
  {
    name: Operator.Categorize,
    description: operatorMap[Operator.Categorize].description,
  },
  {
    name: Operator.Message,
    description: operatorMap[Operator.Message].description,
  },
  {
    name: Operator.Relevant,
    description: operatorMap[Operator.Relevant].description,
  },
  {
    name: Operator.RewriteQuestion,
    description: operatorMap[Operator.RewriteQuestion].description,
  },
  // {
  //   name: Operator.KeywordExtract,
  //   description: operatorMap[Operator.Message].description,
  // },
  // {
  //   name: Operator.DuckDuckGo,
  //   description: operatorMap[Operator.Relevant].description,
  // },
  // {
  //   name: Operator.Baidu,
  //   description: operatorMap[Operator.RewriteQuestion].description,
  // },
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
  prompt: `Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:
  {input}
The above is the content you need to summarize.`,
  cite: true,
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

export const initialFormValuesMap = {
  [Operator.Begin]: initialBeginValues,
  [Operator.Retrieval]: initialRetrievalValues,
  [Operator.Generate]: initialGenerateValues,
  [Operator.Answer]: {},
  [Operator.Categorize]: initialCategorizeValues,
  [Operator.Relevant]: initialRelevantValues,
  [Operator.RewriteQuestion]: initialRewriteQuestionValues,
  [Operator.Message]: initialMessageValues,
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
  [Operator.KeywordExtract]: [Operator.Begin],
  [Operator.Baidu]: [Operator.Begin],
  [Operator.DuckDuckGo]: [Operator.Begin],
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
};
