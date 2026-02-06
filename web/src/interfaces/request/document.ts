export interface IChangeParserConfigRequestBody {
  pages?: number[][];
  chunk_token_num?: number;
  layout_recognize?: string;
  task_page_size?: number;
  delimiter?: string;
  auto_keywords?: number;
  auto_questions?: number;
  html4excel?: boolean;
  toc_extraction?: boolean;
  image_table_context_window?: number;
  image_context_size?: number;
  table_context_size?: number;
}

export interface IChangeParserRequestBody {
  parser_id: string;
  pipeline_id: string;
  doc_id: string;
  parser_config: IChangeParserConfigRequestBody;
}

export interface IDocumentMetaRequestBody {
  documentId: string;
  meta: string; // json format string
}
