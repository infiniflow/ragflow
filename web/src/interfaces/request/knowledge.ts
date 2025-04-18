export interface ITestRetrievalRequestBody {
  question: string;
  similarity_threshold: number;
  keywords_similarity_weight: number;
  rerank_id?: string;
  top_k?: number;
  use_kg?: boolean;
  highlight?: boolean;
  kb_id?: string[];
}
