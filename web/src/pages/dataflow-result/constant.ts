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
  tokenizer = 'indexer',
  end = 'end',
}
