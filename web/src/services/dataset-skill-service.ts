import api from '@/utils/api';
import request from '@/utils/request';

const datasetSkillService = {
  hasAny: (params: { datasetId: string }) =>
    request.get(api.hasAnySkill(params.datasetId)),
  getTree: (params: { datasetId: string }) =>
    request.get(api.getDatasetSkillTree(params.datasetId)),
  getPage: (params: { datasetId: string; skillKwd: string }) =>
    request.get(api.getDatasetSkillPage(params.datasetId, params.skillKwd)),
};

export default datasetSkillService;
