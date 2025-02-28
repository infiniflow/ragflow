import { DocumentParserType } from '@/constants/knowledge';

export function isKnowledgeGraphParser(parserId: DocumentParserType) {
  return parserId === DocumentParserType.KnowledgeGraph;
}

export function isNaiveParser(parserId: DocumentParserType) {
  return parserId === DocumentParserType.Naive;
}
