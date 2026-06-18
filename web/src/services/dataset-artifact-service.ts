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
  updatePage: (params: {
    datasetId: string;
    pageType: string;
    slug: string;
    content_md: string;
  }) =>
    request.put(
      api.updateDatasetArtifactPage(
        params.datasetId,
        params.pageType,
        params.slug,
      ),
      { data: { content_md: params.content_md } },
    ),
  clear: (params: { datasetId: string }) =>
    request.delete(api.clearDatasetArtifacts(params.datasetId)),
  getGraph: (params: { datasetId: string; node?: string }) =>
    request.get(api.getDatasetArtifactGraph(params.datasetId), {
      params: params.node ? { node: params.node } : undefined,
    }),
  listCommits: (params: {
    datasetId: string;
    pageType: string;
    slug: string;
    page?: number;
    page_size?: number;
  }) =>
    request.get(
      api.listDatasetArtifactCommits(
        params.datasetId,
        params.pageType,
        params.slug,
      ),
      {
        params: { page: params.page, page_size: params.page_size },
      },
    ),
  getCommit: (params: { datasetId: string; commitId: string }) =>
    request.get(
      api.getDatasetArtifactCommit(params.datasetId, params.commitId),
    ),
};

export default datasetArtifactService;
