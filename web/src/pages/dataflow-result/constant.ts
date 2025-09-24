export enum ChunkTextMode {
  Full = 'full',
  Ellipse = 'ellipse',
}

export enum TimelineNodeType {
  begin = 'file',
  parser = 'parser',
  splitter = 'splitter',
  contextGenerator = 'contextGenerator',
  titleSplitter = 'titleSplitter',
  characterSplitter = 'characterSplitter',
  tokenizer = 'tokenizer',
  end = 'end',
}
