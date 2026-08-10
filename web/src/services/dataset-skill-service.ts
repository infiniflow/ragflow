import api from '@/utils/api';
import request from '@/utils/next-request';

const datasetSkillService = {
  getTree: (params: { datasetId: string }) =>
    request.get(api.getDatasetSkillTree(params.datasetId)),
  getPage: (params: { datasetId: string; skillKwd: string }) =>
    request.get(api.getDatasetSkillPage(params.datasetId, params.skillKwd)),
  deleteTree: (params: { datasetId: string }) =>
    request.delete(api.deleteDatasetSkillTree(params.datasetId)),
  deletePage: (params: { datasetId: string; skillKwd: string }) =>
    request.delete(
      api.deleteDatasetSkillPage(params.datasetId, params.skillKwd),
    ),
};

export default datasetSkillService;
