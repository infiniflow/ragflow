import api from '@/utils/api';
import { registerNextServer } from '@/utils/register-server';

type DatasetIdParam = string | { datasetId: string };
type RunIdParam = string | { runId: string };

const datasetIdFrom = (param: DatasetIdParam) =>
  typeof param === 'string' ? param : param.datasetId;

const runIdFrom = (param: RunIdParam) =>
  typeof param === 'string' ? param : param.runId;

const evaluationService = registerNextServer({
  listEvaluationDatasets: {
    url: api.listEvaluationDatasets,
    method: 'get',
  },
  createEvaluationDataset: {
    url: api.createEvaluationDataset,
    method: 'post',
  },
  getEvaluationDataset: {
    url: api.getEvaluationDataset,
    method: 'get',
  },
  updateEvaluationDataset: {
    url: api.updateEvaluationDataset,
    method: 'put',
  },
  deleteEvaluationDataset: {
    url: api.deleteEvaluationDataset,
    method: 'delete',
  },
  listEvaluationCases: {
    url: (param: DatasetIdParam) =>
      api.listEvaluationCases(datasetIdFrom(param)),
    method: 'get',
  },
  addEvaluationCase: {
    url: (param: { datasetId: string }) =>
      api.addEvaluationCase(param.datasetId),
    method: 'post',
  },
  importEvaluationCases: {
    url: (param: { datasetId: string }) =>
      api.importEvaluationCases(param.datasetId),
    method: 'post',
  },
  deleteEvaluationCase: {
    url: (param: { datasetId: string; caseId: string }) =>
      api.deleteEvaluationCase(param.datasetId, param.caseId),
    method: 'delete',
  },
  listEvaluationRuns: {
    url: (param: DatasetIdParam) =>
      api.listEvaluationRuns(datasetIdFrom(param)),
    method: 'get',
  },
  startEvaluationRun: {
    url: (param: { datasetId: string }) =>
      api.startEvaluationRun(param.datasetId),
    method: 'post',
  },
  getEvaluationRun: {
    url: api.getEvaluationRun,
    method: 'get',
  },
  getEvaluationRunResults: {
    url: api.getEvaluationRunResults,
    method: 'get',
  },
  getEvaluationRecommendations: {
    url: api.getEvaluationRecommendations,
    method: 'get',
  },
});

export default evaluationService;
