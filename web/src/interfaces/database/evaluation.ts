export interface IEvaluationDataset {
  id: string;
  tenant_id: string;
  name: string;
  description?: string;
  kb_ids: string[];
  created_by: string;
  create_time: number;
  update_time: number;
  status: number;
}

export interface IEvaluationCase {
  id: string;
  dataset_id: string;
  question: string;
  reference_answer?: string;
  relevant_doc_ids?: string[];
  relevant_chunk_ids?: string[];
  case_metadata?: Record<string, unknown>;
  create_time: number;
}

export interface IEvaluationRun {
  id: string;
  dataset_id: string;
  dialog_id: string;
  name: string;
  config_snapshot: Record<string, unknown>;
  metrics_summary?: Record<string, number>;
  status: 'PENDING' | 'RUNNING' | 'COMPLETED' | 'FAILED';
  created_by: string;
  create_time: number;
  complete_time?: number;
}

export interface IEvaluationResult {
  id: string;
  run_id: string;
  case_id: string;
  generated_answer: string;
  retrieved_chunks: Record<string, unknown>[];
  metrics: Record<string, number>;
  execution_time: number;
  token_usage?: Record<string, unknown>;
  create_time: number;
}

export interface IEvaluationRecommendation {
  issue: string;
  severity: string;
  description: string;
  suggestions: string[];
}
