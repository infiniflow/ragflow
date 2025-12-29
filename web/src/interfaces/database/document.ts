import { RunningStatus } from '@/constants/knowledge';

export interface IDocumentInfo {
  chunk_num: number;
  create_date: string;
  create_time: number;
  created_by: string;
  nickname: string;
  id: string;
  kb_id: string;
  location: string;
  name: string;
  parser_config: IParserConfig;
  parser_id: string;
  pipeline_id: string;
  pipeline_name: string;
  process_begin_at?: string;
  process_duration: number;
  progress: number;
  progress_msg: string;
  run: RunningStatus;
  size: number;
  source_type: string;
  status: string;
  suffix: string;
  thumbnail: string;
  token_num: number;
  type: string;
  update_date: string;
  update_time: number;
  meta_fields?: Record<string, any>;
}

export interface IParserConfig {
  delimiter?: string;
  html4excel?: boolean;
  layout_recognize?: string;
  pages?: any[];
  chunk_token_num?: number;
  auto_keywords?: number;
  auto_questions?: number;
  toc_extraction?: boolean;
  task_page_size?: number;
  raptor?: Raptor;
  graphrag?: GraphRag;
  image_context_window?: number;
  image_table_context_window?: number;
  image_context_size?: number;
  table_context_size?: number;
  mineru_parse_method?: 'auto' | 'txt' | 'ocr';
  mineru_formula_enable?: boolean;
  mineru_table_enable?: boolean;
  mineru_lang?: string;
  entity_types?: string[];
  metadata?: Array<{
    key?: string;
    description?: string;
    enum?: string[];
  }>;
  enable_metadata?: boolean;
}

interface Raptor {
  use_raptor: boolean;
}

interface GraphRag {
  community?: boolean;
  entity_types?: string[];
  method?: string;
  resolution?: boolean;
  use_graphrag?: boolean;
}

export type IDocumentInfoFilter = {
  run_status: Record<number, number>;
  suffix: Record<string, number>;
  metadata: Record<string, Record<string, number>>;
};
