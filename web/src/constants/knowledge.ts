export enum KnowledgeRouteKey {
  Dataset = 'dataset',
  Testing = 'testing',
  Configuration = 'configuration',
  TempTesting = 'tempTesting',
}

export enum RunningStatus {
  UNSTART = '0', // need to run
  RUNNING = '1', // need to cancel
  CANCEL = '2', // need to refresh
  DONE = '3', // need to refresh
  FAIL = '4', // need to refresh
}
