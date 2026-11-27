export enum ChunkTextMode {
  Full = 'full',
  Ellipse = 'ellipse',
}

export enum TimelineNodeType {
  begin = 'file',
  parser = 'parser',
  contextGenerator = 'extractor',
  titleChunker = 'titleChunker',
  tokenChunker = 'tokenChunker',
  tokenizer = 'tokenizer',
  compiler = 'compiler',
  end = 'end',
}

export enum PipelineResultSearchParams {
  DocumentId = 'doc_id',
  KnowledgeId = 'knowledgeId',
  Type = 'type',
  IsReadOnly = 'is_read_only',
}
