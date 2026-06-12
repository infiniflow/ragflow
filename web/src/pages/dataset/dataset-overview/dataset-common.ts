export enum LogTabs {
  FILE_LOGS = 'fileLogs',
  DATASET_LOGS = 'datasetLogs',
}

export enum ProcessingType {
  knowledgeGraph = 'Graph',
  raptor = 'RAPTOR',
  artifact = 'Artifact',
}

export const ProcessingTypeMap = {
  [ProcessingType.knowledgeGraph]: 'Knowledge Graph',
  [ProcessingType.raptor]: 'RAPTOR',
  [ProcessingType.artifact]: 'Artifact',
  GraphRAG: 'Knowledge Graph',
};
