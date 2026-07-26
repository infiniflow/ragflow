import api from '@/utils/api';
import request from '@/utils/next-request';

const datasetNavService = {
  getNav: (params: { datasetId: string }) =>
    request.get(api.getDatasetNav(params.datasetId)),
  getNavChildren: (params: { datasetId: string; name: string }) =>
    request.get(api.getDatasetNavChildren(params.datasetId, params.name)),
  deleteNav: (params: { datasetId: string }) =>
    request.delete(api.deleteDatasetNav(params.datasetId)),
  deleteNavNode: (params: { datasetId: string; name: string }) =>
    request.delete(api.deleteDatasetNavNode(params.datasetId, params.name)),
};

export default datasetNavService;
