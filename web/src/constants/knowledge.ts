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
  SCHEDULE = '5',
}

export const RunningStatusMap = {
  [RunningStatus.UNSTART]: 'Pending',
  [RunningStatus.RUNNING]: 'Running',
  [RunningStatus.CANCEL]: 'Cancel',
  [RunningStatus.DONE]: 'Success',
  [RunningStatus.FAIL]: 'Failed',
  [RunningStatus.SCHEDULE]: 'Schedule',
};

export enum ModelVariableType {
  Improvise = 'Improvise',
  Precise = 'Precise',
  Balance = 'Balance',
}

export const settledModelVariableMap = {
  [ModelVariableType.Improvise]: {
    temperature: 0.8,
    top_p: 0.9,
    frequency_penalty: 0.1,
    presence_penalty: 0.1,
    max_tokens: 4096,
  },
  [ModelVariableType.Precise]: {
    temperature: 0.2,
    top_p: 0.75,
    frequency_penalty: 0.5,
    presence_penalty: 0.5,
    max_tokens: 4096,
  },
  [ModelVariableType.Balance]: {
    temperature: 0.5,
    top_p: 0.85,
    frequency_penalty: 0.3,
    presence_penalty: 0.2,
    max_tokens: 4096,
  },
};

export enum LlmModelType {
  Embedding = 'embedding',
  Chat = 'chat',
  Image2text = 'image2text',
  Speech2text = 'speech2text',
  Rerank = 'rerank',
  TTS = 'tts',
  Ocr = 'ocr',
}

export enum KnowledgeSearchParams {
  DocumentId = 'doc_id',
  KnowledgeId = 'id',
  Type = 'type',
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

export const TagRenameId = 'tagRename';
