// for the dataset list
// The data structures returned by the `datasets` interface and `/api/v1/datasets/{id}` are inconsistent.

import { RunningStatus } from '@/constants/knowledge';
import { DataSourceKey } from '@/pages/user-setting/data-source/constant';

export interface IConnector {
  id: string;
  name: string;
  status: RunningStatus;
  source: DataSourceKey;
  auto_parse?: '0' | '1';
}

export interface IDataset {
  avatar?: string;
  chunk_count: number;
  chunk_method: string;
  create_date: string;
  create_time: number;
  created_by: string;
  description?: string;
  document_count: number;
  embedding_model: string;
  size?: number;
  graphrag_task_finish_at: string;
  graphrag_task_id: Nullable<string>;
  id: string;
  language: string;
  mindmap_task_finish_at: null;
  mindmap_task_id: Nullable<string>;
  name: string;
  nickname: string;
  pagerank: number;
  parser_config: Parserconfig;
  permission: string;
  pipeline_id: string;
  raptor_task_finish_at: string;
  raptor_task_id: string;
  similarity_threshold: number;
  status: string;
  tenant_avatar: string;
  tenant_embd_id: number;
  tenant_id: string;
  token_num: number;
  update_date: string;
  update_time: number;
  vector_similarity_weight: number;
  connectors: IConnector[];
}

interface Parserconfig {
  auto_keywords: number;
  auto_questions: number;
  children_delimiter: string;
  chunk_token_num: number;
  delimiter: string;
  from_page?: number;
  to_page?: number;
  graphrag: Graphrag;
  html4excel: boolean;
  image_context_size: number;
  layout_recognize: string;
  llm_id: string;
  metadata?: any;
  built_in_metadata?: Array<{ key: string; type: string }>;
  enable_metadata?: boolean;
  parent_child: Parentchild;
  raptor: Raptor;
  table_context_size: number;
  tag_kb_ids?: string[];
  topn_tags: number;
}

interface Raptor {
  max_cluster: number;
  max_token: number;
  prompt: string;
  random_seed: number;
  threshold: number;
  use_raptor: boolean;
}

interface Parentchild {
  children_delimiter: string;
  use_parent_child: boolean;
}

interface Graphrag {
  entity_types: string[];
  method: string;
  use_graphrag: boolean;
}

export interface IDatasetListResult {
  kbs: IDataset[];
  total_datasets: number;
}

// Types migrated from knowledge.ts

export interface IKnowledgeFileParserConfig {
  chunk_token_num: number;
  layout_recognize: boolean;
  pages: number[][];
  task_page_size: number;
}

export interface IKnowledgeFile {
  chunk_num: number;
  create_date: string;
  create_time: number;
  created_by: string;
  id: string;
  kb_id: string;
  location: string;
  name: string;
  parser_id: string;
  process_begin_at?: any;
  process_duration: number;
  progress: number; // parsing process
  progress_msg: string; // parsing log
  run: RunningStatus; // parsing status
  size: number;
  source_type: string;
  status: string; // enabled
  thumbnail?: any; // base64
  token_num: number;
  type: string;
  update_date: string;
  update_time: number;
  parser_config: IKnowledgeFileParserConfig;
}

export interface ITenantInfo {
  asr_id: string;
  embd_id: string;
  img2txt_id: string;
  llm_id: string;
  name: string;
  parser_ids: string;
  role: string;
  tenant_id: string;
  chat_id: string;
  speech2text_id: string;
  rerank_id?: string;
  tts_id: string;
  // Tenant model IDs
  tenant_asr_id?: string;
  tenant_embd_id?: string;
  tenant_img2txt_id?: string;
  tenant_llm_id?: string;
  tenant_rerank_id?: string;
  tenant_tts_id?: string;
}

export type ChunkDocType = 'image' | 'table' | 'text';

export interface IChunk {
  available_int: number; // Whether to enable, 0: not enabled, 1: enabled
  chunk_id: string;
  content_with_weight: string;
  doc_id: string;
  doc_name: string;
  doc_type_kwd?: ChunkDocType;
  image_id: string;
  important_kwd?: string[];
  question_kwd?: string[]; // keywords
  tag_kwd?: string[];
  positions: number[][];
  tag_feas?: Record<string, number>;
}

export interface ITestingChunk {
  chunk_id: string;
  content_ltks: string;
  content_with_weight: string;
  doc_id: string;
  doc_name: string;
  img_id: string;
  image_id: string;
  important_kwd: any[];
  kb_id: string;
  similarity: number;
  term_similarity: number;
  vector: number[];
  vector_similarity: number;
  highlight: string;
  positions: number[][];
  docnm_kwd: string;
  doc_type_kwd: string;
}

export interface ITestingDocument {
  count: number;
  doc_id: string;
  doc_name: string;
}

export interface ITestingResult {
  chunks: ITestingChunk[];
  documents: ITestingDocument[];
  total: number;
  labels?: Record<string, number>;
}

export interface INextTestingResult {
  chunks: ITestingChunk[];
  doc_aggs: ITestingDocument[];
  total: number;
  labels?: Record<string, number>;
  isRuned?: boolean;
}

export type IRenameTag = { fromTag: string; toTag: string };

export interface IKnowledgeGraph {
  graph: Record<string, any>;
  mind_map: import('@antv/g6/lib/types').TreeData;
}
