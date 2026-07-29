export enum LogTabs {
  FILE_LOGS = 'fileLogs',
  DATASET_LOGS = 'datasetLogs',
}

export enum ProcessingType {
  knowledgeGraph = 'Graph',
  raptor = 'RAPTOR',
  artifact = 'Artifact',
  skill = 'Skill',
  mindmap = 'Mindmap',
  timeline = 'Timeline',
  sessionEssence = 'Session_Essence',
  sessionGraph = 'Session_Graph',
}

export const ProcessingTypeMap = {
  [ProcessingType.knowledgeGraph]: 'Knowledge Graph',
  [ProcessingType.raptor]: 'RAPTOR',
  [ProcessingType.artifact]: 'Artifact',
  [ProcessingType.skill]: 'Skill',
  [ProcessingType.mindmap]: 'Mind Map',
  [ProcessingType.timeline]: 'Timeline',
  [ProcessingType.sessionEssence]: 'Session Essence',
  [ProcessingType.sessionGraph]: 'Session Graph',
  GraphRAG: 'Knowledge Graph',
};
