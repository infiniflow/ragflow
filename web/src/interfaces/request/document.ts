export interface IChangeParserConfigRequestBody {
  pages: number[][];
  chunk_token_num: number;
  layout_recognize: boolean;
  task_page_size: number;
}

export interface IChangeParserRequestBody {
  parser_id: string;
  doc_id: string;
  parser_config: IChangeParserConfigRequestBody;
}

export interface IDocumentMetaRequestBody {
  documentId: string;
  meta: string; // json format string
}
