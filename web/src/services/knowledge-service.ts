/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { ProcessingType } from '@/constants/knowledge';
import { IRenameTag } from '@/interfaces/database/dataset';
import {
  IFetchArtifactGraphRequestParams,
  IFetchArtifactListRequestParams,
  IFetchArtifactTopicListRequestParams,
  IFetchDocumentListRequestBody,
  IFetchKnowledgeListRequestParams,
  IUpdateArtifactPageRequestBody,
} from '@/interfaces/request/knowledge';
import api from '@/utils/api';
import nextRequest from '@/utils/next-request';
import registerServer, { registerNextServer } from '@/utils/register-server';
import request from '@/utils/request';

const {
  createKb,
  rmKb,
  kbList,
  documentThumbnails,
  documentIngest,
  listTagByKnowledgeIds,
  listPipelines,
  setMeta,
  getMeta,
  getMetaKeys,
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
  documentIngest: {
    url: documentIngest,
    method: 'post',
  },
  documentThumbnails: {
    url: documentThumbnails,
    method: 'get',
  },
  setMeta: {
    url: setMeta,
    method: 'post',
  },
  listTagByKnowledgeIds: {
    url: listTagByKnowledgeIds,
    method: 'get',
  },
  getMeta: {
    url: getMeta,
    method: 'get',
  },
  getMetaKeys: {
    url: getMetaKeys,
    method: 'get',
  },
  retrievalTestShare: {
    url: retrievalTestShare,
    method: 'post',
  },
  listPipelines: {
    url: listPipelines,
    method: 'get',
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

const mapChunkToRetrieval = (chunk: Record<string, any>) => ({
  ...chunk,
  id: chunk.id || chunk.chunk_id,
  content: chunk.content ?? chunk.content_with_weight,
  document_id: chunk.document_id || chunk.doc_id,
  document_keyword: chunk.document_keyword || chunk.docnm_kwd || chunk.doc_name,
  dataset_id: chunk.dataset_id || chunk.kb_id,
  important_keywords: chunk.important_keywords || chunk.important_kwd || [],
  questions: chunk.questions || chunk.question_kwd || [],
});

const mapRetrievalResponse = (response: any) => {
  if (response.data?.code === 0) {
    response.data.data = {
      ...response.data.data,
      chunks: (response.data.data?.chunks || []).map(mapChunkToRetrieval),
    };
  }
  return response;
};

const toLegacyMetadataFilter = (condition?: Record<string, any>) => {
  if (!condition?.conditions?.length) {
    return undefined;
  }
  return {
    method: 'manual',
    logic: condition.logic,
    manual: condition.conditions.map((item: Record<string, any>) => ({
      key: item.name,
      op: item.comparison_operator,
      value: item.value,
    })),
  };
};

const mapDocumentToLegacy = (doc: Record<string, any>) => ({
  ...doc,
  chunk_num: doc.chunk_num ?? doc.chunk_count,
  kb_id: doc.kb_id || doc.dataset_id,
  parser_id: doc.parser_id || doc.chunk_method,
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
  retrievalTest: async (params: Record<string, any>) => {
    const datasetId = params.dataset_id || params.kb_id || params.knowledge_id;
    if (!datasetId) {
      throw new Error(
        'dataset_id (or kb_id/knowledge_id) is required for retrievalTest',
      );
    }
    const datasetIds = Array.isArray(datasetId) ? datasetId : [datasetId];
    const rest = { ...params };
    delete rest.dataset_id;
    delete rest.kb_id;
    delete rest.knowledge_id;
    const data = {
      dataset_ids: datasetIds,
      document_ids: rest.document_ids ?? rest.doc_ids,
      question: rest.question,
      page: rest.page,
      page_size: rest.page_size ?? rest.size,
      similarity_threshold: rest.similarity_threshold,
      vector_similarity_weight: rest.vector_similarity_weight,
      top_k: rest.top_k,
      knn_top_k: rest.knn_top_k,
      knn_num_candidates: rest.knn_num_candidates,
      rerank_candidates_count: rest.rerank_candidates_count,
      rerank_id: rest.rerank_id,
      search_id: rest.search_id,
      keyword: rest.keyword,
      highlight: rest.highlight,
      cross_languages: rest.cross_languages,
      meta_data_filter: rest.meta_data_filter,
      chat_id: rest.chat_id,
      use_kg: rest.use_kg,
      toc_enhance: rest.toc_enhance,
      include_knowledge_compilation: rest.include_knowledge_compilation,
      reference_metadata: rest.reference_metadata,
    };
    const response = await request.post(api.retrievalTest, {
      data,
    });
    return mapRetrievalResponse(response);
  },
  retrievalTestShare: async (params: Record<string, any>) => {
    const response = await baseKbService.retrievalTestShare({
      ...params,
      doc_ids: params.doc_ids ?? params.document_ids,
      size: params.size ?? params.page_size,
      meta_data_filter:
        params.meta_data_filter ??
        toLegacyMetadataFilter(params.metadata_condition),
    });
    return mapRetrievalResponse(response);
  },
  chunkList: async (params: Record<string, any>) => {
    const datasetId = getDatasetId(params);
    const documentId = getDocumentId(params);
    const response = await request.get(api.chunkList(datasetId, documentId), {
      params: {
        page: params.page,
        page_size: params.page_size || params.size,
        keywords: params.keywords,
        available: getAvailableParam(params.available_int),
        chunk_ids: params.chunk_ids,
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

export const getKbDetail = async (datasetId: string) => {
  const response = await request.get(api.getKbDetail(datasetId));
  // The /api/v1/datasets/<id> endpoint returns chunk_count/document_count,
  // but legacy consumers (e.g. the GraphRAG/Raptor "magic wand" enable check
  // in dataset/index.tsx) read chunk_num/doc_num. Normalize both shapes.
  if (response.data?.code === 0 && response.data.data) {
    const d = response.data.data;
    response.data.data = {
      ...d,
      chunk_num: d.chunk_num ?? d.chunk_count,
      doc_num: d.doc_num ?? d.document_count,
    };
  }
  return response;
};

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
  return request.delete(api.knowledgeGraph(knowledgeId));
}

export const listDataset = (params?: IFetchKnowledgeListRequestParams) =>
  request.get(api.kbList, {
    params: params
      ? { ...params, owner_ids: params.owner_ids?.join(',') }
      : params,
  });

// Fetch datasets by a set of IDs via the `ids` query param (comma-joined).
// Used to echo back already-selected datasets whose names are not present
// in the first page of the paginated list.
export const listDatasetByIds = (ids: string[]) =>
  request.get(api.kbList, {
    params: { ids: ids.join(','), page_size: ids.length },
  });

export const datasetFilter = () => request.get(api.datasetFilter);

export const updateKb = (datasetId: string, data: Record<string, any>) =>
  request.put(api.updateKb(datasetId), { data });

export const runIndex = (datasetId: string, indexType: string) =>
  request.post(api.runIndex(datasetId, indexType));

export const traceIndex = (datasetId: string, indexType: string) =>
  request.get(api.traceIndex(datasetId, indexType));

// getDatasetCompilationStatus reads the Go scheduler compile-status contract
// (GET /datasets/:id/compilation/status), used on the Go backend to
// replace the legacy traceIndex task-progress endpoint. Route it through the
// service-layer proxy (registerNextServer -> next-request) like the rest of the
// *-service.ts HTTP proxies.
const compilationStatusProxy = registerNextServer({
  getDatasetCompilationStatus: {
    url: (datasetId: string) => api.compilationStatus(datasetId),
    method: 'get',
  },
} as const);
export const getDatasetCompilationStatus = (datasetId: string) =>
  compilationStatusProxy.getDatasetCompilationStatus(datasetId);

// Using RESTful API: GET /api/v1/datasets/{dataset_id}/documents
export const listDocument = (
  params?: IFetchKnowledgeListRequestParams,
  body?: IFetchDocumentListRequestBody,
) => {
  if (!params || !params.id) {
    throw new Error('params and params.id are required');
  }
  const { page, page_size, keywords } = params;
  const mergedParams = {
    page,
    page_size,
    keywords,
    ...body,
  };
  return request.get(api.getDocumentList(params.id), { params: mergedParams });
};

export const documentFilter = (kb_id: string) =>
  request.get(api.getDatasetFilter(kb_id), { params: {} });

export const uploadDocument = async (datasetId: string, formData: FormData) => {
  const url = api.documentUpload(datasetId);
  const response = await request.post(url, { data: formData });
  return response.data;
};

export const createDocument = async (datasetId: string, name: string) => {
  const response = await request.post(api.documentCreate(datasetId), {
    data: { name },
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

export const listArtifacts = (
  datasetId: string,
  params?: IFetchArtifactListRequestParams,
) => request.get(api.artifactsList(datasetId), { params });

export const listArtifactTopics = (
  datasetId: string,
  params?: IFetchArtifactTopicListRequestParams,
) => request.get(api.artifactsTopicList(datasetId), { params });

export const getArtifactPage = (
  datasetId: string,
  pageType: string,
  slug: string,
) => request.get(api.getArtifactPage(datasetId, pageType, slug));

export const getArtifactGraph = (
  datasetId: string,
  params?: IFetchArtifactGraphRequestParams,
) => request.get(api.getArtifactGraph(datasetId), { params });

export const getArtifactsAlteration = (datasetId: string, kind: string) =>
  request.get(api.artifactsAlteration(datasetId), { params: { kind } });

export const getArtifactsStructure = (
  datasetId: string,
  kind: string,
  keywords?: string,
) =>
  request.get(api.artifactsStructure(datasetId), {
    params: keywords ? { kind, keywords } : { kind },
  });

export const deleteArtifactsStructure = (datasetId: string, kind: string) =>
  request.delete(api.artifactsStructure(datasetId), { params: { kind } });

export const updateArtifactPage = (
  datasetId: string,
  pageType: string,
  slug: string,
  data: IUpdateArtifactPageRequestBody,
) => request.put(api.getArtifactPage(datasetId, pageType, slug), { data });

export const listWikiCommits = (
  datasetId: string,
  pageType: string,
  slug: string,
  params?: { page?: number; page_size?: number },
) =>
  request.get(api.listWikiCommits(datasetId), {
    params: {
      ...params,
      slug: slug.startsWith(`${pageType}/`) ? slug : `${pageType}/${slug}`,
    },
  });

export const getWikiCommit = (datasetId: string, commitId: string) =>
  request.get(api.getWikiCommit(datasetId, commitId));

export const clearWiki = (datasetId: string) =>
  nextRequest.delete(api.clearWiki(datasetId), {});

export const checkEmbedding = (datasetId: string, data: Record<string, any>) =>
  request.post(api.checkEmbedding(datasetId), { data });

export const kbUpdateMetaData = (
  datasetId: string,
  data: Record<string, any>,
) => request.put(api.kbUpdateMetaData(datasetId), { data });

export function deletePipelineTask({
  kb_id,
  type,
  wipe,
}: {
  kb_id: string;
  type: ProcessingType;
  wipe?: boolean;
}) {
  return request.delete(api.unbindPipelineTask(kb_id, type, wipe));
}

export default kbService;
