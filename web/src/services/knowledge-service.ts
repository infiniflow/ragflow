import { IRenameTag } from '@/interfaces/database/knowledge';
import {
  IFetchDocumentListRequestBody,
  IFetchKnowledgeListRequestBody,
  IFetchKnowledgeListRequestParams,
} from '@/interfaces/request/knowledge';
import { ProcessingType } from '@/pages/dataset/dataset-overview/dataset-common';
import api from '@/utils/api';
import registerServer from '@/utils/register-server';
import request, { post } from '@/utils/request';

const {
  create_kb,
  update_kb,
  rm_kb,
  get_kb_detail,
  kb_list,
  get_document_list,
  document_change_status,
  document_rm,
  document_delete,
  document_create,
  document_change_parser,
  document_thumbnails,
  chunk_list,
  create_chunk,
  set_chunk,
  get_chunk,
  switch_chunk,
  rm_chunk,
  retrieval_test,
  document_rename,
  document_run,
  document_upload,
  web_crawl,
  knowledge_graph,
  document_infos,
  upload_and_parse,
  listTagByKnowledgeIds,
  setMeta,
  getMeta,
  retrievalTestShare,
  getKnowledgeBasicInfo,
  fetchDataPipelineLog,
  fetchPipelineDatasetLogs,
  runGraphRag,
  traceGraphRag,
  runRaptor,
  traceRaptor,
  check_embedding,
} = api;

const methods = {
  // 知识库管理
  createKb: {
    url: create_kb,
    method: 'post',
  },
  updateKb: {
    url: update_kb,
    method: 'post',
  },
  rmKb: {
    url: rm_kb,
    method: 'post',
  },
  get_kb_detail: {
    url: get_kb_detail,
    method: 'get',
  },
  getList: {
    url: kb_list,
    method: 'post',
  },
  // document manager
  get_document_list: {
    url: get_document_list,
    method: 'get',
  },
  document_change_status: {
    url: document_change_status,
    method: 'post',
  },
  document_rm: {
    url: document_rm,
    method: 'post',
  },
  document_rename: {
    url: document_rename,
    method: 'post',
  },
  document_create: {
    url: document_create,
    method: 'post',
  },
  document_run: {
    url: document_run,
    method: 'post',
  },
  document_change_parser: {
    url: document_change_parser,
    method: 'post',
  },
  document_thumbnails: {
    url: document_thumbnails,
    method: 'get',
  },
  document_upload: {
    url: document_upload,
    method: 'post',
  },
  web_crawl: {
    url: web_crawl,
    method: 'post',
  },
  document_infos: {
    url: document_infos,
    method: 'post',
  },
  setMeta: {
    url: setMeta,
    method: 'post',
  },
  // chunk管理
  chunk_list: {
    url: chunk_list,
    method: 'post',
  },
  create_chunk: {
    url: create_chunk,
    method: 'post',
  },
  set_chunk: {
    url: set_chunk,
    method: 'post',
  },
  get_chunk: {
    url: get_chunk,
    method: 'get',
  },
  switch_chunk: {
    url: switch_chunk,
    method: 'post',
  },
  rm_chunk: {
    url: rm_chunk,
    method: 'post',
  },
  retrieval_test: {
    url: retrieval_test,
    method: 'post',
  },
  knowledge_graph: {
    url: knowledge_graph,
    method: 'get',
  },
  document_delete: {
    url: document_delete,
    method: 'delete',
  },
  upload_and_parse: {
    url: upload_and_parse,
    method: 'post',
  },
  listTagByKnowledgeIds: {
    url: listTagByKnowledgeIds,
    method: 'get',
  },
  documentFilter: {
    url: api.get_dataset_filter,
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
  get_pipeline_detail: {
    url: api.get_pipeline_detail,
    method: 'get',
  },

  runGraphRag: {
    url: runGraphRag,
    method: 'post',
  },
  traceGraphRag: {
    url: traceGraphRag,
    method: 'get',
  },
  runRaptor: {
    url: runRaptor,
    method: 'post',
  },
  traceRaptor: {
    url: traceRaptor,
    method: 'get',
  },
  pipelineRerun: {
    url: api.pipelineRerun,
    method: 'post',
  },

  checkEmbedding: {
    url: check_embedding,
    method: 'post',
  },
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

export const listDataset = (
  params?: IFetchKnowledgeListRequestParams,
  body?: IFetchKnowledgeListRequestBody,
) => request.post(api.kb_list, { data: body || {}, params });

export const listDocument = (
  params?: IFetchKnowledgeListRequestParams,
  body?: IFetchDocumentListRequestBody,
) => request.post(api.get_document_list, { data: body || {}, params });

export const documentFilter = (kb_id: string) =>
  request.post(api.get_dataset_filter, { kb_id });

export const listDataPipelineLogDocument = (
  params?: IFetchKnowledgeListRequestParams,
  body?: IFetchDocumentListRequestBody,
) => request.post(api.fetchDataPipelineLog, { data: body || {}, params });
export const listPipelineDatasetLogs = (
  params?: IFetchKnowledgeListRequestParams,
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
