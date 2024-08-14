export interface IDocumentInfo {
  chunk_num: number;
  create_date: string;
  create_time: number;
  created_by: string;
  id: string;
  kb_id: string;
  location: string;
  name: string;
  parser_config: Parserconfig;
  parser_id: string;
  process_begin_at: null;
  process_duation: number;
  progress: number;
  progress_msg: string;
  run: string;
  size: number;
  source_type: string;
  status: string;
  thumbnail: string;
  token_num: number;
  type: string;
  update_date: string;
  update_time: number;
}

interface Parserconfig {
  chunk_token_num: number;
  layout_recognize: boolean;
  raptor: Raptor;
}

interface Raptor {
  use_raptor: boolean;
}
