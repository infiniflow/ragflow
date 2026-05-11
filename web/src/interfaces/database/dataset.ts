// for the dataset list
// The data structures returned by the `datasets` interface and `kb/detail` are inconsistent.

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
  graphrag_task_finish_at: null;
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
}

interface Parserconfig {
  auto_keywords: number;
  auto_questions: number;
  children_delimiter: string;
  chunk_token_num: number;
  delimiter: string;
  graphrag: Graphrag;
  html4excel: boolean;
  image_context_size: number;
  layout_recognize: string;
  llm_id: string;
  parent_child: Parentchild;
  raptor: Raptor;
  table_context_size: number;
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
