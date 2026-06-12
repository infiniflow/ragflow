import api from '@/utils/api';
import request from '@/utils/request';

const datasetArtifactService = {
  hasAny: (params: { datasetId: string }) =>
    request.get(api.hasAnyArtifact(params.datasetId)),
  list: (params: {
    datasetId: string;
    page?: number;
    page_size?: number;
    page_type?: string;
  }) =>
    request.get(api.listDatasetArtifacts(params.datasetId), {
      params: {
        page: params.page,
        page_size: params.page_size,
        page_type: params.page_type,
      },
    }),
  getPage: (params: { datasetId: string; pageType: string; slug: string }) =>
    request.get(
      api.getDatasetArtifactPage(
        params.datasetId,
        params.pageType,
        params.slug,
      ),
    ),
  clear: (params: { datasetId: string }) =>
    request.delete(api.clearDatasetArtifacts(params.datasetId)),
  getGraph: (params: { datasetId: string }) =>
    request.get(api.getDatasetArtifactGraph(params.datasetId)),
};

export default datasetArtifactService;
