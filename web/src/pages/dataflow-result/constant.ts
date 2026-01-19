export enum ChunkTextMode {
  Full = 'full',
  Ellipse = 'ellipse',
}

export enum TimelineNodeType {
  begin = 'file',
  parser = 'parser',
  contextGenerator = 'extractor',
  titleSplitter = 'hierarchicalMerger',
  characterSplitter = 'splitter',
  tokenizer = 'tokenizer',
  end = 'end',
}

export enum PipelineResultSearchParams {
  DocumentId = 'doc_id',
  KnowledgeId = 'knowledgeId',
  Type = 'type',
  IsReadOnly = 'is_read_only',
  AgentId = 'agent_id',
  AgentTitle = 'agent_title',
  CreatedBy = 'created_by', // Who uploaded the file
  DocumentExtension = 'extension',
}
