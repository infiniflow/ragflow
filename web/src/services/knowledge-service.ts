import { Authorization } from '@/constants/authorization';
import { IRenameTag } from '@/interfaces/database/knowledge';
import {
  IFetchDocumentListRequestBody,
  IFetchKnowledgeListRequestParams,
} from '@/interfaces/request/knowledge';
import { ProcessingType } from '@/pages/dataset/dataset-overview/dataset-common';
import api from '@/utils/api';
import { getAuthorization } from '@/utils/authorization-util';
import registerServer from '@/utils/register-server';
import request, { post } from '@/utils/request';
import axios from 'axios';

const {
  createKb,
  rmKb,
  getKbDetail,
  kbList,
  getDocumentList,
  documentChangeStatus,
  documentRm,
  documentDelete,
  documentCreate,
  documentChangeParser,
  documentThumbnails,
  chunkList,
  createChunk,
  setChunk,
  getChunk,
  switchChunk,
  rmChunk,
  retrievalTest,
  documentRun,
  documentUpload,
  webCrawl,
  knowledgeGraph,
  documentInfos,
  listTagByKnowledgeIds,
  setMeta,
  getMeta,
  retrievalTestShare,
  getKnowledgeBasicInfo,
  fetchDataPipelineLog,
  fetchPipelineDatasetLogs,
  checkEmbedding,
  kbUpdateMetaData,
  documentUpdateMetaData,
} = api;

const methods = {
  createKb: {
    url: createKb,
    method: 'post',
  },
  rmKb: {
    url: rmKb,
    method: 'delete',
  },
  getKbDetail: {
    url: getKbDetail,
    method: 'get',
  },
  getList: {
    url: kbList,
    method: 'get',
  },
  // document manager
  getDocumentList: {
    url: getDocumentList,
    method: 'get',
  },
  documentChangeStatus: {
    url: documentChangeStatus,
    method: 'post',
  },
  documentRm: {
    url: documentRm,
    method: 'post',
  },
  documentCreate: {
    url: documentCreate,
    method: 'post',
  },
  documentRun: {
    url: documentRun,
    method: 'post',
  },
  documentChangeParser: {
    url: documentChangeParser,
    method: 'post',
  },
  documentThumbnails: {
    url: documentThumbnails,
    method: 'get',
  },
  documentUpload: {
    url: documentUpload,
    method: 'post',
  },
  webCrawl: {
    url: webCrawl,
    method: 'post',
  },
  documentInfos: {
    url: documentInfos,
    method: 'post',
  },
  setMeta: {
    url: setMeta,
    method: 'post',
  },
  // chunk管理
  chunkList: {
    url: chunkList,
    method: 'post',
  },
  createChunk: {
    url: createChunk,
    method: 'post',
  },
  setChunk: {
    url: setChunk,
    method: 'post',
  },
  getChunk: {
    url: getChunk,
    method: 'get',
  },
  switchChunk: {
    url: switchChunk,
    method: 'post',
  },
  rmChunk: {
    url: rmChunk,
    method: 'post',
  },
  retrievalTest: {
    url: retrievalTest,
    method: 'post',
  },
  knowledgeGraph: {
    url: knowledgeGraph,
    method: 'get',
  },
  documentDelete: {
    url: documentDelete,
    method: 'delete',
  },
  listTagByKnowledgeIds: {
    url: listTagByKnowledgeIds,
    method: 'get',
  },
  documentFilter: {
    url: api.getDatasetFilter,
    method: 'post',
  },
  getMeta: {
    url: getMeta,
    method: 'get',
  },
  retrievalTestShare: {
    url: retrievalTestShare,
    method: 'post',
  },
  getKnowledgeBasicInfo: {
    url: getKnowledgeBasicInfo,
    method: 'get',
  },
  fetchDataPipelineLog: {
    url: fetchDataPipelineLog,
    method: 'post',
  },
  fetchPipelineDatasetLogs: {
    url: fetchPipelineDatasetLogs,
    method: 'post',
  },
  getPipelineDetail: {
    url: api.getPipelineDetail,
    method: 'get',
  },

  pipelineRerun: {
    url: api.pipelineRerun,
    method: 'post',
  },

  checkEmbedding: {
    url: checkEmbedding,
    method: 'post',
  },
  kbUpdateMetaData: {
    url: kbUpdateMetaData,
    method: 'post',
  },
  documentUpdateMetaData: {
    url: documentUpdateMetaData,
    method: 'post',
  },
  // getMetaData: {
  //   url: getMetaData,
  //   method: 'get',
  // },
};

