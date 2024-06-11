import {
  MergeCellsOutlined,
  RocketOutlined,
  SendOutlined,
  SlidersOutlined,
} from '@ant-design/icons';

export enum Operator {
  Begin = 'Begin',
  Retrieval = 'Retrieval',
  Generate = 'Generate',
  Answer = 'Answer',
}

export const operatorIconMap = {
  [Operator.Retrieval]: RocketOutlined,
  [Operator.Generate]: MergeCellsOutlined,
  [Operator.Answer]: SendOutlined,
  [Operator.Begin]: SlidersOutlined,
};

export const componentList = [
  {
    name: Operator.Retrieval,
    description: '',
  },
  {
    name: Operator.Generate,
    description: '',
  },
  {
    name: Operator.Answer,
    description: '',
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

export const initialGenerateValues = {
  // parameters: ModelVariableType.Precise,
  // temperatureEnabled: true,
  temperature: 0.1,
  top_p: 0.3,
  frequency_penalty: 0.7,
  presence_penalty: 0.4,
  max_tokens: 512,
  prompt: `Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:
  {cluster_content}
The above is the content you need to summarize.`,
  cite: true,
};

export const initialFormValuesMap = {
  [Operator.Begin]: initialBeginValues,
  [Operator.Retrieval]: initialRetrievalValues,
  [Operator.Generate]: initialGenerateValues,
  [Operator.Answer]: {},
};
