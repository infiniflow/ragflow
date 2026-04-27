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
import request from '@/utils/request';
import axios from 'axios';

const {
  createKb,
  rmKb,
  kbList,
  documentChangeStatus,
  documentChangeParser,
  documentThumbnails,
  retrievalTest,
  documentRun,
  documentUpload,
  webCrawl,
  knowledgeGraph,
  listTagByKnowledgeIds,
  setMeta,
  getMeta,
  retrievalTestShare,
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
  getList: {
    url: kbList,
    method: 'get',
  },
  // document manager
  documentChangeStatus: {
    url: documentChangeStatus,
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
  setMeta: {
    url: setMeta,
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
  listTagByKnowledgeIds: {
    url: listTagByKnowledgeIds,
    method: 'get',
  },
  documentFilter: {
    url: api.getDatasetFilter,
    method: 'get',
  },
  getMeta: {
    url: getMeta,
    method: 'get',
  },
  retrievalTestShare: {
    url: retrievalTestShare,
    method: 'post',
  },
  pipelineRerun: {
    url: api.pipelineRerun,
    method: 'post',
  },
};

const baseKbService = registerServer<keyof typeof methods>(methods, request);

const getDatasetId = (params: Record<string, any>) =>
  params.dataset_id || params.kb_id || params.knowledge_id;

const getDocumentId = (params: Record<string, any>) =>
  params.document_id || params.doc_id;

const mapChunkToLegacy = (chunk: Record<string, any>) => ({
  ...chunk,
  chunk_id: chunk.chunk_id || chunk.id,
  content_with_weight: chunk.content_with_weight || chunk.content,
  doc_id: chunk.doc_id || chunk.document_id,
  doc_name: chunk.doc_name || chunk.docnm_kwd,
  image_id: chunk.image_id || chunk.img_id,
  important_kwd: chunk.important_kwd || chunk.important_keywords || [],
  question_kwd: chunk.question_kwd || chunk.questions || [],
  available_int: chunk.available_int ?? (chunk.available === false ? 0 : 1),
  positions: chunk.positions || chunk.position_int || [],
});

const mapDocumentToLegacy = (doc: Record<string, any>) => ({
  ...doc,
  chunk_num: doc.chunk_num ?? doc.chunk_count,
  kb_id: doc.kb_id || doc.dataset_id,
});

const mapChunkPayloadToRest = (payload: Record<string, any>) => ({
  content: payload.content ?? payload.content_with_weight,
  important_keywords: payload.important_keywords ?? payload.important_kwd,
  questions: payload.questions ?? payload.question_kwd,
  tag_kwd: payload.tag_kwd,
  tag_feas: payload.tag_feas,
  positions: payload.positions,
  available:
    payload.available ??
    (payload.available_int === undefined
      ? undefined
      : payload.available_int === 1),
  image_base64: payload.image_base64,
});

const getAvailableParam = (available?: number) => {
  if (available === undefined) {
    return undefined;
  }
  return available === 1 ? 'true' : 'false';
};

const chunkService = {
  chunkList: async (params: Record<string, any>) => {
    const datasetId = getDatasetId(params);
    const documentId = getDocumentId(params);
    const response = await request.get(api.chunkList(datasetId, documentId), {
      params: {
        page: params.page,
        page_size: params.page_size || params.size,
        keywords: params.keywords,
        available: getAvailableParam(params.available_int),
      },
    });

    if (response.data?.code === 0) {
      response.data.data = {
        ...response.data.data,
        chunks: (response.data.data?.chunks || []).map(mapChunkToLegacy),
        doc: mapDocumentToLegacy(response.data.data?.doc || {}),
      };
    }

    return response;
  },
  createChunk: async (payload: Record<string, any>) => {
    const datasetId = getDatasetId(payload);
    const documentId = getDocumentId(payload);
    const response = await request.post(api.chunkList(datasetId, documentId), {
      data: mapChunkPayloadToRest(payload),
    });

    if (response.data?.code === 0 && response.data.data?.chunk) {
      response.data.data.chunk = mapChunkToLegacy(response.data.data.chunk);
    }

    return response;
  },
  setChunk: (payload: Record<string, any>) => {
    const datasetId = getDatasetId(payload);
    const documentId = getDocumentId(payload);
    const chunkId = payload.chunk_id || payload.id;
    return request.patch(api.chunkDetail(datasetId, documentId, chunkId), {
      data: mapChunkPayloadToRest(payload),
    });
  },
  getChunk: async (params: Record<string, any>) => {
    const datasetId = getDatasetId(params);
    const documentId = getDocumentId(params);
    const chunkId = params.chunk_id || params.id;
    const response = await request.get(
      api.chunkDetail(datasetId, documentId, chunkId),
    );

    if (response.data?.code === 0) {
      response.data.data = mapChunkToLegacy(response.data.data || {});
    }

    return response;
  },
  switchChunk: (params: Record<string, any>) => {
    const datasetId = getDatasetId(params);
    const documentId = getDocumentId(params);
    return request.patch(api.chunkList(datasetId, documentId), {
      data: {
        chunk_ids: params.chunk_ids || params.chunkIds,
        available_int: params.available_int,
      },
    });
  },
  rmChunk: (params: Record<string, any>) => {
    const datasetId = getDatasetId(params);
    const documentId = getDocumentId(params);
    return request.delete(api.chunkList(datasetId, documentId), {
      data: {
        chunk_ids: params.chunk_ids || params.chunkIds,
        delete_all: params.delete_all,
      },
    });
  },
};

const kbService = {
  ...baseKbService,
  ...chunkService,
};

export const getKbDetail = (datasetId: string) =>
  request.get(api.getKbDetail(datasetId));

export const listTag = (knowledgeId: string) =>
  request.get(api.listTag(knowledgeId));

export const removeTag = (knowledgeId: string, tags: string[]) =>
  request.delete(api.removeTag(knowledgeId), { data: { tags } });

export const renameTag = (
  knowledgeId: string,
  { fromTag, toTag }: IRenameTag,
) => request.put(api.renameTag(knowledgeId), { data: { fromTag, toTag } });

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
  request.get(api.getDatasetFilter(kb_id), { params: {} });

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

export const createDocument = async (datasetId: string, name: string) => {
  const response = await request.post(api.documentCreate(datasetId), {
    data: { name },
  });
  return response.data;
};

export const webCrawlDocument = async (
  datasetId: string,
  formData: FormData,
) => {
  const response = await axios.post(api.webCrawl(datasetId), formData, {
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

export const changeDocumentParser = (
  datasetId: string,
  documentId: string,
  data: { name?: string },
) => request.patch(api.documentChangeParser(datasetId, documentId), { data });

export const deleteDocument = (datasetId: string, documentIds: string[]) =>
  request.delete(api.documentDelete(datasetId), { data: { ids: documentIds } });

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
export const updateDocumentsMetadata = ({
  dataset_id,
  selector,
  updates,
  deletes,
}: {
  dataset_id: string;
  selector?: {
    document_ids?: string[];
    metadata_condition?: any;
  };
  updates?: any[];
  deletes?: any[];
}) =>
  request.patch(api.updateDocumentsMetadata(dataset_id), {
    data: { selector, updates, deletes },
  });

export const updateDocumentMetaDataConfig = ({
  kb_id,
  doc_id,
  data,
}: {
  kb_id: string;
  doc_id: string;
  data: any;
}) =>
  request.put(api.documentUpdateMetaDataConfig(kb_id, doc_id), {
    data: { ...data },
  });

export const changeDocumentsStatus = ({
  kb_id,
  doc_ids,
  status,
}: {
  kb_id: string;
  doc_ids?: string[];
  status: number;
}) =>
  request.post(api.documentChangeStatus(kb_id), { data: { doc_ids, status } });

export const listDataPipelineLogDocument = (
  datasetId: string,
  params?: Record<string, any>,
) => request.get(api.fetchDataPipelineLog(datasetId), { params });

export const listPipelineDatasetLogs = (
  datasetId: string,
  params?: Record<string, any>,
) => request.get(api.fetchPipelineDatasetLogs(datasetId), { params });

export const getPipelineDetail = (datasetId: string, logId: string) =>
  request.get(api.getPipelineDetail(datasetId, logId));

export const getKnowledgeBasicInfo = (datasetId: string) =>
  request.get(api.getKnowledgeBasicInfo(datasetId));

export const checkEmbedding = (datasetId: string, data: Record<string, any>) =>
  request.post(api.checkEmbedding(datasetId), { data });

export const kbUpdateMetaData = (
  datasetId: string,
  data: Record<string, any>,
) => request.put(api.kbUpdateMetaData(datasetId), { data });

export function deletePipelineTask({
  kb_id,
  type,
}: {
  kb_id: string;
  type: ProcessingType;
}) {
  return request.delete(api.unbindPipelineTask(kb_id, type));
}

export default kbService;
