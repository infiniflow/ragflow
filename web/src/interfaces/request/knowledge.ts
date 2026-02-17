export interface ITestRetrievalRequestBody {
  question: string;
  similarity_threshold: number;
  vector_similarity_weight: number;
  rerank_id?: string;
  top_k?: number;
  use_kg?: boolean;
  highlight?: boolean;
  kb_id?: string[];
  meta_data_filter?: {
    logic?: string;
    method?: string;
    manual?: Array<{
      key: string;
      op: string;
      value: string;
    }>;
    semi_auto?: string[];
  };
}

export interface IFetchKnowledgeListRequestBody {
  owner_ids?: string[];
}

export interface IFetchKnowledgeListRequestParams {
  kb_id?: string;
  keywords?: string;
  page?: number;
  page_size?: number;
}

export interface IFetchDocumentListRequestBody {
  suffix?: string[];
  run_status?: string[];
  return_empty_metadata?: boolean;
  metadata?: Record<string, string[]>;
}
