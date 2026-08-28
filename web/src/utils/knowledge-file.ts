type KnowledgeFileParserInfo = {
  chunk_method?: string;
  parser_id?: string;
};

export const getKnowledgeFileParserId = ({
  chunk_method,
  parser_id,
}: KnowledgeFileParserInfo) => chunk_method ?? parser_id ?? '';
