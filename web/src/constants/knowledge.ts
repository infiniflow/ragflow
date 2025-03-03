export enum KnowledgeRouteKey {
  Dataset = 'dataset',
  Testing = 'testing',
  Configuration = 'configuration',
  KnowledgeGraph = 'knowledgeGraph',
}

export const DatasetBaseKey = 'dataset';

export enum RunningStatus {
  UNSTART = '0', // need to run
  RUNNING = '1', // need to cancel
  CANCEL = '2', // need to refresh
  DONE = '3', // need to refresh
  FAIL = '4', // need to refresh
}

export enum ModelVariableType {
  Improvise = 'Improvise',
  Precise = 'Precise',
  Balance = 'Balance',
}

export const settledModelVariableMap = {
  [ModelVariableType.Improvise]: {
    temperature: 0.9,
    top_p: 0.9,
    frequency_penalty: 0.2,
    presence_penalty: 0.4,
    max_tokens: 512,
  },
  [ModelVariableType.Precise]: {
    temperature: 0.1,
    top_p: 0.3,
    frequency_penalty: 0.7,
    presence_penalty: 0.4,
    max_tokens: 512,
  },
  [ModelVariableType.Balance]: {
    temperature: 0.5,
    top_p: 0.5,
    frequency_penalty: 0.7,
    presence_penalty: 0.4,
    max_tokens: 512,
  },
};

export enum LlmModelType {
  Embedding = 'embedding',
  Chat = 'chat',
  Image2text = 'image2text',
  Speech2text = 'speech2text',
  Rerank = 'rerank',
  TTS = 'tts',
}

export enum KnowledgeSearchParams {
  DocumentId = 'doc_id',
  KnowledgeId = 'id',
}

export enum DocumentType {
  Virtual = 'virtual',
  Visual = 'visual',
}

export enum DocumentParserType {
  Naive = 'naive',
  Qa = 'qa',
  Resume = 'resume',
  Manual = 'manual',
  Table = 'table',
  Paper = 'paper',
  Book = 'book',
  Laws = 'laws',
  Presentation = 'presentation',
  Picture = 'picture',
  One = 'one',
  Audio = 'audio',
  Email = 'email',
  Tag = 'tag',
  KnowledgeGraph = 'knowledge_graph',
}