const kbService = registerServer<keyof typeof methods>(methods, request);

export const listTag = (knowledgeId: string) =>
  request.get(api.listTag(knowledgeId));

export const removeTag = (knowledgeId: string, tags: string[]) =>
  post(api.removeTag(knowledgeId), { tags });

export const renameTag = (
  knowledgeId: string,
  { fromTag, toTag }: IRenameTag,
) => post(api.renameTag(knowledgeId), { fromTag, toTag });

export function getKnowledgeGraph(knowledgeId: string) {
  return request.get(api.getKnowledgeGraph(knowledgeId));
}

export function deleteKnowledgeGraph(knowledgeId: string) {
  return request.delete(api.getKnowledgeGraph(knowledgeId));
}

export const listDataset = (params?: IFetchKnowledgeListRequestParams) =>
  request.get(api.kbList, { params });

export const updateKb = (datasetId: string, data: Record<string, any>) =>
  request.put(api.updateKb(datasetId), { data });

export const runIndex = (datasetId: string, indexType: string) =>
  request.post(api.runIndex(datasetId, indexType));

export const traceIndex = (datasetId: string, indexType: string) =>
  request.get(api.traceIndex(datasetId, indexType));

// Using RESTful API: GET /api/v1/datasets/{dataset_id}/documents
export const listDocument = (
  params?: IFetchKnowledgeListRequestParams,
  body?: IFetchDocumentListRequestBody,
) => {
  if (!params || !params.id) {
    throw new Error('params and params.id are required');
  }
  // Extract page, page_size, and ext.keywords from params
  const { page, page_size, ext } = params;
  // Merge: page, page_size, keywords (from ext), body, and remaining params
  const mergedParams = {
    page,
    page_size,
    keywords: ext?.keywords,
    ...body,
  };
  return request.get(api.getDocumentList(params.id), { params: mergedParams });
};

export const documentFilter = (kb_id: string) =>
  request.post(api.getDatasetFilter, { kb_id });

// Custom upload function that handles dynamic URL using axios directly
export const uploadDocument = async (datasetId: string, formData: FormData) => {
  const url = api.documentUpload(datasetId);
  const response = await axios.post(url, formData, {
    headers: {
      [Authorization]: getAuthorization(),
    },
  });
  return response.data;
};

export const renameDocument = (
  datasetId: string,
  documentId: string,
  data: { name?: string },
) => request.patch(api.documentRename(datasetId, documentId), { data });

export const getMetaDataService = ({
  kb_id,
  doc_ids,
}: {
  kb_id: string;
  doc_ids?: string[];
}) =>
  request.get(api.getMetaData(kb_id), {
    params: doc_ids?.length ? { doc_ids: doc_ids.join(',') } : undefined,
  });
export const updateMetaData = ({
  kb_id,
  doc_ids,
  data,
}: {
  kb_id: string;
  doc_ids?: string[];
  data: any;
}) => request.post(api.updateMetaData, { data: { kb_id, doc_ids, ...data } });

export const listDataPipelineLogDocument = (
  params?: IFetchKnowledgeListRequestParams,
  body?: IFetchDocumentListRequestBody,
) => request.post(api.fetchDataPipelineLog, { data: body || {}, params });
export const listPipelineDatasetLogs = (
  params?: IFetchKnowledgeListRequestParams & {
    kb_id?: string;
    keywords?: string;
  },
  body?: IFetchDocumentListRequestBody,
) => request.post(api.fetchPipelineDatasetLogs, { data: body || {}, params });

export function deletePipelineTask({
  kb_id,
  type,
}: {
  kb_id: string;
  type: ProcessingType;
}) {
  return request.delete(api.unbindPipelineTask({ kb_id, type }));
}

export default kbService;
