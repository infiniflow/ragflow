import { RunningStatus } from '@/constants/knowledge';

export interface IDocumentInfo {
  chunk_num: number;
  create_date: string;
  create_time: number;
  created_by: string;
  id: string;
  kb_id: string;
  location: string;
  name: string;
  parser_config: IParserConfig;
  parser_id: string;
  process_begin_at?: string;
  process_duation: number;
  progress: number;
  progress_msg: string;
  run: RunningStatus;
  size: number;
  source_type: string;
  status: string;
  thumbnail: string;
  token_num: number;
  type: string;
  update_date: string;
  update_time: number;
}

export interface IParserConfig {
  delimiter?: string;
  html4excel?: boolean;
  layout_recognize?: boolean;
  pages: any[];
  raptor?: Raptor;
}

interface Raptor {
  use_raptor: boolean;
}
